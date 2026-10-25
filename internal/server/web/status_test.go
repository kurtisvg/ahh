package web_test

import (
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalExitStatus(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		page := fixture.page
		trackConnections(t, page)
		openTerminal(t, page, fixture.url)
		requireOutput(t, page, "fake harness ready")

		// Losing the status endpoint must not turn a disconnect into a claimed exit.
		requireNoError(t, page.Route("**/conversations/default/terminal", func(route playwright.Route) {
			if err := route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(503)}); err != nil {
				t.Error(err)
			}
		}))
		sendLine(t, page, "last output")
		requireOutput(t, page, "received: last output")
		sendLine(t, page, "exit")
		requireStatus(t, page, "Disconnected")
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#status")).ToHaveAttribute("data-state", "disconnected"))
		requireOutput(t, page, "final output before exit")
		// Wait beyond two retry intervals to prove that complete output stops retries.
		time.Sleep(1200 * time.Millisecond)
		if count := connectionCount(t, page); count != 1 {
			t.Fatalf("reconnected after complete output: %d connections", count)
		}

		requireNoError(t, page.Unroute("**/conversations/default/terminal"))
		requireStatus(t, page, "Stopped (exit 0)")
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#terminal")).ToBeVisible())
		requireOutput(t, page, "received: last output")
		_, err := page.Reload()
		requireNoError(t, err)
		requireStatus(t, page, "Stopped (exit 0)")
		snapshot, err := page.Request().Get(fixture.endpoint)
		requireNoError(t, err)
		var state struct {
			Invocation string `json:"invocation"`
		}
		requireNoError(t, snapshot.JSON(&state))
		response, err := page.Request().Get(fixture.endpoint + "/" + state.Invocation + "/pty")
		requireNoError(t, err)
		if response.Status() != 410 {
			t.Fatalf("terminal after exit: HTTP %d", response.Status())
		}
	})
}
