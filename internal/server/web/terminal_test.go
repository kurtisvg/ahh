package web_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalInputAndResize(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		page := fixture.page
		openTerminal(t, page, fixture.url)
		sendLine(t, page, "hello-browser")
		requireOutput(t, page, "received: hello-browser")
		requireOutput(t, page, fixture.directory)
		sendLine(t, page, "size")
		size := regexp.MustCompile(`size=(\d+ \d+)`)
		requireOutput(t, page, size)
		output, err := page.Locator(".xterm-accessibility-tree").TextContent()
		requireNoError(t, err)
		initial := size.FindStringSubmatch(output)[1]

		requireNoError(t, page.SetViewportSize(900, 600))
		requireTerminalFits(t, page)
		sendLine(t, page, "size")
		// Wait until the harness observes a size different from the original one.
		handle, err := page.WaitForFunction(`initial => {
			const output = document.querySelector('.xterm-accessibility-tree').textContent;
			return [...output.matchAll(/size=(\d+ \d+)/g)].some(match => match[1] !== initial);
		}`, initial)
		requireNoError(t, err)
		requireNoError(t, handle.Dispose())
		_, err = page.Screenshot(playwright.PageScreenshotOptions{
			Path: playwright.String(filepath.Join(t.ArtifactDir(), "terminal.png")),
		})
		requireNoError(t, err)
	})
}

func TestTerminalSharedInput(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		openTerminal(t, fixture.page, fixture.url)
		second, err := fixture.context.NewPage()
		requireNoError(t, err)
		openTerminal(t, second, fixture.url)

		pages := []playwright.Page{fixture.page, second}
		for index, page := range pages {
			line := fmt.Sprintf("from-browser-%d", index)
			sendLine(t, page, line)
			for _, reader := range pages {
				requireOutput(t, reader, "received: "+line)
			}
		}
	})
}
