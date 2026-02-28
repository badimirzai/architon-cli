package kicad

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/badimirzai/robotics-verifier-cli/internal/report"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file path")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func TestDetectDelimiter_AutoDetectsSemicolon(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, "bom_semicolon.csv"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	delimiter := detectDelimiter(splitNormalizedLines(string(data)))
	if delimiter != ';' {
		t.Fatalf("expected semicolon delimiter, got %q", string(delimiter))
	}
}

func TestImportKiCadBOM_ValidParse(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_kicad_default.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if design.Source != "kicad_bom_csv" {
		t.Fatalf("expected source kicad_bom_csv, got %q", design.Source)
	}
	if len(design.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(design.Parts))
	}
	first := design.Parts[0]
	if first.Ref != "R1" || first.Value != "10k" || first.Footprint == "" {
		t.Fatalf("unexpected first part: %+v", first)
	}
	if first.Fields["Reference"] != "R1" {
		t.Fatalf("expected raw field Reference to be preserved")
	}
	if len(design.ParseWarnings) != 0 {
		t.Fatalf("expected no parse warnings, got %v", design.ParseWarnings)
	}
	if len(design.ParseErrors) != 0 {
		t.Fatalf("expected no parse errors, got %v", design.ParseErrors)
	}
}

func TestImportKiCadBOM_SemicolonDelimiter(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_semicolon.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(design.Parts))
	}
	if got := design.Parts[1].Ref; got != "C1" {
		t.Fatalf("expected second ref C1, got %q", got)
	}
}

func TestImportKiCadBOM_PreambleRows(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_preamble_rows.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(design.Parts))
	}
	if got := design.Parts[0].Ref; got != "R1" {
		t.Fatalf("expected first ref R1 after skipping preamble, got %q", got)
	}
}

func TestImportKiCadBOM_HeaderAutoDetection(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_kicad_real.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(design.Parts))
	}
	part := design.Parts[0]
	if part.Ref != "U1" {
		t.Fatalf("expected ref U1, got %q", part.Ref)
	}
	if part.Footprint == "" {
		t.Fatal("expected footprint to be detected from Package header")
	}
	if part.MPN == "" || part.Manufacturer == "" {
		t.Fatalf("expected optional columns auto-detected, got mpn=%q manufacturer=%q", part.MPN, part.Manufacturer)
	}
}

func TestImportKiCadBOM_ExplicitMapping(t *testing.T) {
	tmpDir := t.TempDir()
	mappingFile := filepath.Join(tmpDir, "mapping.yaml")
	mappingYAML := `ref: Designator
value: Component
footprint: Package
`
	if err := os.WriteFile(mappingFile, []byte(mappingYAML), 0o644); err != nil {
		t.Fatalf("write mapping file: %v", err)
	}

	mapping, err := LoadColumnMapping(mappingFile)
	if err != nil {
		t.Fatalf("LoadColumnMapping returned error: %v", err)
	}

	design, err := ImportKiCadBOM(fixturePath(t, "bom_explicit_mapping.csv"), mapping)
	if err != nil {
		t.Fatalf("ImportKiCadBOM with explicit mapping returned error: %v", err)
	}

	if len(design.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(design.Parts))
	}
	if design.Parts[0].Value != "SS14" {
		t.Fatalf("expected mapped value SS14, got %q", design.Parts[0].Value)
	}
}

func TestImportKiCadBOM_MissingColumnsBecomeParseErrors(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_missing_required_header.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 0 {
		t.Fatalf("expected 0 parts, got %d", len(design.Parts))
	}
	if len(design.ParseErrors) != 1 {
		t.Fatalf("expected 1 parse error, got %d", len(design.ParseErrors))
	}
	want := `row 1: missing required BOM column for "value"`
	if !strings.Contains(design.ParseErrors[0], want) {
		t.Fatalf("expected parse error to contain %q, got %q", want, design.ParseErrors[0])
	}
}

func TestImportKiCadBOM_ValueHeaderSynonymComponent(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_value_as_component.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(design.Parts))
	}
	if got := design.Parts[0].Value; got != "DRV8833" {
		t.Fatalf("expected Component header to map to value DRV8833, got %q", got)
	}
}

func TestImportKiCadBOM_GroupedReferencesExpand(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_grouped_refs.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 3 {
		t.Fatalf("expected 3 expanded parts, got %d", len(design.Parts))
	}
	gotRefs := []string{design.Parts[0].Ref, design.Parts[1].Ref, design.Parts[2].Ref}
	wantRefs := []string{"R1", "R2", "R3"}
	for i := range wantRefs {
		if gotRefs[i] != wantRefs[i] {
			t.Fatalf("expected refs %v, got %v", wantRefs, gotRefs)
		}
	}
}

func TestImportKiCadBOM_MalformedShortRowRecorded(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_bad_row_missing_comma.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 1 {
		t.Fatalf("expected only the valid row to produce a part, got %d", len(design.Parts))
	}
	if len(design.ParseErrors) != 1 {
		t.Fatalf("expected 1 parse error, got %d", len(design.ParseErrors))
	}
	want := "row 3: malformed CSV row: expected 3 columns from header, got 1"
	if design.ParseErrors[0] != want {
		t.Fatalf("expected parse error %q, got %q", want, design.ParseErrors[0])
	}
}

func TestImportKiCadBOM_MissingFootprintWarningInReport(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_missing_footprint.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	wantWarning := "row 2 (ref=D1): missing recommended field footprint (column=Footprint)"
	if len(design.ParseWarnings) != 1 {
		t.Fatalf("expected 1 parse warning, got %d", len(design.ParseWarnings))
	}
	if design.ParseWarnings[0] != wantWarning {
		t.Fatalf("expected warning %q, got %q", wantWarning, design.ParseWarnings[0])
	}

	result := report.NewVerificationReport(design)
	if result.Summary.ParseErrorsCount != 0 {
		t.Fatalf("expected 0 parse errors, got %d", result.Summary.ParseErrorsCount)
	}
	if result.Summary.ParseWarningsCount != 1 {
		t.Fatalf("expected 1 parse warning, got %d", result.Summary.ParseWarningsCount)
	}
	if len(result.Summary.ParseWarnings) != 1 {
		t.Fatalf("expected 1 reported parse warning, got %d", len(result.Summary.ParseWarnings))
	}
	if result.Summary.ParseWarnings[0] != wantWarning {
		t.Fatalf("expected reported warning %q, got %q", wantWarning, result.Summary.ParseWarnings[0])
	}
}

func TestImportKiCadBOM_QuantityWithoutRefRecorded(t *testing.T) {
	design, err := ImportKiCadBOM(fixturePath(t, "bom_qty_without_ref.csv"), ColumnMapping{})
	if err != nil {
		t.Fatalf("ImportKiCadBOM returned error: %v", err)
	}

	if len(design.Parts) != 0 {
		t.Fatalf("expected 0 parts, got %d", len(design.Parts))
	}
	if len(design.ParseErrors) != 1 {
		t.Fatalf("expected 1 parse error, got %d", len(design.ParseErrors))
	}
	want := "row 2: quantity present (Qty=10) but ref is empty; explicit refs are required for now"
	if design.ParseErrors[0] != want {
		t.Fatalf("expected parse error %q, got %q", want, design.ParseErrors[0])
	}
}

func TestRawFields_ShortRecord(t *testing.T) {
	fields := rawFields([]string{"Reference", "Value", "Footprint"}, []string{"D1"})

	if fields["Reference"] != "D1" {
		t.Fatalf("expected Reference field D1, got %q", fields["Reference"])
	}
	if fields["Value"] != "" {
		t.Fatalf("expected missing Value field to map to empty string, got %q", fields["Value"])
	}
	if fields["Footprint"] != "" {
		t.Fatalf("expected missing Footprint field to map to empty string, got %q", fields["Footprint"])
	}
}
