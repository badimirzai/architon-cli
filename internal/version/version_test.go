package version

import (
	"strings"
	"testing"
)

func TestGetUsesLinkTimeVersion(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })

	Version = "v0.15.0"
	if got := Get().Version; got != "v0.15.0" {
		t.Fatalf("Get().Version = %q, want v0.15.0", got)
	}
	if line := Line(); !strings.HasPrefix(line, "rv version: v0.15.0") {
		t.Fatalf("Line() = %q, want prefix %q", line, "rv version: v0.15.0")
	}
}

func TestGetIgnoresBlankLinkTimeVersion(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })

	Version = "   "
	if got := Get().Version; strings.TrimSpace(got) == "" {
		t.Fatal("Get().Version is blank, want build info or default version")
	}
}
