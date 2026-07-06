package server

import (
	"embed"
	"net/http"
)

// assetsFS keeps browser-terminal assets local so the wrapper can be tested
// through localhost proxies without CDN access.
//
//go:generate npm --prefix web install
//go:generate npm --prefix web run build:assets
//go:embed assets/*
var assetsFS embed.FS

func serveAssets() http.Handler {
	fileServer := http.FileServer(http.FS(assetsFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, r)
	})
}
