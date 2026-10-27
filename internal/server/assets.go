package server

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:generate npm --prefix web run build

//go:embed web/dist
var assets embed.FS

func serveAssets() http.Handler {
	files, err := fs.Sub(assets, "web/dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(files))
}

// servePage sets a relative base so a stripped proxy prefix is preserved even
// when the browser opens a nested route directly. Only known UI routes use it.
func servePage(w http.ResponseWriter, r *http.Request) {
	page, err := assets.ReadFile("web/dist/index.html")
	if err != nil {
		http.Error(w, "UI unavailable", http.StatusServiceUnavailable)
		return
	}
	depth := strings.Count(strings.Trim(r.URL.Path, "/"), "/")
	base := "./"
	if depth > 0 {
		base = strings.Repeat("../", depth)
	}
	page = bytes.Replace(page, []byte("<head>"), []byte(`<head><base href="`+base+`">`), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}
