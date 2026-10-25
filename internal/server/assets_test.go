package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNestedPageBase(t *testing.T) {
	for path, base := range map[string]string{"/terminal": "./", "/p/id": "../", "/p/id/c": "../../"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			servePage(response, httptest.NewRequest("GET", path, nil))
			if !strings.Contains(response.Body.String(), `<base href="`+base+`">`) {
				t.Fatal(response.Body.String())
			}
		})
	}
}
