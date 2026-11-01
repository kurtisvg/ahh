package web_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestProjectWorkflowAfterRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	binary := buildBrowserBinary(t)
	for _, mode := range []string{"direct", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			bin := filepath.Join(directory, "bin")
			requireNoError(t, os.Mkdir(bin, 0o700))
			script := `#!/bin/sh
			printf 'ready cwd=%s\n' "$PWD"

			while IFS= read -r line; do
			  case "$line" in
			    write\ *)
			      printf '%s\n' "${line#write }" > note.txt
			      printf 'wrote %s\n' "${line#write }"
			      ;;
			    *)
			      printf 'received: %s\n' "$line"
			      ;;
			  esac
			done
			`
			requireNoError(t, os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o700))
			start := func() (string, func()) {
				basePath := "/"
				if mode == "proxy" {
					basePath = browserProxyPath
				}
				url, stop := startBrowserServer(
					t, binary, directory, basePath,
					"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				)
				if mode == "proxy" {
					url = startBrowserProxy(t, url)
				}
				return strings.TrimRight(url, "/"), stop
			}
			url, stop := start()
			page, err := newBrowserContext(t).NewPage()
			requireNoError(t, err)
			a := createBrowserProject(t, page, url, "Alpha")
			b := createBrowserProject(t, page, url, "Beta")
			expect := playwright.NewPlaywrightAssertions()
			for _, p := range []struct{ id, value string }{{a.ID, "alpha"}, {b.ID, "beta"}} {
				openTerminal(t, page, url+"/p/"+p.id+"/c")
				sendLine(t, page, "write "+p.value)
				requireOutput(t, page, "wrote "+p.value)
			}
			openTerminal(t, page, url+"/p/"+a.ID+"/c")
			requireOutput(t, page, "wrote alpha")
			requireNoError(t, expect.Locator(page.Locator(".xterm-accessibility-tree")).Not().ToContainText("wrote beta"))
			stop()
			url, _ = start()
			_, err = page.Goto(url + "/p/" + a.ID + "/c")
			requireNoError(t, err)
			requireStatus(t, page, "Stopped")
			for _, p := range []struct{ id, value string }{{a.ID, "alpha"}, {b.ID, "beta"}} {
				data, err := os.ReadFile(filepath.Join(directory, "workspace", "projects", p.id, "repo", "note.txt"))
				requireNoError(t, err)
				if strings.TrimSpace(string(data)) != p.value {
					t.Fatalf("lost Project files: %q", data)
				}
			}
			response, err := page.Request().Get(url + "/api/projects/" + b.ID + "/conversations/default/terminal")
			requireNoError(t, err)
			var state struct {
				State string `json:"state"`
			}
			requireNoError(t, response.JSON(&state))
			if state.State != "stopped" {
				t.Fatal("restart launched Beta")
			}
			openTerminal(t, page, url+"/p/"+a.ID+"/c")
			openTerminal(t, page, url+"/p/"+b.ID+"/c")
			sendLine(t, page, "write beta-after-restart")
			requireOutput(t, page, "wrote beta-after-restart")
			_, err = page.Goto(url + "/p/" + a.ID)
			requireNoError(t, err)
			requireNoError(t, page.Locator("#delete-project").Click())
			requireNoError(t, page.GetByRole("dialog").GetByRole("button", playwright.LocatorGetByRoleOptions{
				Name: "Delete Project", Exact: playwright.Bool(true),
			}).Click())
			requireNoError(t, expect.Page(page).ToHaveURL(url+"/p/"+b.ID))
			requireNoError(t, expect.Locator(page.Locator(".sidebar-heading > h2")).ToBeFocused())
			requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
				Name: "Open conversations", Exact: playwright.Bool(true),
			}).Click())
			requireStatus(t, page, "Connected")
			requireOutput(t, page, "wrote beta-after-restart")
			sendLine(t, page, "still-running")
			requireOutput(t, page, "received: still-running")
			if _, err := os.Stat(filepath.Join(directory, "workspace", "projects", a.ID)); !os.IsNotExist(err) {
				t.Fatal("deleted files remain", err)
			}
			data, err := os.ReadFile(filepath.Join(directory, "workspace", "projects", b.ID, "repo", "note.txt"))
			requireNoError(t, err)
			if strings.TrimSpace(string(data)) != "beta-after-restart" {
				t.Fatalf("Beta changed: %q", data)
			}
		})
	}
}
