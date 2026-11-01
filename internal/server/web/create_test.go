package web_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestCreateWithoutHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	binary := buildBrowserBinary(t)
	git, err := exec.LookPath("git")
	requireNoError(t, err)
	for _, mode := range []string{"direct", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			bin := filepath.Join(directory, "bin")
			requireNoError(t, os.Mkdir(bin, 0o700))
			basePath := "/"
			if mode == "proxy" {
				basePath = browserProxyPath
			}
			url, _ := startBrowserServer(t, binary, directory, basePath, "PATH="+bin)
			if mode == "proxy" {
				url = startBrowserProxy(t, url)
			}
			page, err := newBrowserContext(t).NewPage()
			requireNoError(t, err)
			_, err = page.Goto(strings.TrimRight(url, "/") + "/p")
			requireNoError(t, err)
			expect := playwright.NewPlaywrightAssertions()
			requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
				Name:  "Add Project",
				Exact: playwright.Bool(true),
			}).Click())
			requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
				Name:  "Cancel",
				Exact: playwright.Bool(true),
			}).Click())
			requireNoError(t,
				expect.Locator(page.Locator("#detail")).ToContainText("No Projects yet"),
			)
			requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
				Name:  "Add Project",
				Exact: playwright.Bool(true),
			}).Click())
			requireNoError(t, page.GetByLabel("Display name", playwright.PageGetByLabelOptions{
				Exact: playwright.Bool(true),
			}).Fill("alpha"))
			requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
				Name:  "Create Project",
				Exact: playwright.Bool(true),
			}).Click())
			requireNoError(t,
				expect.Locator(page.Locator("dialog[open] [role=alert]")).ToContainText("Could not create project"),
			)
			requireNoError(t, expect.Page(page).ToHaveURL(strings.TrimRight(url, "/")+"/p"))
			response, err := page.Request().Get(strings.TrimRight(url, "/") + "/api/projects")
			requireNoError(t, err)
			var projects []struct{ ID string }
			requireNoError(t, response.JSON(&projects))
			if len(projects) != 0 {
				t.Fatalf("created %d projects, want no failed attempts", len(projects))
			}
			requireNoError(t, os.Symlink(git, filepath.Join(bin, "git")))
			requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
				Name:  "Create Project",
				Exact: playwright.Bool(true),
			}).Click())
			requireNoError(t, expect.Page(page).ToHaveURL(strings.TrimRight(url, "/")+"/p/alpha"))
			requireNoError(t, expect.Locator(page.Locator("dialog[open]")).ToHaveCount(0))
			_, err = page.Reload()
			requireNoError(t, err)
			requireNoError(t,
				expect.Locator(page.Locator("#project-name")).ToHaveValue("alpha"),
			)
		})
	}
}

func TestCreateWaitsForRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	directory := t.TempDir()
	bin := filepath.Join(directory, "bin")
	requireNoError(t, os.Mkdir(bin, 0o700))
	git, err := exec.LookPath("git")
	requireNoError(t, err)
	release := filepath.Join(directory, "release")
	script := fmt.Sprintf(`#!/bin/sh
for arg do
  if [ "$arg" = init ]; then
    while [ ! -f %q ]; do /bin/sleep 0.05; done
  fi
done
exec %q "$@"
`, release, git)
	requireNoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700))
	url, _ := startBrowserServer(t, buildBrowserBinary(t), directory, "/", "PATH="+bin)
	page, err := newBrowserContext(t).NewPage()
	requireNoError(t, err)
	_, err = page.Goto(strings.TrimRight(url, "/") + "/p")
	requireNoError(t, err)
	expect := playwright.NewPlaywrightAssertions()
	requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Add Project",
		Exact: playwright.Bool(true),
	}).Click())
	requireNoError(t, page.GetByLabel("Display name", playwright.PageGetByLabelOptions{
		Exact: playwright.Bool(true),
	}).Fill("slow-setup"))
	requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Create Project",
		Exact: playwright.Bool(true),
	}).Click())
	requireNoError(t,
		expect.Locator(page.Locator("dialog[open] [role=status]")).ToContainText("Creating project…"),
	)
	requireNoError(t,
		expect.Locator(page.Locator("dialog[open] [type=submit]")).ToBeDisabled(),
	)
	requireNoError(t, expect.Page(page).ToHaveURL(strings.TrimRight(url, "/")+"/p"))
	requireNoError(t, os.WriteFile(release, nil, 0o600))
	requireNoError(t,
		expect.Locator(page.Locator("#project-name")).ToHaveValue("slow-setup"),
	)
	requireNoError(t, expect.Locator(page.Locator("dialog[open]")).ToHaveCount(0))
}

func TestCreationNameAndID(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	url, _ := startBrowserServer(t, buildBrowserBinary(t), t.TempDir(), "/")
	page, err := newBrowserContext(t).NewPage()
	requireNoError(t, err)
	_, err = page.Goto(url + "/p")
	requireNoError(t, err)
	expect := playwright.NewPlaywrightAssertions()
	add := page.GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Add Project", Exact: playwright.Bool(true),
	})
	requireNoError(t, add.Click())
	name := page.GetByLabel("Display name", playwright.PageGetByLabelOptions{
		Exact: playwright.Bool(true),
	})
	id := page.GetByLabel("Project ID", playwright.PageGetByLabelOptions{
		Exact: playwright.Bool(true),
	})
	create := page.GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Create Project", Exact: playwright.Bool(true),
	})
	requireNoError(t, expect.Locator(create).ToBeDisabled())
	requireNoError(t, name.Fill("  Café & Notes!  "))
	requireNoError(t, expect.Locator(id).ToHaveValue("cafe-notes"))
	requireNoError(t, name.Fill("Updated Project"))
	requireNoError(t, expect.Locator(id).ToHaveValue("updated-project"))
	requireNoError(t, id.Fill("Invalid ID"))
	requireNoError(t, expect.Locator(id).ToHaveAttribute("aria-invalid", "true"))
	requireNoError(t, expect.Locator(page.Locator("#create-project-id-error")).
		ToContainText("lowercase letters or digits"))
	requireNoError(t, expect.Locator(create).ToBeDisabled())
	requireNoError(t, id.Fill(strings.Repeat("a", 65)))
	requireNoError(t, expect.Locator(page.Locator("#create-project-id-error")).
		ToContainText("64 characters"))
	requireNoError(t, id.Fill("custom-id"))
	requireNoError(t, name.Fill(" A Different Display Name "))
	requireNoError(t, expect.Locator(id).ToHaveValue("custom-id"))
	requireNoError(t, expect.Locator(id).ToHaveAttribute("aria-invalid", "false"))
	requireNoError(t, expect.Locator(create).ToBeEnabled())
	requireNoError(t, create.Click())
	requireNoError(t, expect.Page(page).ToHaveURL(url+"/p/custom-id"))
	requireNoError(t, expect.Locator(page.Locator("#project-name")).
		ToHaveValue("A Different Display Name"))
	_, err = page.Reload()
	requireNoError(t, err)
	requireNoError(t, expect.Locator(page.Locator("#project-name")).
		ToHaveValue("A Different Display Name"))
	requireNoError(t, add.Click())
	requireNoError(t, name.Fill("Custom ID"))
	requireNoError(t, create.Click())
	requireNoError(t, expect.Locator(page.Locator("#create-project-id-error")).
		ToContainText("ID is taken"))
	requireNoError(t, expect.Locator(create).ToBeDisabled())
	requireNoError(t, id.Fill("another-id"))
	requireNoError(t, expect.Locator(create).ToBeEnabled())
}
