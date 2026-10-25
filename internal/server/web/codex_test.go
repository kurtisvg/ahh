//go:build integration_codex

package web_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestCodexSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}

	version := runBrowserCommand(t, ".", "codex", "--version")
	t.Logf("Codex version: %s", strings.TrimSpace(string(version)))
	requireNoError(t, os.WriteFile(filepath.Join(t.ArtifactDir(), "codex-version.txt"), version, 0o600))
	directory := t.TempDir()
	url, stop := startBrowserServer(t, buildBrowserBinary(t), directory, "/", "TERM=")
	browser := newBrowserContext(t)
	page, err := browser.NewPage()
	requireNoError(t, err)
	p := createBrowserProject(t, page, url, "codex-smoke")
	openTerminal(t, page, url+"/p/"+p.ID+"/c")

	// A new directory may show the trust screen; leave that decision untouched.
	ready := regexp.MustCompile(`OpenAI Codex|Trust this folder\?`)
	expect := playwright.NewPlaywrightAssertions()
	requireNoError(t, expect.Locator(page.Locator(".xterm-accessibility-tree")).ToContainText(ready,
		playwright.LocatorAssertionsToContainTextOptions{Timeout: playwright.Float(20000)}))
	requireNoError(t, page.SetViewportSize(900, 600))
	requireTerminalFits(t, page)
	response, err := page.Request().Get(url + "/api/projects/" + p.ID + "/conversations/default/terminal")
	requireNoError(t, err)
	var status struct {
		State string `json:"state"`
	}
	requireNoError(t, response.JSON(&status))
	if status.State != "running" {
		t.Fatalf("terminal state = %q, want running", status.State)
	}
	_, err = page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(filepath.Join(t.ArtifactDir(), "codex.png")),
	})
	requireNoError(t, err)

	stop()
	requireNoError(t, expect.Locator(page.Locator("#status")).ToContainText("Disconnected"))
}
