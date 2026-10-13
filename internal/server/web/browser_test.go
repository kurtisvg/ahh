package web_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

type browserTest struct {
	page      playwright.Page
	context   playwright.BrowserContext
	url       string
	directory string
}

// testConnections gives each direct/proxy case its own process, browser, and files.
func testConnections(t *testing.T, script string, run func(*testing.T, browserTest)) {
	t.Helper()
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	binary := buildBrowserBinary(t)
	if script == "" {
		data, err := os.ReadFile("../../harness/pty/testdata/fake-harness")
		requireNoError(t, err)
		script = string(data)
	}

	for _, mode := range []string{"direct", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			bin := filepath.Join(directory, "bin")
			requireNoError(t, os.Mkdir(bin, 0o700))
			requireNoError(t, os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o700))
			url, _ := startBrowserServer(t, binary, directory, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if mode == "proxy" {
				url = startBrowserProxy(t, url)
			}
			browser := newBrowserContext(t)
			page, err := browser.NewPage()
			requireNoError(t, err)
			run(t, browserTest{page: page, context: browser, url: url, directory: directory})
		})
	}
}

func buildBrowserBinary(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	requireNoError(t, err)
	binary := filepath.Join(t.TempDir(), "ahh")
	runBrowserCommand(t, root, "go", "build", "-o", binary, ".")
	return binary
}

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

// startBrowserServer returns the ready URL and a graceful stop function.
// Cleanup cancels and reaps the process even when a test fails before stopping it.
func startBrowserServer(t *testing.T, binary, directory string, env ...string) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, binary, "serve", "--port", "0")
	command.Dir = directory
	command.Env = append(os.Environ(), env...)
	command.Stderr = os.Stderr
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 10 * time.Second
	stdout, err := command.StdoutPipe()
	requireNoError(t, err)
	if err := command.Start(); err != nil {
		_ = stdout.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		if command.ProcessState == nil {
			_ = command.Wait()
		}
		_ = stdout.Close()
	})

	url, err := bufio.NewReader(stdout).ReadString('\n')
	requireNoError(t, err)
	return strings.TrimSpace(url), func() {
		t.Helper()
		requireNoError(t, command.Process.Signal(syscall.SIGTERM))
		requireNoError(t, command.Wait())
	}
}

func newBrowserContext(t *testing.T) playwright.BrowserContext {
	t.Helper()
	pw, err := playwright.Run()
	requireNoError(t, err)
	t.Cleanup(func() {
		if err := pw.Stop(); err != nil {
			t.Error(err)
		}
	})
	browser, err := pw.Chromium.Launch()
	requireNoError(t, err)
	t.Cleanup(func() {
		if err := browser.Close(); err != nil {
			t.Error(err)
		}
	})
	browserContext, err := browser.NewContext()
	requireNoError(t, err)
	browserContext.SetDefaultTimeout(5000)
	t.Cleanup(func() {
		if err := browserContext.Close(); err != nil {
			t.Error(err)
		}
	})
	requireNoError(t, browserContext.Tracing().Start(playwright.TracingStartOptions{
		Screenshots: playwright.Bool(true),
		Snapshots:   playwright.Bool(true),
	}))
	t.Cleanup(func() {
		var paths []string
		if t.Failed() {
			paths = append(paths, filepath.Join(t.ArtifactDir(), "trace.zip"))
		}
		if err := browserContext.Tracing().Stop(paths...); err != nil {
			t.Error(err)
		}
	})
	return browserContext
}

func openTerminal(t *testing.T, page playwright.Page, url string) {
	t.Helper()
	_, err := page.Goto(url)
	requireNoError(t, err)
	requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#status")).ToHaveText("Connected"))
}

func sendLine(t *testing.T, page playwright.Page, line string) {
	t.Helper()
	requireNoError(t, page.Locator(".xterm-helper-textarea").Focus())
	requireNoError(t, page.Keyboard().Type(line))
	requireNoError(t, page.Keyboard().Press("Enter"))
}

func requireOutput(t *testing.T, page playwright.Page, expected any) {
	t.Helper()
	requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator(".xterm-accessibility-tree")).ToContainText(expected))
}

func requireTerminalFits(t *testing.T, page playwright.Page) {
	t.Helper()
	// Wait for the resize observer to fit xterm after a viewport change.
	handle, err := page.WaitForFunction(`() => {
		const screen = document.querySelector('.xterm-screen').getBoundingClientRect();
		const container = document.querySelector('#terminal').getBoundingClientRect();
		return screen.right <= container.right && screen.bottom <= container.bottom;
	}`, nil)
	requireNoError(t, err)
	requireNoError(t, handle.Dispose())
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireStatus(t *testing.T, page playwright.Page, expected string) {
	t.Helper()
	requireNoError(t, playwright.NewPlaywrightAssertions().Locator(page.Locator("#status")).ToHaveText(expected))
}

// Expose sockets so tests can disconnect without stopping the harness.
func trackConnections(t *testing.T, page playwright.Page) {
	t.Helper()
	requireNoError(t, page.AddInitScript(playwright.Script{Content: playwright.String(`
		const NativeWebSocket = window.WebSocket;
		window.connections = [];
		window.WebSocket = class extends NativeWebSocket {
			constructor(...args) {
				super(...args);
				window.connections.push(this);
			}
		};
	`)}))
}

func connectionCount(t *testing.T, page playwright.Page) int {
	t.Helper()
	value, err := page.Evaluate("() => window.connections.length")
	requireNoError(t, err)
	count, ok := value.(int)
	if !ok {
		t.Fatalf("unexpected connection count: %v", value)
	}
	return count
}
