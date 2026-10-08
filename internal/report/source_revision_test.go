package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteVerificationReport_SourceRevisionSetAndAbsent(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv("RV_SOURCE_REVISION", "0123456789abcdef")
		body := writeSourceRevisionReport(t, "")
		if !strings.Contains(body, `"source_revision": "0123456789abcdef"`) {
			t.Fatalf("expected source_revision in report, got %s", body)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Setenv("RV_SOURCE_REVISION", "should-be-cleared")
		if err := os.Unsetenv("RV_SOURCE_REVISION"); err != nil {
			t.Fatalf("unset RV_SOURCE_REVISION: %v", err)
		}
		body := writeSourceRevisionReport(t, "")
		if strings.Contains(body, "source_revision") {
			t.Fatalf("expected source_revision to be omitted, got %s", body)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv("RV_SOURCE_REVISION", "   ")
		body := writeSourceRevisionReport(t, "")
		if strings.Contains(body, "source_revision") {
			t.Fatalf("expected empty RV_SOURCE_REVISION to omit source_revision, got %s", body)
		}
	})

	t.Run("keeps existing", func(t *testing.T) {
		t.Setenv("RV_SOURCE_REVISION", "from-env")
		body := writeSourceRevisionReport(t, "from-report")
		if !strings.Contains(body, `"source_revision": "from-report"`) {
			t.Fatalf("expected existing source_revision to stay, got %s", body)
		}
		if strings.Contains(body, "from-env") {
			t.Fatalf("RV_SOURCE_REVISION must leave an existing source revision unchanged, got %s", body)
		}
	})
}

func writeSourceRevisionReport(t *testing.T, revision string) string {
	t.Helper()
	result := NewVerificationReport(nil)
	result.Summary.SourceRevision = revision
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteVerificationReport(path, result); err != nil {
		t.Fatalf("write report: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	return string(data)
}
