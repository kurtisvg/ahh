package web_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestProjectLayoutWithLongNames(t *testing.T) {
	testConnections(t, "", func(t *testing.T, f browserTest) {
		page := f.page
		project := createBrowserProject(t, page, f.baseURL, "long-name")
		response, err := page.Request().Patch(f.baseURL+"/api/projects/"+project.ID, playwright.APIRequestContextPatchOptions{
			Data: map[string]any{
				"name":    strings.Repeat("W", 100),
				"version": project.Version,
			},
		})
		requireNoError(t, err)
		if response.Status() != 200 {
			t.Fatal(response.Status())
		}
		bookmark := f.baseURL + "/p/" + project.ID
		expect := playwright.NewPlaywrightAssertions()
		for _, width := range []int{320, 390, 768} {
			t.Run(strconv.Itoa(width), func(t *testing.T) {
				requireNoError(t, page.SetViewportSize(width, 740))
				_, err := page.Goto(bookmark)
				requireNoError(t, err)
				requireNoError(t, expect.Locator(page.Locator("#project-name")).ToBeVisible())
				requireNoHorizontalOverflow(t, page, "#detail")
				requireNoError(t, page.Locator("#delete-project").Click())
				requireNoHorizontalOverflow(t, page, "dialog[open]")
				requireNoError(t, page.Keyboard().Press("Escape"))
				requireNoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{
					Name: "Add Project", Exact: playwright.Bool(true),
				}).Click())
				requireNoHorizontalOverflow(t, page, "dialog[open]")
				requireNoError(t, page.Keyboard().Press("Escape"))
				_, err = page.Goto(bookmark + "/c")
				requireNoError(t, err)
				requireStatus(t, page, "Stopped")
				requireNoHorizontalOverflow(t, page, "#detail")
				requireNoError(t, expect.Locator(page.Locator("#terminal")).ToBeHidden())
				requireNoError(t, expect.Locator(page.Locator(".conversation-start #start")).ToBeVisible())
			})
		}
	})
}

func requireNoHorizontalOverflow(t *testing.T, page playwright.Page, selector string) {
	t.Helper()
	overflow, err := page.Locator(selector).Evaluate(`element =>
		element.scrollWidth > element.clientWidth + 1
		|| document.documentElement.scrollWidth > innerWidth`, nil)
	requireNoError(t, err)
	if overflow == true {
		t.Fatalf("%s clips content horizontally", selector)
	}
}
