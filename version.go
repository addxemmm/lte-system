package ltesystem

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embeddedVersion string

// Version returns the release version embedded from the repository's VERSION file.
func Version() string {
	return strings.TrimSpace(embeddedVersion)
}
