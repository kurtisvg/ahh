package web_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestDeleteConfirmationAndRetry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	testConnections(t, "", func(t *testing.T, f browserTest) {
		page := f.page
		a := createBrowserProject(t, page, f.baseURL, "Alpha")
		b := createBrowserProject(t, page, f.baseURL, "Beta")
		_, err := page.Goto(f.baseURL + "/p/" + a.ID)
		requireNoError(t, err)
		expect := playwright.NewPlaywrightAssertions()
		dialog := page.GetByRole("dialog", playwright.PageGetByRoleOptions{
			Name: "Delete Project", Exact: playwright.Bool(true),
		})
		cancel := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{
			Name: "Cancel", Exact: playwright.Bool(true),
		})
		confirm := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{
			Name: "Delete Project", Exact: playwright.Bool(true),
		})
		open := func() { t.Helper(); requireNoError(t, page.Locator("#delete-project").Click()) }
		open()
		cancelBox, err := cancel.BoundingBox()
		requireNoError(t, err)
		deleteBox, err := confirm.BoundingBox()
		requireNoError(t, err)
		if cancelBox.Height != deleteBox.Height || cancelBox.Y != deleteBox.Y {
			t.Fatal("dialog actions must align at equal height")
		}
		requireNoError(t, expect.Locator(cancel).ToBeFocused())
		requireNoError(t, cancel.Click())
		requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue(a.Name))
		// Prevent removing the repository so the failure remains retryable.
		parent := filepath.Dir(filepath.Dir(f.directory))
		container := filepath.Join(parent, a.ID)
		requireNoError(t, os.Chmod(container, 0o500))
		t.Cleanup(func() { _ = os.Chmod(container, 0o700) })
		open()
		requireNoError(t, confirm.Click())
		retry := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{
			Name: "Retry deletion", Exact: playwright.Bool(true),
		})
		requireNoError(t, expect.Locator(retry).ToBeEnabled())
		requireNoError(t, expect.Locator(page.Locator("#detail")).ToContainText("Deletion could not finish"))
		_, err = page.Reload()
		requireNoError(t, err)
		requireNoError(t, expect.Locator(page.Locator("#delete-project")).ToHaveText("Retry deletion"))
		requireNoError(t, os.Chmod(container, 0o700))
		open()
		requireNoError(t, confirm.Click())
		requireNoError(t, expect.Page(page).ToHaveURL(f.baseURL+"/p/"+b.ID))
		requireNoError(t, expect.Locator(page.Locator("#project-name")).ToHaveValue(b.Name))
		if _, err := os.Stat(container); !os.IsNotExist(err) {
			t.Fatal("Project files remain", err)
		}
	})
}
