package web_test

import (
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
