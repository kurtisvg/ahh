package web

import "embed"

// Files contains the embedded browser shell assets.
//
//go:embed index.html styles.css
var Files embed.FS
