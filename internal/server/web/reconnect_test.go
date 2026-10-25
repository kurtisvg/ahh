package web_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalReconnectAfterRendering(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		page := fixture.page
		trackConnections(t, page)
		openTerminal(t, page, fixture.url)
		requireOutput(t, page, "fake harness ready")
		output, err := page.Locator(".xterm-accessibility-tree").TextContent()
		requireNoError(t, err)
		pid := regexp.MustCompile(`pid=\d+`).FindString(output)
		if pid == "" {
			t.Fatal("harness did not report its PID")
		}

		// Hold render callbacks across disconnect to exercise the resume cursor.
		_, err = page.Evaluate(`() => {
			const write = Terminal.prototype.write;
			const callbacks = [];
			Terminal.prototype.write = function (data, callback) {
				write.call(this, data, () => callbacks.push(callback));
			};
			window.finishRender = () => {
				Terminal.prototype.write = write;
				for (const callback of callbacks) callback();
			};
		}`)
		requireNoError(t, err)
		sendLine(t, page, "before-disconnect")
		requireOutput(t, page, "received: before-disconnect")
		disconnect(t, page)
		// No retry should start while the last rendered offset is still unknown.
		time.Sleep(700 * time.Millisecond)
		if count := connectionCount(t, page); count != 1 {
			t.Fatalf("reconnected before rendering: %d connections", count)
		}
		_, err = page.Evaluate("() => window.finishRender()")
		requireNoError(t, err)
		requireStatus(t, page, "Connected")
		if count := connectionCount(t, page); count != 2 {
			t.Fatalf("connections after reconnect = %d, want 2", count)
		}
		output, err = page.Locator(".xterm-accessibility-tree").TextContent()
		requireNoError(t, err)
		if strings.Count(output, "received: before-disconnect") != 1 {
			t.Fatal("reconnect repeated or lost rendered output")
		}
		requireOutput(t, page, pid)

		// Both browsers can still write after one reconnects.
		second, err := fixture.context.NewPage()
		requireNoError(t, err)
		openTerminal(t, second, fixture.url)
		for index, client := range []playwright.Page{page, second} {
			line := fmt.Sprintf("after-reconnect-%d", index)
			sendLine(t, client, line)
			requireOutput(t, page, "received: "+line)
			requireOutput(t, second, "received: "+line)
		}
	})
}

func TestTerminalReconnectHistoryGap(t *testing.T) {
	// Wait for input before overflowing history, then stay alive for reconnect.
	script := `#!/bin/sh
		printf 'ready for flood\n'
		IFS= read -r line
		head -c 1200000 /dev/zero | tr '\000' x
		printf '\nflood complete\n'
		while IFS= read -r line; do
			:
		done
	`
	testConnections(t, script, func(t *testing.T, fixture browserTest) {
		page := fixture.page
		trackConnections(t, page)
		reconnect := controlReconnect(t, page)
		openTerminal(t, page, fixture.url)
		requireOutput(t, page, "ready for flood")

		// Exercise the history gap after an earlier successful reconnect.
		attempts := connectionCount(t, page)
		_, err := page.Evaluate("() => window.connections.at(-1).close()")
		requireNoError(t, err)
		handle, err := page.WaitForFunction(`previous =>
			window.connections.length > previous &&
			window.connections.at(-1).readyState === WebSocket.OPEN`, attempts)
		requireNoError(t, err)
		requireNoError(t, handle.Dispose())
		requireStatus(t, page, "Connected")

		reconnect.Store(false)
		disconnect(t, page)

		second, err := fixture.context.NewPage()
		requireNoError(t, err)
		openTerminal(t, second, fixture.url)
		sendLine(t, second, "flood")
		expect := playwright.NewPlaywrightAssertions(15000)
		requireNoError(t, expect.Locator(second.Locator(".xterm-accessibility-tree")).ToContainText("flood complete"))

		reconnect.Store(true)
		requireNoError(t, expect.Locator(page.Locator("#notice")).ToHaveText("Earlier output is no longer available"))
		requireNoError(t, expect.Locator(page.Locator(".xterm-accessibility-tree")).ToContainText("flood complete"))
		output, err := page.Locator(".xterm-accessibility-tree").TextContent()
		requireNoError(t, err)
		if strings.Contains(output, "ready for flood") {
			t.Fatal("terminal retained stale output after a history gap")
		}
	})
}

func TestTerminalStopsReconnectAfterExit(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		page := fixture.page
		trackConnections(t, page)
		reconnect := controlReconnect(t, page)
		openTerminal(t, page, fixture.url)
		requireOutput(t, page, "fake harness ready")
		reconnect.Store(false)
		disconnect(t, page)

		second, err := fixture.context.NewPage()
		requireNoError(t, err)
		openTerminal(t, second, fixture.url)
		sendLine(t, second, "exit")
		requireOutput(t, second, "final output before exit")
		requireStatus(t, page, "Stopped (exit 0)")

		reconnect.Store(true)
		attempts := connectionCount(t, page)
		// Wait beyond two retry intervals to prove that confirmed exit stops retries.
		time.Sleep(1200 * time.Millisecond)
		if count := connectionCount(t, page); count != attempts {
			t.Fatalf("reconnected after confirmed exit: %d connections, previously %d", count, attempts)
		}
	})
}

func disconnect(t *testing.T, page playwright.Page) {
	t.Helper()
	_, err := page.Evaluate("() => window.connections.at(-1).close()")
	requireNoError(t, err)
	requireStatus(t, page, "Reconnecting")
}

func controlReconnect(t *testing.T, page playwright.Page) *atomic.Bool {
	t.Helper()
	allowed := &atomic.Bool{}
	allowed.Store(true)
	requireNoError(t, page.RouteWebSocket("**/pty*", func(route playwright.WebSocketRoute) {
		if !allowed.Load() {
			route.Close()
			return
		}
		if _, err := route.ConnectToServer(); err != nil {
			t.Error(err)
		}
	}))
	return allowed
}
