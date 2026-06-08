package web

import "embed"

//go:embed index.html styles.css
var Files embed.FS
