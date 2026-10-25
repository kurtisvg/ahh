package web_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalPageRoutes(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		url := fixture.url
		page := fixture.page
		openTerminal(t, page, url)
		expect := playwright.NewPlaywrightAssertions()
		requireNoError(t, expect.Locator(page.Locator("[aria-live]:not([aria-live=off]) #terminal")).ToHaveCount(0))
		requireNoError(t, expect.Locator(page.Locator("#terminal [aria-live=assertive]")).ToHaveCount(1))
		sendLine(t, page, "bookmark")
		requireOutput(t, page, "received: bookmark")
		_, err := page.Reload()
		requireNoError(t, err)
		requireStatus(t, page, "Connected")
		requireOutput(t, page, "received: bookmark")
		_, err = page.Goto(fixture.baseURL + "/")
		requireNoError(t, err)
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#detail")).ToContainText("Fixture"))
		requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
			Name:  "Conversations",
			Exact: playwright.Bool(true),
		}).Click())
		requireNoError(t, playwright.NewPlaywrightAssertions().Page(page).ToHaveURL(url))
		_, err = page.GoBack()
		requireNoError(t, err)
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#detail")).ToContainText("Fixture"))
		_, err = page.GoForward()
		requireNoError(t, err)
		requireStatus(t, page, "Connected")
		for _, path := range []string{"api/missing", "missing.js", "terminal", "status", "pty"} {
			response, err := page.Request().Get(fixture.baseURL + "/" + path)
			requireNoError(t, err)
			if response.Status() != 404 {
				t.Fatalf("%s returned %d", path, response.Status())
			}
		}
	})
}
