package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageBase(t *testing.T) {
	for _, base := range []string{"/", "/proxy/40443/"} {
		page, err := servePage(base)
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range []string{"/", "/terminal", "/p/id", "/p/id/c"} {
			t.Run(base+route, func(t *testing.T) {
				response := httptest.NewRecorder()
				page.ServeHTTP(response, httptest.NewRequest("GET", route, nil))
				if !strings.Contains(response.Body.String(), `<base href="`+base+`">`) {
					t.Fatal(response.Body.String())
				}
			})
		}
	}
}
