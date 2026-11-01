package server

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
)

//go:generate npm --prefix web run build

//go:embed web/dist
var assets embed.FS

func serveAssets() http.Handler {
	files, err := fs.Sub(assets, "web/dist")
	if err != nil {
		panic(err)
	}
	return serveAssetFiles(files)
}

func serveAssetFiles(files fs.FS) http.Handler {
	// Embedded files are immutable; hash once when constructing the handler.
	etags := make(map[string]string)
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		etags["/"+name] = fmt.Sprintf(`"%x"`, sha256.Sum256(data))
		return nil
	})
	if err != nil {
		panic(err)
	}
	handler := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stable URLs must revalidate so deployments cannot reuse stale assets.
		w.Header().Set("Cache-Control", "no-cache")
		if etag, ok := etags[path.Clean("/"+r.URL.Path)]; ok {
			w.Header().Set("ETag", etag)
		}
		// FileServer handles conditional requests and ranges using this ETag.
		handler.ServeHTTP(w, r)
	})
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
