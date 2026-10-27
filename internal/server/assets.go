package server

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"net/http"
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

// servePage prepares HTML once; all page routes share the resulting handler.
func servePage(basePath string) (http.Handler, error) {
	page, err := assets.ReadFile("web/dist/index.html")
	if err != nil {
		return nil, err
	}
	baseTag := `<base href="` + html.EscapeString(basePath) + `">`
	page = bytes.Replace(page, []byte("<head>"), []byte("<head>"+baseTag), 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(page)
	}), nil
}
