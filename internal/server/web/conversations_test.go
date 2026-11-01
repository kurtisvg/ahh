package web_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/mxschmitt/playwright-go"
)

func createBrowserProject(t *testing.T, page playwright.Page, url, name string) project.Project {
	t.Helper()
	endpoint := strings.TrimRight(url, "/") + "/api/projects"
	response, err := page.Request().Post(endpoint, playwright.APIRequestContextPostOptions{
		Data: map[string]string{
			"id":   strings.ToLower(name),
			"name": name,
		},
	})
	requireNoError(t, err)
	if response.Status() != http.StatusCreated {
		t.Fatal(response.Status())
	}
	var p project.Project
	requireNoError(t, response.JSON(&p))
	return p
}

func TestConversationNavigation(t *testing.T) {
	testConnections(t, "", func(t *testing.T, f browserTest) {
		page := f.page
		a := createBrowserProject(t, page, f.baseURL, "Alpha")
		b := createBrowserProject(t, page, f.baseURL, "Beta")
		base := f.baseURL
		bookmark := base + "/p/" + a.ID + "/c"
		_, err := page.Goto(base + "/c")
		requireNoError(t, err)
		expect := playwright.NewPlaywrightAssertions()
		requireNoError(t, expect.Page(page).ToHaveURL(bookmark))
		requireStatus(t, page, "Stopped")
		requireNoError(t, expect.Page(page).ToHaveTitle("Alpha · Ahh"))
		requireNoError(t, expect.Locator(page.Locator("#terminal")).ToBeHidden())
		requireNoError(t, expect.Locator(page.Locator(".conversation-start #start")).ToBeVisible())
		picker, err := page.Locator("#project-select").BoundingBox()
		requireNoError(t, err)
		heading, err := page.Locator(".conversation-list-heading").BoundingBox()
		requireNoError(t, err)
		if picker.Y+picker.Height > heading.Y {
			t.Fatal("project picker should precede conversations")
		}
		requireNoError(t, page.Locator("#start").Click())
		requireStatus(t, page, "Connected")
		requireNoError(t, expect.Locator(page.Locator("#terminal")).ToBeVisible())
		requireNoError(t, expect.Locator(page.Locator(".conversation-start")).ToBeHidden())
		requireNoError(t, expect.Locator(page.Locator("#status")).ToHaveAttribute("data-state", "connected"))
		sendLine(t, page, "from-alpha")
		requireOutput(t, page, "received: from-alpha")
		_, err = page.Locator("#project-select").SelectOption(playwright.SelectOptionValues{
			Values: &[]string{b.ID},
		})
		requireNoError(t, err)
		requireStatus(t, page, "Stopped")
		requireNoError(t, page.Locator("#start").Click())
		requireStatus(t, page, "Connected")
		sendLine(t, page, "from-beta")
		requireOutput(t, page, "received: from-beta")
		requireNoError(t, expect.Locator(page.Locator(".xterm-accessibility-tree")).Not().ToContainText("from-alpha"))
		_, err = page.Locator("#project-select").SelectOption(playwright.SelectOptionValues{
			Values: &[]string{a.ID},
		})
		requireNoError(t, err)
		requireStatus(t, page, "Connected")
		requireOutput(t, page, "received: from-alpha")
		requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
			Name:  "Settings",
			Exact: playwright.Bool(true),
		}).Click())
		requireNoError(t, expect.Page(page).ToHaveURL(base+"/p/"+a.ID))
		requireNoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{
			Name:  "Conversations",
			Exact: playwright.Bool(true),
		}).Click())
		requireStatus(t, page, "Connected")
		requireOutput(t, page, "received: from-alpha")
		_, err = page.Reload()
		requireNoError(t, err)
		requireStatus(t, page, "Connected")
		requireNoError(t, page.SetViewportSize(390, 700))
		requireTerminalFits(t, page)
	})
}
