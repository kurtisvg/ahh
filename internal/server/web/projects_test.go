package web_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/store/sqlite"
	"github.com/mxschmitt/playwright-go"
)

func setupTestDB(t *testing.T, directory string) {
	t.Helper()
	workspace := filepath.Join(directory, "workspace")
	requireNoError(t, os.MkdirAll(workspace, 0o700))
	path := filepath.Join(workspace, "ahh.db")
	store, err := sqlite.Open(t.Context(), path)
	requireNoError(t, err)
	requireNoError(t, store.Close())
	db, err := sql.Open("sqlite", path)
	requireNoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO projects(id,name,type,state,version) VALUES
	('a','Alpha','git','ready',1), ('b','Beta','git','failed',1)`)
	requireNoError(t, err)
}

func TestProjectPages(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	binary := buildBrowserBinary(t)
	for _, mode := range []string{"direct", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			setupTestDB(t, directory)
			basePath := "/"
			if mode == "proxy" {
				basePath = browserProxyPath
			}
			url, _ := startBrowserServer(t, binary, directory, basePath, "PATH="+directory)
			if mode == "proxy" {
				url = startBrowserProxy(t, url)
			}
			url = strings.TrimRight(url, "/")
			page, err := newBrowserContext(t).NewPage()
			requireNoError(t, err)
			expect := playwright.NewPlaywrightAssertions()
			_, err = page.Goto(url + "/p")
			requireNoError(t, err)
			requireNoError(t, expect.Page(page).ToHaveURL(url+"/p/a"))
			requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue("Alpha"))
			requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Beta · failed", Exact: playwright.Bool(true)}).Focus())
			requireNoError(t, page.Keyboard().Press("Enter"))
			requireNoError(t, expect.Page(page).ToHaveURL(url+"/p/b"))
			requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue("Beta"))
			requireNoError(t, expect.Locator(page.Locator("#detail")).ToBeFocused())
			requireNoError(t, page.Keyboard().Press("Shift+Tab"))
			requireNoError(t, expect.Locator(page.Locator("#sidebar [aria-current=page]")).ToBeFocused())
			_, err = page.Reload()
			requireNoError(t, err)
			requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue("Beta"))
			_, err = page.GoBack()
			requireNoError(t, err)
			requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue("Alpha"))
			requireNoError(t, page.SetViewportSize(390, 700))
			overflow, err := page.Evaluate("() => document.documentElement.scrollWidth > innerWidth")
			requireNoError(t, err)
			if overflow == true {
				t.Fatal("narrow layout overflows")
			}
			_, err = page.Goto(url + "/p/unknown")
			requireNoError(t, err)
			requireNoError(t, expect.Locator(page.Locator("#detail")).ToContainText("Project not found"))
			requireNoError(t, expect.Page(page).ToHaveURL(url+"/p/unknown"))
			requireNoError(t, page.Route("**/api/projects", func(route playwright.Route) {
				requireNoError(t, route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(503)}))
			}))
			_, err = page.Reload()
			requireNoError(t, err)
			requireNoError(t, expect.Locator(page.Locator("#detail")).ToContainText("Could not load Projects"))
			requireNoError(t, page.Unroute("**/api/projects"))
			requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Retry", Exact: playwright.Bool(true)}).Click())
			requireNoError(t, expect.Locator(page.Locator("#detail")).ToContainText("Project not found"))
			requireNoError(t, expect.Locator(page.Locator("#detail")).ToBeFocused())
		})
	}
}
