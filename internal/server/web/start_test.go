package web_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalWithoutInstalledHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	directory := t.TempDir()
	git, err := exec.LookPath("git")
	requireNoError(t, err)
	requireNoError(t, os.Symlink(git, filepath.Join(directory, "git")))
	url, _ := startBrowserServer(t, buildBrowserBinary(t), directory, "/", "PATH="+directory)
	browser := newBrowserContext(t)
	page, err := browser.NewPage()
	requireNoError(t, err)
	p := createBrowserProject(t, page, url, "no-harness")
	_, err = page.Goto(url + "/p/" + p.ID + "/c")
	requireNoError(t, err)
	requireStatus(t, page, "Stopped")
	expect := playwright.NewPlaywrightAssertions()
	requireNoError(t, expect.Locator(page.Locator("#terminal")).ToBeHidden())
	requireNoError(t, expect.Locator(page.Locator(".conversation-start #start")).ToBeVisible())
	_, err = page.Reload()
	requireNoError(t, err)
	requireStatus(t, page, "Stopped")
	for range 2 {
		requireNoError(t, page.Locator("#start").Click())
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#notice")).ToContainText("Could not start terminal"))
		requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#start")).ToBeEnabled())
		requireNoError(t, expect.Locator(page.Locator("#terminal")).ToBeHidden())
	}
}
