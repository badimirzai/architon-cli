package report

import (
	"fmt"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
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
	if result.Summary.NextSteps[0] != "Run rv scan <bom.csv> --out report.json and inspect summary.parse_errors" {
		t.Fatalf("expected default parse error next step, got %v", result.Summary.NextSteps)
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

func TestNewVerificationReport_AddsDelimiterAndNextSteps(t *testing.T) {
	result := NewVerificationReport(&ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "kicad_bom_csv",
		Metadata: ir.IRMetadata{
			InputFile: "bom.csv",
			Delimiter: ";",
		},
		ParseErrors: []string{
			"row 3: malformed CSV row: expected 3 columns from header, got 1",
			`row 1: missing required BOM column for "value"`,
		},
	})

	if result.Summary.Delimiter != ";" {
		t.Fatalf("expected delimiter %q, got %q", ";", result.Summary.Delimiter)
	}
	wantSteps := []string{
		"Re-export BOM (CSV) and check missing delimiters/quotes",
		"Use --map mapping.yaml to map headers",
		"Run rv scan <bom.csv> --out report.json and inspect summary.parse_errors",
	}
	if len(result.Summary.NextSteps) != len(wantSteps) {
		t.Fatalf("expected next steps %v, got %v", wantSteps, result.Summary.NextSteps)
	}
	for i := range wantSteps {
		if result.Summary.NextSteps[i] != wantSteps[i] {
			t.Fatalf("expected next step %d to be %q, got %q", i, wantSteps[i], result.Summary.NextSteps[i])
		}
	}
}

func TestNewVerificationReport_OmitsNextStepsWithoutParseErrors(t *testing.T) {
	result := NewVerificationReport(&ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "kicad_bom_csv",
		Metadata: ir.IRMetadata{
			Delimiter: ",",
		},
	})

	if result.Summary.Delimiter != "," {
		t.Fatalf("expected delimiter %q, got %q", ",", result.Summary.Delimiter)
	}
	if len(result.Summary.NextSteps) != 0 {
		t.Fatalf("expected no next steps, got %v", result.Summary.NextSteps)
	}
}
