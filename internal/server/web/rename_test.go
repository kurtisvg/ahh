package web_test

import (
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestRenameDraftAndConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("browser tests are excluded in short mode")
	}
	directory := t.TempDir()
	setupTestDB(t, directory)
	url, _ := startBrowserServer(t, buildBrowserBinary(t), directory, "/")
	page, err := newBrowserContext(t).NewPage()
	requireNoError(t, err)
	_, err = page.Goto(url + "/p/a")
	requireNoError(t, err)
	name := page.GetByLabel("Project name", playwright.PageGetByLabelOptions{
		Exact: playwright.Bool(true),
	})
	expect := playwright.NewPlaywrightAssertions()
	save := page.GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Save changes",
		Exact: playwright.Bool(true),
	})
	discard := page.GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Discard changes",
		Exact: playwright.Bool(true),
	})
	requireNoError(t, expect.Locator(save).ToBeDisabled())
	requireNoError(t, expect.Locator(discard).ToBeDisabled())
	requireNoError(t, name.Fill("Renamed"))
	requireNoError(t, expect.Locator(save).ToBeEnabled())
	requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
		Name:  "Beta · failed",
		Exact: playwright.Bool(true),
	}).Click())
	requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
		Name:  "Alpha · ready",
		Exact: playwright.Bool(true),
	}).Click())
	requireNoError(t, expect.Locator(name).ToHaveValue("Renamed"))
	response, err := page.Request().Patch(
		strings.TrimRight(url, "/")+"/api/projects/a",
		playwright.APIRequestContextPatchOptions{
			Data: map[string]any{
				"name":    "Other tab",
				"version": 1,
			},
		},
	)
	requireNoError(t, err)
	if response.Status() != 200 {
		t.Fatal(response.Status())
	}
	requireNoError(t, save.Click())
	requireNoError(t,
		expect.Locator(page.Locator("#detail form [role=alert]")).
			ToContainText("Current saved name: Other tab"),
	)
	requireNoError(t, expect.Locator(name).ToHaveValue("Renamed"))
	requireNoError(t, discard.Click())
	requireNoError(t, expect.Locator(name).ToHaveValue("Other tab"))
	requireNoError(t, expect.Locator(save).ToBeDisabled())
	requireNoError(t, name.Fill("Saved"))
	requireNoError(t, save.Click())
	requireNoError(t, expect.Locator(page.Locator("#project-form input")).ToHaveValue("Saved"))
	requireNoError(t, expect.Locator(page.Locator("#project-form [role=status]")).ToHaveText("Saved"))
	requireNoError(t, expect.Locator(page.Locator("#project-form [role=alert]")).ToBeEmpty())
	requireNoError(t, expect.Locator(save).ToBeDisabled())
	requireNoError(t, expect.Locator(discard).ToBeDisabled())
}
