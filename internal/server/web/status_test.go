package web_test

import (
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalExitStatus(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		page := fixture.page
		openTerminal(t, page, fixture.url)
		requireOutput(t, page, "fake harness ready")

		// Losing the status endpoint must not turn a disconnect into a claimed exit.
		requireNoError(t, page.Route("**/status", func(route playwright.Route) {
			if err := route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(503)}); err != nil {
				t.Error(err)
			}
		}))
		sendLine(t, page, "last output")
		requireOutput(t, page, "received: last output")
		sendLine(t, page, "exit")
		requireStatus(t, page, "Disconnected")

		requireNoError(t, page.Unroute("**/status"))
		requireStatus(t, page, "Stopped (exit 0)")
		requireOutput(t, page, "received: last output")
		_, err := page.Reload()
		requireNoError(t, err)
		requireStatus(t, page, "Stopped (exit 0)")
		response, err := page.Request().Get(strings.TrimRight(fixture.url, "/") + "/pty")
		requireNoError(t, err)
		if response.Status() != 410 {
			t.Fatalf("terminal after exit: HTTP %d", response.Status())
		}
	})
}
