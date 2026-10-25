package server

import (
	"embed"
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
