package web_test

import (
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalViewDisposal(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		page := fixture.page
		trackConnections(t, page)
		openTerminal(t, page, fixture.url)
		_, err := page.Evaluate(`() => {
			const write = Terminal.prototype.write;
			const callbacks = [];
			Terminal.prototype.write = function(data, callback) {
				write.call(this, data, () => callbacks.push(callback));
			};
			window.finishWrites = () => callbacks.forEach(callback => callback());
		}`)
		requireNoError(t, err)
		sendLine(t, page, "pending-render")
		requireOutput(t, page, "received: pending-render")
		polls := make(chan playwright.Route, 1)
		requireNoError(t, page.Route("**/conversations/default/terminal", func(route playwright.Route) { polls <- route }))
		var poll playwright.Route
		select {
		case poll = <-polls:
		case <-time.After(5 * time.Second):
			t.Fatal("status was not polled")
		}
		_, err = page.Evaluate(`async () => {
			const app = await import(new URL('app.js', document.baseURI));
			app.dispose();
			app.dispose();
			document.querySelector('#status').textContent = 'Away';
			window.finishWrites();
		}`)
		requireNoError(t, err)
		requireNoError(t, poll.Fulfill(playwright.RouteFulfillOptions{
			ContentType: playwright.String("application/json"),
			Body:        `{"state":"exited","exit_code":7}`,
		}))
		// Cross a reconnect and polling interval; stale work must not revive the view.
		time.Sleep(1200 * time.Millisecond)
		requireStatus(t, page, "Away")
		if connectionCount(t, page) != 1 {
			t.Fatal("disposed view reconnected")
		}
		response, err := page.Request().Get(fixture.endpoint)
		requireNoError(t, err)
		var state struct {
			State string `json:"state"`
		}
		requireNoError(t, response.JSON(&state))
		if state.State != "running" {
			t.Fatalf("disposal stopped harness: %s", state.State)
		}
	})
}
