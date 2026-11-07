package web_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestTerminalLargePaste(t *testing.T) {
	// Capture bytes without terminal line limits or input echo.
	script := `#!/bin/sh
		while IFS= read -r count; do
			stty raw -echo
			printf 'capture ready: %s\n' "$count"
			head -c "$count" > pasted-input
			stty sane
			printf 'captured: %s\n' "$count"
		done
	`
	testConnections(t, script, func(t *testing.T, fixture browserTest) {
		page := fixture.page
		var mu sync.Mutex
		var frameSizes []int
		page.OnWebSocket(func(socket playwright.WebSocket) {
			socket.OnFrameSent(func(data []byte) {
				mu.Lock()
				defer mu.Unlock()
				frameSizes = append(frameSizes, len(data))
			})
		})
		openTerminal(t, page, fixture.url)

		for _, text := range []string{
			strings.Repeat("a", 64*1024),
			strings.Repeat("x", 8191) + strings.Repeat("🙂界\"\\\x01", 8192),
			strings.Repeat("\x01", 32*1024),
		} {
			sendLine(t, page, fmt.Sprint(len(text)))
			requireOutput(t, page, fmt.Sprintf("capture ready: %d", len(text)))
			mu.Lock()
			frameSizes = nil
			mu.Unlock()

			_, err := page.Locator(".xterm-helper-textarea").Evaluate(`(element, text) => {
				const clipboardData = new DataTransfer();
				clipboardData.setData('text/plain', text);
				element.dispatchEvent(new ClipboardEvent('paste', { clipboardData, bubbles: true }));
			}`, text)
			requireNoError(t, err)
			requireOutput(t, page, fmt.Sprintf("captured: %d", len(text)))
			captured, err := os.ReadFile(filepath.Join(fixture.directory, "pasted-input"))
			requireNoError(t, err)
			if string(captured) != text {
				t.Fatalf("paste changed bytes: got %d, want %d", len(captured), len(text))
			}

			mu.Lock()
			sizes := slices.Clone(frameSizes)
			mu.Unlock()
			if len(sizes) < 2 {
				t.Fatalf("large paste used only %d frames", len(sizes))
			}
			for _, size := range sizes {
				if size > 64*1024 {
					t.Fatalf("frame exceeded the wrapper limit: %d bytes", size)
				}
			}
			requireStatus(t, page, "Connected")
		}
	})
}

func TestTerminalClipboard(t *testing.T) {
	const text = "copied from the terminal: 界 🙂"
	script := fmt.Sprintf(`#!/bin/sh
		while IFS= read -r line; do
			if [ "$line" = query ]; then
				stty raw -echo
				printf '\033]52;c;?\007'
				head -c 8 > clipboard-response
				stty sane
				printf 'query completed\n'
				continue
			fi
			printf '\033]52;c;%s\007'
			printf 'copy completed: %%s\n' "$line"
		done
	`, base64.StdEncoding.EncodeToString([]byte(text)))
	testConnections(t, script, func(t *testing.T, fixture browserTest) {
		page := fixture.page
		requireNoError(t, fixture.context.GrantPermissions([]string{
			"clipboard-read", "clipboard-write",
		}))
		openTerminal(t, page, fixture.url)
		expect := playwright.NewPlaywrightAssertions()

		sendLine(t, page, "initial")
		requireOutput(t, page, "copy completed: initial")
		copied, err := page.Evaluate("() => navigator.clipboard.readText()")
		requireNoError(t, err)
		if copied != text {
			t.Fatalf("clipboard = %q, want %q", copied, text)
		}

		sendLine(t, page, "query")
		requireOutput(t, page, "query completed")
		response, err := os.ReadFile(filepath.Join(fixture.directory, "clipboard-response"))
		requireNoError(t, err)
		if string(response) != "\x1b]52;c;\x07" {
			t.Fatalf("clipboard query response = %q, want empty clipboard text", response)
		}

		// Browser restrictions must leave rendering and subsequent input working.
		for _, mode := range []string{"denied", "unavailable"} {
			t.Run(mode, func(t *testing.T) {
				t.Cleanup(func() {
					_, err := page.Evaluate(`() => {
						delete navigator.clipboard;
						delete navigator.clipboard.writeText;
					}`)
					requireNoError(t, err)
				})
				_, err := page.Evaluate(`mode => {
					if (mode === 'denied') {
						Object.defineProperty(navigator.clipboard, 'writeText', {
							configurable: true,
							value: () => Promise.reject(
								new DOMException('Denied', 'NotAllowedError')
							),
						});
					} else {
						Object.defineProperty(navigator, 'clipboard', {
							configurable: true, value: undefined,
						});
					}
				}`, mode)
				requireNoError(t, err)
				sendLine(t, page, mode)
				requireOutput(t, page, "copy completed: "+mode)
				requireNoError(t, expect.Locator(page.Locator("#notice")).ToContainText("Could not copy to clipboard"))
				requireStatus(t, page, "Connected")

				_, err = page.Evaluate(`async () => {
					delete navigator.clipboard;
					delete navigator.clipboard.writeText;
					await navigator.clipboard.writeText('before retry');
				}`)
				requireNoError(t, err)
				sendLine(t, page, mode+"-retry")
				requireOutput(t, page, "copy completed: "+mode+"-retry")
				requireNoError(t, expect.Locator(page.Locator("#notice")).ToBeEmpty())
				copied, err := page.Evaluate("() => navigator.clipboard.readText()")
				requireNoError(t, err)
				if copied != text {
					t.Fatalf("clipboard after retry = %q, want %q", copied, text)
				}
			})
		}
	})
}
