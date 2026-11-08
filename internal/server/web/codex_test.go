//go:build integration_codex

package web_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

func runBrowserCommand(t *testing.T, directory string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Dir = directory
	command.WaitDelay = 10 * time.Second
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", args[0], err, output)
	}
	return output
}

func TestCodexSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}

	version := runBrowserCommand(t, ".", "codex", "--version")
	t.Logf("Codex version: %s", strings.TrimSpace(string(version)))
	requireNoError(t, os.WriteFile(filepath.Join(t.ArtifactDir(), "codex-version.txt"), version, 0o600))
	directory := t.TempDir()
	url, stop := startBrowserServer(t, ahhBinary.Get(t), directory, "/", "TERM=")
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
