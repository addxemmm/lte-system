package ltesystem_test

import (
	_ "embed"
	"strings"
	"testing"

	ltesystem "github.com/addxemmm/lte-system"
)

//go:embed VERSION
var versionFixture string

func TestVersionMatchesTrimmedVersionFile(t *testing.T) {
	want := strings.TrimSpace(versionFixture)
	if want == "" {
		t.Fatal("VERSION must not be empty")
	}
	if got := ltesystem.Version(); got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}
