package web_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
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

func TestTerminalSharedTabs(t *testing.T) {
	testConnections(t, "", func(t *testing.T, fixture browserTest) {
		openTerminal(t, fixture.page, fixture.url)
		sendLine(t, fixture.page, "first-tab")
		requireOutput(t, fixture.page, "received: first-tab")
		second, err := fixture.context.NewPage()
		requireNoError(t, err)

		err = second.AddInitScript(playwright.Script{
			Content: playwright.String(`
				window.resizeMessages = [];
				const send = WebSocket.prototype.send;
				WebSocket.prototype.send = function(data) {
				  const message = JSON.parse(data);
				  if (message.type === 'resize') {
				    window.resizeMessages.push(message);
				  }
				  return send.call(this, data);
				};
			`),
		})
		requireNoError(t, err)
		_, err = second.Goto(fixture.url)
		requireNoError(t, err)
		requireStatus(t, second, "Connected")
		sendLine(t, fixture.page, "both-attached")
		requireOutput(t, fixture.page, "received: both-attached")
		requireOutput(t, second, "received: both-attached")
		sendLine(t, second, "second-tab")
		requireOutput(t, fixture.page, "received: second-tab")
		requireOutput(t, second, "received: second-tab")

		// Headless Chromium reports every page focused; provide the background
		// focus state while keeping real layout, sockets, and harness resizing.
		_, err = second.Evaluate(`
			document.hasFocus = () => false;
			window.resizeMessages = [];
		`)
		requireNoError(t, err)
		requireNoError(t, second.SetViewportSize(900, 600))
		requireTerminalFits(t, second)
		messages, err := second.Evaluate("window.resizeMessages.length")
		requireNoError(t, err)
		if messages != 0 {
			t.Fatal("background tab resized the shared terminal", messages)
		}
		_, err = second.Evaluate(`
			document.hasFocus = () => true;
			window.dispatchEvent(new Event('focus'));
		`)
		requireNoError(t, err)
		handle, err := second.WaitForFunction("window.resizeMessages.length > 0", nil)
		requireNoError(t, err)
		requireNoError(t, handle.Dispose())
		requireNoError(t, fixture.page.Close())
		requireStatus(t, second, "Connected")
		requireOutput(t, second, "received: first-tab")
		sendLine(t, second, "remaining-tab")
		requireOutput(t, second, "received: remaining-tab")
	})
}

func TestTerminalKittyImages(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 128, 64))
	random := rand.New(rand.NewPCG(1, 2))
	for y := range 64 {
		for x := range 128 {
			value := random.Uint32()
			pixel := color.NRGBA{R: uint8(value), G: uint8(value >> 8), B: uint8(value >> 16), A: 255}
			if x < 64 {
				pixel = color.NRGBA{G: 255, B: 255, A: 255}
			}
			pixels.SetNRGBA(x, y, pixel)
		}
	}
	var encoded bytes.Buffer
	requireNoError(t, png.Encode(&encoded, pixels))
	payload := base64.StdEncoding.EncodeToString(encoded.Bytes())
	if len(payload) <= 4096 {
		t.Fatal("image fixture must require multiple Kitty chunks")
	}
	// Match Codex's inline PNG framing, including quiet replies and cursor policy.
	var frames string
	for start := 0; start < len(payload); start += 4096 {
		end := min(start+4096, len(payload))
		more := 0
		if end < len(payload) {
			more = 1
		}
		control := fmt.Sprintf("m=%d", more)
		if start == 0 {
			control = "a=T,t=d,f=100,c=16,r=4,q=2,i=42,C=1," + control
		}
		frames += fmt.Sprintf("\x1b_G%s;%s\x1b\\", control, payload[start:end])
	}
	// A reload can repeat the capability reply before the next shell command.
	script := fmt.Sprintf(`#!/bin/sh
		while IFS= read -r line; do
			case "$line" in
				*query)
					line=query
					stty raw -echo
					printf '\033_Ga=q,i=31,t=d,f=24,s=1,v=1;AAAA\033\\'
					head -c 12 > image-response
					stty sane
					;;
				*draw)
					line=draw
					printf '\0337\033[5;4H'
					printf '%%s' '%s'
					printf '\0338'
					;;
				*delete)
					line=delete
					printf '\033_Ga=d,d=I,i=42,q=2;\033\\'
					;;
			esac
			printf 'image command: %%s\n' "$line"
		done
	`, frames)
	testConnections(t, script, func(t *testing.T, fixture browserTest) {
		page := fixture.page
		page.OnPageError(func(err error) { t.Errorf("browser error: %v", err) })
		trackConnections(t, page)
		openTerminal(t, page, fixture.url)
		sendLine(t, page, "query")
		requireOutput(t, page, "image command: query")
		response, err := os.ReadFile(filepath.Join(fixture.directory, "image-response"))
		requireNoError(t, err)
		if string(response) != "\x1b_Gi=31;OK\x1b\\" {
			t.Fatalf("Kitty query response = %q", response)
		}

		requireImage := func(want bool) {
			t.Helper()
			// Parsing completes before the image canvas necessarily paints.
			_, err := page.Evaluate(`() => new Promise(resolve => {
				requestAnimationFrame(() => requestAnimationFrame(resolve));
			})`)
			requireNoError(t, err)
			screenshot, err := page.Locator("#terminal").Screenshot()
			requireNoError(t, err)
			painted, err := png.Decode(bytes.NewReader(screenshot))
			requireNoError(t, err)
			cyan := 0
			for y := painted.Bounds().Min.Y; y < painted.Bounds().Max.Y; y++ {
				for x := painted.Bounds().Min.X; x < painted.Bounds().Max.X; x++ {
					r, g, b, _ := painted.At(x, y).RGBA()
					if r < 0x1000 && g > 0xf000 && b > 0xf000 {
						cyan++
					}
				}
			}
			if (want && cyan < 1000) || (!want && cyan != 0) {
				t.Fatalf("image visible = %v, cyan pixels = %d", want, cyan)
			}
		}
		sendLine(t, page, "draw")
		requireOutput(t, page, "image command: draw")
		requireImage(true)
		requireNoError(t, page.SetViewportSize(900, 600))
		requireTerminalFits(t, page)
		requireImage(true)

		disconnect(t, page)
		handle, err := page.WaitForFunction("() => window.connections.length === 2", nil)
		requireNoError(t, err)
		requireNoError(t, handle.Dispose())
		requireStatus(t, page, "Connected")
		requireImage(true)
		_, err = page.Reload()
		requireNoError(t, err)
		requireStatus(t, page, "Connected")
		requireOutput(t, page, "image command: draw")
		requireImage(true)
		sendLine(t, page, "delete")
		requireOutput(t, page, "image command: delete")
		requireImage(false)
	})
}
