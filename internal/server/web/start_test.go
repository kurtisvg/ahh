package web_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalWithoutInstalledHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	directory := t.TempDir()
	url, _ := startBrowserServer(t, buildBrowserBinary(t), directory, "PATH="+directory)
	browser := newBrowserContext(t)
	page, err := browser.NewPage()
	requireNoError(t, err)
	_, err = page.Goto(url)
	requireNoError(t, err)
	requireStatus(t, page, "Stopped")
	_, err = page.Reload()
	requireNoError(t, err)
	requireStatus(t, page, "Stopped")
	for range 2 {
		requireNoError(t, page.Locator("#start").Click())
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#notice")).ToContainText("Could not start terminal"))
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#start")).ToBeEnabled())
	}
}
