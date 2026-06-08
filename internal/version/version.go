package version

import (
	_ "embed"
	"strings"
)

// Version is the current Ahh version.
//
//go:embed version.txt
var Version string

func init() {
	Version = strings.TrimSpace(Version)
}
