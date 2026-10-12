// Package version provides the embedded application version.
package version

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var versionText string

// Version is the application version embedded at build time.
var Version = strings.TrimSpace(versionText)
