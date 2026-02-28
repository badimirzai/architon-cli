package report

import (
	"fmt"
	"testing"

	"github.com/badimirzai/robotics-verifier-cli/internal/ir"
)

func TestNewVerificationReport_CapsParseDiagnostics(t *testing.T) {
	parseErrors := make([]string, 0, 22)
	for i := 1; i <= 22; i++ {
		parseErrors = append(parseErrors, fmt.Sprintf("error %d", i))
	}
	parseWarnings := make([]string, 0, 21)
	for i := 1; i <= 21; i++ {
		parseWarnings = append(parseWarnings, fmt.Sprintf("warning %d", i))
	}

	result := NewVerificationReport(&ir.DesignIR{
		Source:        "kicad_bom_csv",
		ParseErrors:   parseErrors,
		ParseWarnings: parseWarnings,
	})

	if result.Summary.ParseErrorsCount != 22 {
		t.Fatalf("expected 22 parse errors, got %d", result.Summary.ParseErrorsCount)
	}
	if result.Summary.ParseWarningsCount != 21 {
		t.Fatalf("expected 21 parse warnings, got %d", result.Summary.ParseWarningsCount)
	}
	if len(result.Summary.ParseErrors) != 20 {
		t.Fatalf("expected 20 reported parse errors, got %d", len(result.Summary.ParseErrors))
	}
	if len(result.Summary.ParseWarnings) != 20 {
		t.Fatalf("expected 20 reported parse warnings, got %d", len(result.Summary.ParseWarnings))
	}
	if result.Summary.ParseErrors[0] != "error 1" || result.Summary.ParseErrors[19] != "error 20" {
		t.Fatalf("unexpected reported parse errors: %v", result.Summary.ParseErrors)
	}
	if result.Summary.ParseWarnings[0] != "warning 1" || result.Summary.ParseWarnings[19] != "warning 20" {
		t.Fatalf("unexpected reported parse warnings: %v", result.Summary.ParseWarnings)
	}
	if result.ReportVersion != SchemaVersion {
		t.Fatalf("expected report version %q, got %q", SchemaVersion, result.ReportVersion)
	}
	if result.DesignIR == nil || result.DesignIR.Version != ir.SchemaVersion {
		t.Fatalf("expected design IR version %q, got %+v", ir.SchemaVersion, result.DesignIR)
	}
}

func TestNewVerificationReport_SetsSchemaVersionForNilDesign(t *testing.T) {
	result := NewVerificationReport(nil)

	if result.ReportVersion != SchemaVersion {
		t.Fatalf("expected report version %q, got %q", SchemaVersion, result.ReportVersion)
	}
	if result.DesignIR == nil {
		t.Fatal("expected design IR to be initialized")
	}
	if result.DesignIR.Version != ir.SchemaVersion {
		t.Fatalf("expected design IR version %q, got %q", ir.SchemaVersion, result.DesignIR.Version)
	}
}
