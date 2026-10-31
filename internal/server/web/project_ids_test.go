package web_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestReadableProjectIDs(t *testing.T) {
	testConnections(t, "", func(t *testing.T, f browserTest) {
		page := f.page
		p := createBrowserProject(t, page, f.baseURL, "my-project")
		bookmark := f.baseURL + "/p/my-project"
		if p.ID != "my-project" {
			t.Fatal(p.ID)
		}
		expect := playwright.NewPlaywrightAssertions()
		_, err := page.Goto(bookmark)
		requireNoError(t, err)
		requireNoError(t, page.Locator("#project-name").Fill("Renamed"))
		requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
			Name: "Save changes", Exact: playwright.Bool(true),
		}).Click())
		requireNoError(t, expect.Page(page).ToHaveTitle("Renamed · Settings · Ahh"))
		requireNoError(t, expect.Page(page).ToHaveURL(bookmark))
		_, err = page.Reload()
		requireNoError(t, err)
		requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue("Renamed"))
		_, err = page.Goto(bookmark + "/c")
		requireNoError(t, err)
		requireStatus(t, page, "Stopped")
		requireNoError(t, expect.Page(page).ToHaveTitle("Renamed · Ahh"))
		requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
			Name: "Settings", Exact: playwright.Bool(true),
		}).Click())
		requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
			Name: "Add Project", Exact: playwright.Bool(true),
		}).Click())
		requireNoError(t, page.GetByLabel("Display name").Fill("My Project"))
		id := page.GetByLabel("Project ID", playwright.PageGetByLabelOptions{Exact: playwright.Bool(true)})
		requireNoError(t, id.Fill("my-project"))
		create := page.GetByRole("button", playwright.PageGetByRoleOptions{
			Name: "Create Project", Exact: playwright.Bool(true),
		})
		requireNoError(t, create.Click())
		requireNoError(t, expect.Locator(page.Locator("#create-project-id-error")).ToContainText("ID is taken"))
		requireNoError(t, expect.Locator(id).ToBeEnabled())
		requireNoError(t, id.Fill("another-project"))
		requireNoError(t, create.Click())
		requireNoError(t, expect.Page(page).ToHaveURL(f.baseURL+"/p/another-project"))
		_, err = page.GoBack()
		requireNoError(t, err)
		requireNoError(t, expect.Page(page).ToHaveURL(bookmark))
		requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue("Renamed"))
	})
}
