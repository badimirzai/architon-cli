package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
	reportpkg "github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/ui"
)

type scanReport struct {
	ReportVersion string `json:"report_version"`
	Summary       struct {
		Parts            int      `json:"parts"`
		Nets             int      `json:"nets"`
		ParseErrorsCount int      `json:"parse_errors_count"`
		ParseWarnings    []string `json:"parse_warnings"`
		ParseErrors      []string `json:"parse_errors"`
	} `json:"summary"`
	DesignIR struct {
		Version string `json:"version"`
		Source  string `json:"source"`
		Parts   []struct {
			Ref string `json:"ref"`
		} `json:"parts"`
		Nets []struct {
			Name string `json:"name"`
		} `json:"nets"`
	} `json:"design_ir"`
	Rules []struct {
		ID           string `json:"id"`
		RuleID       string `json:"rule_id"`
		Severity     string `json:"severity"`
		Message      string `json:"message"`
		ComponentRef string `json:"component_ref"`
		Net          string `json:"net"`
		Pin          string `json:"pin"`
		Source       string `json:"source"`
		Provenance   *struct {
			Source   string `json:"source"`
			SourceID string `json:"source_id"`
			Detail   string `json:"detail"`
		} `json:"provenance"`
		Fix       string `json:"fix"`
		Inference *struct {
			NetName         string  `json:"net_name"`
			Source          string  `json:"source"`
			ConfidenceScore float64 `json:"confidence_score"`
			ConfidenceLevel string  `json:"confidence_level"`
			Reason          string  `json:"reason"`
		} `json:"inference"`
	} `json:"rules"`
	Derived *struct {
		NetVoltages []struct {
			Net     string  `json:"net"`
			Voltage float64 `json:"voltage"`
			Source  string  `json:"source"`
		} `json:"net_voltages"`
		InferredNetVoltages []struct {
			Net        string  `json:"net"`
			Voltage    float64 `json:"voltage"`
			Source     string  `json:"source"`
			Confidence string  `json:"confidence"`
			Reason     string  `json:"reason"`
		} `json:"inferred_net_voltages"`
		UnknownVoltageNets []struct {
			Net    string `json:"net"`
			Reason string `json:"reason"`
		} `json:"unknown_voltage_nets"`
		RailInferences []struct {
			NetName         string   `json:"net_name"`
			Voltage         *float64 `json:"voltage"`
			Source          string   `json:"source"`
			ConfidenceScore float64  `json:"confidence_score"`
			ConfidenceLevel string   `json:"confidence_level"`
			Reason          string   `json:"reason"`
			Evidence        []string `json:"evidence"`
			Warnings        []string `json:"warnings"`
		} `json:"rail_inferences"`
		RailCoverage struct {
			TotalNets           int      `json:"total_nets"`
			RailsWithVoltage    int      `json:"rails_with_voltage"`
			RailsUnknown        int      `json:"rails_unknown"`
			HighConfidence      int      `json:"high_confidence"`
			MediumConfidence    int      `json:"medium_confidence"`
			LowConfidence       int      `json:"low_confidence"`
			UnknownConfidence   int      `json:"unknown_confidence"`
			CoverageRatio       float64  `json:"coverage_ratio"`
			HighConfidenceRatio float64  `json:"high_confidence_ratio"`
			UsableForRulesRatio float64  `json:"usable_for_rules_ratio"`
			OverallLevel        string   `json:"overall_level"`
			Warnings            []string `json:"warnings"`
		} `json:"rail_coverage"`
	} `json:"derived"`
}

func kicadFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file path")
	}
	return filepath.Join(filepath.Dir(file), "..", "internal", "importers", "kicad", "testdata", name)
}

func rootFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file path")
	}
	return filepath.Join(filepath.Dir(file), "..", "testdata", name)
}

func runScanCommand(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	ui.EnableColors(false)
	t.Cleanup(func() {
		ui.EnableColors(ui.DefaultColorEnabled())
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	cmd := newScanCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout.String(), err
}

func readScanReport(t *testing.T, path string) scanReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report scanReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	return report
}

func writeScanTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func kicadFixtureData(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(kicadFixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(data)
}

func TestScan_WritesReportWhenParseErrorsExist(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "bom_bad_row_missing_comma.csv"))
	if err == nil {
		t.Fatal("expected parse-error exit")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Summary.Parts != 1 {
		t.Fatalf("expected 1 parsed part in report, got %d", report.Summary.Parts)
	}
	if report.Summary.ParseErrorsCount != 1 {
		t.Fatalf("expected 1 parse error in report, got %d", report.Summary.ParseErrorsCount)
	}
	if len(report.Summary.ParseErrors) != 1 {
		t.Fatalf("expected 1 parse error message, got %d", len(report.Summary.ParseErrors))
	}
	if report.DesignIR.Parts[0].Ref != "R1" {
		t.Fatalf("expected valid part R1 to be preserved, got %q", report.DesignIR.Parts[0].Ref)
	}
	if report.ReportVersion != reportpkg.SchemaVersion {
		t.Fatalf("expected report version %q, got %q", reportpkg.SchemaVersion, report.ReportVersion)
	}
	if report.DesignIR.Version != ir.SchemaVersion {
		t.Fatalf("expected design IR version %q, got %q", ir.SchemaVersion, report.DesignIR.Version)
	}
}

func TestScan_ESP32BuiltInContractOvervoltage(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	netlist := rootFixturePath(t, filepath.Join("esp32_overvoltage", "netlist.net"))
	metaPath := rootFixturePath(t, filepath.Join("esp32_overvoltage", "meta.yaml"))

	_, err := runScanCommand(t, tmpDir, netlist, "--meta", metaPath, "--out", reportPath)
	if err == nil {
		t.Fatal("expected overvoltage exit")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}

	report := readScanReport(t, reportPath)
	if report.ReportVersion != "1" {
		t.Fatalf("expected report_version 1, got %q", report.ReportVersion)
	}
	if len(report.Rules) != 1 {
		t.Fatalf("expected one violation, got %+v", report.Rules)
	}
	finding := report.Rules[0]
	if finding.RuleID != "supply_abs_max" || finding.Severity != "ERROR" {
		t.Fatalf("expected supply_abs_max ERROR, got %+v", finding)
	}
	if finding.ComponentRef != "U1" || finding.Net != "/+5V" || finding.Pin != "VDD" {
		t.Fatalf("expected U1 /+5V VDD finding, got %+v", finding)
	}
	if finding.Source != "built-in" {
		t.Fatalf("expected built-in source, got %+v", finding)
	}
	if finding.Provenance == nil || finding.Provenance.Source != "built-in" {
		t.Fatalf("expected built-in provenance, got %+v", finding)
	}
	if finding.Fix == "" {
		t.Fatalf("expected fix, got %+v", finding)
	}
}

func TestScan_CleanScanReturnsExitCodeZero(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "bom_minimal.csv"))
	if err != nil {
		t.Fatalf("expected clean scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "ARCHITON SCAN\n") {
		t.Fatalf("expected scan summary header, got %q", stdout)
	}
	if !strings.Contains(stdout, "Target: "+kicadFixturePath(t, "bom_minimal.csv")+"\n") {
		t.Fatalf("expected target line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Result: OK") {
		t.Fatalf("expected clean result line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Parts: 2\n") {
		t.Fatalf("expected parts line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Nets: 0\n") {
		t.Fatalf("expected nets line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Errors: 0\n") {
		t.Fatalf("expected errors line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Warnings: 0\n") {
		t.Fatalf("expected warnings line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Wrote "+defaultScanReportPath) {
		t.Fatalf("expected stdout to mention written report, got %q", stdout)
	}
	if !strings.Contains(stdout, "exit code: 0\n") {
		t.Fatalf("expected exit code line, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.ReportVersion != reportpkg.SchemaVersion {
		t.Fatalf("expected report version %q, got %q", reportpkg.SchemaVersion, report.ReportVersion)
	}
	if report.DesignIR.Version != ir.SchemaVersion {
		t.Fatalf("expected design IR version %q, got %q", ir.SchemaVersion, report.DesignIR.Version)
	}
	if report.Summary.ParseErrorsCount != 0 {
		t.Fatalf("expected 0 parse errors, got %d", report.Summary.ParseErrorsCount)
	}
}

func TestScan_WritesReportToCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	customReportPath := filepath.Join(tmpDir, "result.json")

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "bom_minimal.csv"), "--out", customReportPath)
	if err != nil {
		t.Fatalf("expected clean scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Wrote "+customReportPath) {
		t.Fatalf("expected stdout to mention custom report path, got %q", stdout)
	}

	report := readScanReport(t, customReportPath)
	if report.Summary.Parts != 2 {
		t.Fatalf("expected 2 parts in custom report, got %d", report.Summary.Parts)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, defaultScanReportPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected default report path to remain unused, stat err=%v", err)
	}
}

func TestResolveScanInput(t *testing.T) {
	t.Run("bom/bom.csv wins over everything", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "bom", "bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "bom", "project-bom.csv"), "Ref,Qty\nR2,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "exports", "bom.csv"), "Ref,Qty\nR3,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "demo1.bom.csv"), "Ref,Qty\nR4,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !got.Directory {
			t.Fatalf("expected directory discovery")
		}
		if !got.BOMDiscovered {
			t.Fatalf("expected BOM discovery")
		}
		if got.BOMPath != expected {
			t.Fatalf("expected %q, got %q", expected, got.BOMPath)
		}
	})

	t.Run("exports/bom.csv wins over exports/project-bom.csv", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "exports", "bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "exports", "project-bom.csv"), "Ref,Qty\nR2,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !got.Directory {
			t.Fatalf("expected directory discovery")
		}
		if !got.BOMDiscovered {
			t.Fatalf("expected BOM discovery")
		}
		if got.BOMPath != expected {
			t.Fatalf("expected %q, got %q", expected, got.BOMPath)
		}
	})

	t.Run("root bom.csv remains canonical", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "demo1.bom.csv"), "Ref,Qty\nR2,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !got.Directory {
			t.Fatalf("expected directory discovery")
		}
		if !got.BOMDiscovered {
			t.Fatalf("expected BOM discovery")
		}
		if got.BOMPath != expected {
			t.Fatalf("expected %q, got %q", expected, got.BOMPath)
		}
	})

	t.Run("bom/project-bom.csv is detected when no canonical file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "bom", "project-bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "exports", "kicad_bom.csv"), "Ref,Qty\nR2,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "demo1.bom.csv"), "Ref,Qty\nR3,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !got.Directory {
			t.Fatalf("expected directory discovery")
		}
		if !got.BOMDiscovered {
			t.Fatalf("expected BOM discovery")
		}
		if got.BOMPath != expected {
			t.Fatalf("expected %q, got %q", expected, got.BOMPath)
		}
	})

	t.Run("root demo1.bom.csv is detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "demo1.bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !got.Directory {
			t.Fatalf("expected directory discovery")
		}
		if !got.BOMDiscovered {
			t.Fatalf("expected BOM discovery")
		}
		if got.BOMPath != expected {
			t.Fatalf("expected %q, got %q", expected, got.BOMPath)
		}
	})

	t.Run("multiple matches in same folder pick lexical first", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeScanTestFile(t, filepath.Join(tmpDir, "bom", "zeta-bom.csv"), "Ref,Qty\nR2,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "bom", "alpha-bom.csv"), "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "nested", "ignored.bom.csv"), "Ref,Qty\nR3,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !got.Directory {
			t.Fatalf("expected directory discovery")
		}
		expected := filepath.Join(tmpDir, "bom", "alpha-bom.csv")
		if !got.BOMDiscovered {
			t.Fatalf("expected BOM discovery")
		}
		if got.BOMPath != expected {
			t.Fatalf("expected %q, got %q", expected, got.BOMPath)
		}
	})

	t.Run("random CSV does not match", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeScanTestFile(t, filepath.Join(tmpDir, "notes.csv"), "Ref,Qty\nR1,1\n")

		got, err := resolveScanInput(tmpDir, "", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if got != (resolvedScanInput{}) {
			t.Fatalf("expected empty resolution, got %+v", got)
		}
		if err.Error() != noScanInputsFoundInProjectDirMessage {
			t.Fatalf("expected exact error %q, got %q", noScanInputsFoundInProjectDirMessage, err.Error())
		}
	})

	t.Run("directory with no scan inputs returns exact error", func(t *testing.T) {
		tmpDir := t.TempDir()

		got, err := resolveScanInput(tmpDir, "", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if got != (resolvedScanInput{}) {
			t.Fatalf("expected empty resolution, got %+v", got)
		}
		if err.Error() != noScanInputsFoundInProjectDirMessage {
			t.Fatalf("expected exact error %q, got %q", noScanInputsFoundInProjectDirMessage, err.Error())
		}
	})

	t.Run("file input remains unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()
		inputPath := filepath.Join(tmpDir, "input.csv")
		writeScanTestFile(t, inputPath, "Ref,Qty\nR1,1\n")

		got, err := resolveScanInput(inputPath, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got.Directory {
			t.Fatalf("expected file input to skip discovery")
		}
		if got.DirectPath != inputPath {
			t.Fatalf("expected %q, got %q", inputPath, got.DirectPath)
		}
	})
}

func TestScan_DirectoryInputDetectsBOMAndWritesReport(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "bom", "bom.csv"), kicadFixtureData(t, "bom_minimal.csv"))

	stdout, err := runScanCommand(t, tmpDir, ".")
	if err != nil {
		t.Fatalf("expected clean scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "ARCHITON SCAN\n") {
		t.Fatalf("expected scan summary header, got %q", stdout)
	}
	if !strings.Contains(stdout, "Target: .\n") {
		t.Fatalf("expected target line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Detected BOM: ") {
		t.Fatalf("expected detected BOM message, got %q", stdout)
	}
	if !strings.Contains(stdout, filepath.Join("bom", "bom.csv")) {
		t.Fatalf("expected detected BOM message, got %q", stdout)
	}
	if !strings.Contains(stdout, "Wrote "+defaultScanReportPath) {
		t.Fatalf("expected report output message, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Summary.Parts != 2 {
		t.Fatalf("expected 2 parts in report, got %d", report.Summary.Parts)
	}
}

func TestScan_NetlistFileWritesReport(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "netlist_simple.net"))
	if err != nil {
		t.Fatalf("expected clean netlist scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "ARCHITON SCAN\n") {
		t.Fatalf("expected scan summary header, got %q", stdout)
	}
	if !strings.Contains(stdout, "Parts: 3\n") {
		t.Fatalf("expected parts line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Nets: 2\n") {
		t.Fatalf("expected nets line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Errors: 0\n") {
		t.Fatalf("expected errors line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Warnings: 0\n") {
		t.Fatalf("expected warnings line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Wrote "+defaultScanReportPath) {
		t.Fatalf("expected stdout to mention written report, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Summary.Parts != 3 {
		t.Fatalf("expected 3 parts in report, got %d", report.Summary.Parts)
	}
	if report.Summary.Nets != 2 {
		t.Fatalf("expected 2 nets in report summary, got %d", report.Summary.Nets)
	}
	if len(report.DesignIR.Nets) != 2 {
		t.Fatalf("expected 2 nets in design IR, got %d", len(report.DesignIR.Nets))
	}
}

func TestScan_NetlistReportsInferredAndUnknownVoltageNets(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, filepath.Join("overvoltage", "netlist_overvoltage.net")))
	if err != nil {
		t.Fatalf("expected netlist scan with inferred voltages to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Inferred voltages: 2 Unknown voltage nets: 1 Rail coverage: LOW 67%\n") {
		t.Fatalf("expected voltage inference counts, got %q", stdout)
	}
	if !strings.Contains(stdout, "Inferred rails: 2\n") {
		t.Fatalf("expected inferred rails line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Voltage coverage: 2/3 nets with inferred voltage\n") {
		t.Fatalf("expected voltage coverage line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Metadata: inferred\n") {
		t.Fatalf("expected inferred metadata mode, got %q", stdout)
	}
	if strings.Contains(stdout, "Rail inference:\n") {
		t.Fatalf("expected default output to stay compact, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Derived == nil {
		t.Fatal("expected derived voltage report")
	}

	var found5V bool
	for _, nv := range report.Derived.InferredNetVoltages {
		if nv.Net == "/+5V" {
			found5V = true
			if nv.Voltage != 5.0 {
				t.Fatalf("expected /+5V inferred as 5.0, got %v", nv.Voltage)
			}
			if nv.Source != "net_name" {
				t.Fatalf("expected inferred source net_name, got %q", nv.Source)
			}
			if nv.Confidence != "HIGH" || nv.Reason == "" {
				t.Fatalf("expected inferred voltage provenance, got %+v", nv)
			}
		}
	}
	if !found5V {
		t.Fatalf("expected /+5V in inferred voltages, got %+v", report.Derived.InferredNetVoltages)
	}

	if len(report.Derived.UnknownVoltageNets) != 1 {
		t.Fatalf("expected 1 unknown voltage net, got %+v", report.Derived.UnknownVoltageNets)
	}
	if report.Derived.UnknownVoltageNets[0].Net != "/VBAT" {
		t.Fatalf("expected /VBAT unknown, got %+v", report.Derived.UnknownVoltageNets)
	}
	if report.Derived.UnknownVoltageNets[0].Reason != "ambiguous power net name" {
		t.Fatalf("expected ambiguous power reason, got %q", report.Derived.UnknownVoltageNets[0].Reason)
	}

	var foundRailInference bool
	for _, inference := range report.Derived.RailInferences {
		if inference.NetName != "/+5V" {
			continue
		}
		foundRailInference = true
		if inference.Voltage == nil || *inference.Voltage != 5.0 {
			t.Fatalf("expected /+5V rail inference voltage 5.0, got %+v", inference.Voltage)
		}
		if inference.Source != "net_name" {
			t.Fatalf("expected /+5V rail inference source net_name, got %q", inference.Source)
		}
		if inference.ConfidenceLevel != "HIGH" || inference.ConfidenceScore != 0.95 {
			t.Fatalf("expected /+5V high confidence score 0.95, got %+v", inference)
		}
		if inference.Reason == "" {
			t.Fatalf("expected /+5V inference reason, got %+v", inference)
		}
	}
	if !foundRailInference {
		t.Fatalf("expected /+5V rail inference in report, got %+v", report.Derived.RailInferences)
	}
	if report.Derived.RailCoverage.TotalNets != 3 {
		t.Fatalf("expected rail coverage total nets 3, got %+v", report.Derived.RailCoverage)
	}
	if report.Derived.RailCoverage.RailsWithVoltage != 2 || report.Derived.RailCoverage.RailsUnknown != 1 {
		t.Fatalf("unexpected rail coverage counts: %+v", report.Derived.RailCoverage)
	}
	if report.Derived.RailCoverage.UsableForRulesRatio != 0.6667 {
		t.Fatalf("expected usable ratio 0.6667, got %+v", report.Derived.RailCoverage)
	}
}

func TestScan_ExplainRailsPrintsInferenceDetails(t *testing.T) {
	tmpDir := t.TempDir()
	metaPath := filepath.Join(tmpDir, "meta.yaml")
	writeScanTestFile(t, metaPath, `version: "0"
sources:
  - net: /VBAT
    voltage: 24.0
regulators:
  - ref: U2
    in_pin: "1"
    out_pin: "3"
    out_voltage: 5.0
components:
  - ref: U1
    max_voltage: 6.0
`)

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, filepath.Join("overvoltage", "netlist_overvoltage.net")), "--meta", metaPath, "--explain-rails")
	if err != nil {
		t.Fatalf("expected netlist scan with inferred voltages to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Rail inference:\n") {
		t.Fatalf("expected rail inference header, got %q", stdout)
	}
	if !strings.Contains(stdout, "- /+5V: 5.00V  HIGH   0.95  net_name\n") {
		t.Fatalf("expected /+5V rail line, got %q", stdout)
	}
	if !strings.Contains(stdout, "- /VBAT: 24.00V HIGH   1.00  USER_OVERRIDE\n") {
		t.Fatalf("expected /VBAT rail line, got %q", stdout)
	}
	if !strings.Contains(stdout, "- GND:   0.00V  HIGH   0.95  net_name\n") {
		t.Fatalf("expected GND rail line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Rail coverage:\n") {
		t.Fatalf("expected rail coverage section, got %q", stdout)
	}
	if !strings.Contains(stdout, "- Total nets: 3\n") {
		t.Fatalf("expected total nets line, got %q", stdout)
	}
	if !strings.Contains(stdout, "- Usable for rules: 3/3\n") {
		t.Fatalf("expected usable line, got %q", stdout)
	}
	if !strings.Contains(stdout, "- Coverage: HIGH 100%\n") {
		t.Fatalf("expected coverage line, got %q", stdout)
	}
}

func TestScan_OvervoltageUsesInferredVoltageWithoutMetaSource(t *testing.T) {
	tmpDir := t.TempDir()
	metaPath := filepath.Join(tmpDir, "meta.yaml")
	writeScanTestFile(t, metaPath, `version: "0"
sources: []
components:
  - ref: U1
    max_voltage: 3.3
`)

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, filepath.Join("overvoltage", "netlist_overvoltage.net")), "--meta", metaPath)
	if err == nil {
		t.Fatalf("expected overvoltage exit, got success\n%s", stdout)
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d\n%s", exitErr.Code, stdout)
	}
	if !strings.Contains(stdout, "RULE_SUPPLY_CONTRACT") {
		t.Fatalf("expected supply contract finding, got %q", stdout)
	}
	if !strings.Contains(stdout, "Net /+5V provides 5.00V but U1 pin 1 allows max 3.30V") {
		t.Fatalf("expected inferred /+5V overvoltage, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if len(report.Rules) != 1 {
		t.Fatalf("expected one rule finding, got %+v", report.Rules)
	}
	if report.Rules[0].Inference == nil {
		t.Fatalf("expected overvoltage finding inference provenance, got %+v", report.Rules[0])
	}
	if report.Rules[0].Inference.NetName != "/+5V" {
		t.Fatalf("expected inference net /+5V, got %+v", report.Rules[0].Inference)
	}
	if report.Rules[0].Inference.Source != "net_name" {
		t.Fatalf("expected inference source net_name, got %+v", report.Rules[0].Inference)
	}
	if report.Rules[0].Inference.ConfidenceLevel != "HIGH" || report.Rules[0].Inference.ConfidenceScore != 0.95 {
		t.Fatalf("expected high confidence inference, got %+v", report.Rules[0].Inference)
	}
	if report.Rules[0].Inference.Reason == "" {
		t.Fatalf("expected inference reason provenance, got %+v", report.Rules[0].Inference)
	}
}

func TestScan_MetaSourceOverridesInferredVoltage(t *testing.T) {
	tmpDir := t.TempDir()
	metaPath := filepath.Join(tmpDir, "meta.yaml")
	writeScanTestFile(t, metaPath, `version: "0"
sources:
  - net: /+5V
    voltage: 4.7
components:
  - ref: U1
    max_voltage: 4.8
`)

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, filepath.Join("overvoltage", "netlist_overvoltage.net")), "--meta", metaPath)
	if err != nil {
		t.Fatalf("expected meta override to avoid overvoltage, got %v\n%s", err, stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Derived == nil {
		t.Fatal("expected derived voltage report")
	}

	var found bool
	for _, nv := range report.Derived.NetVoltages {
		if nv.Net != "/+5V" {
			continue
		}
		found = true
		if nv.Voltage != 4.7 {
			t.Fatalf("expected meta override voltage 4.7, got %v", nv.Voltage)
		}
		if nv.Source != "source" {
			t.Fatalf("expected metadata source, got %q", nv.Source)
		}
	}
	if !found {
		t.Fatalf("expected /+5V in propagated net voltages, got %+v", report.Derived.NetVoltages)
	}
}

func TestScan_DirectoryInputMergesBOMAndNetlist(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "bom", "bom.csv"), kicadFixtureData(t, "bom_minimal.csv"))
	writeScanTestFile(t, filepath.Join(tmpDir, "exports", "example.net"), kicadFixtureData(t, "netlist_simple.net"))

	stdout, err := runScanCommand(t, tmpDir, ".")
	if err != nil {
		t.Fatalf("expected merged directory scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Detected BOM: ") {
		t.Fatalf("expected detected BOM message, got %q", stdout)
	}
	if !strings.Contains(stdout, "Detected Netlist: ") {
		t.Fatalf("expected detected netlist message, got %q", stdout)
	}
	if !strings.Contains(stdout, "Parts: 3\n") {
		t.Fatalf("expected parts line, got %q", stdout)
	}
	if !strings.Contains(stdout, "Nets: 2\n") {
		t.Fatalf("expected nets line, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.DesignIR.Source != "kicad_project" {
		t.Fatalf("expected merged design source kicad_project, got %q", report.DesignIR.Source)
	}
	if report.Summary.Parts != 3 {
		t.Fatalf("expected merged report to contain 3 parts, got %d", report.Summary.Parts)
	}
	if report.Summary.Nets != 2 {
		t.Fatalf("expected merged report to contain 2 nets, got %d", report.Summary.Nets)
	}
	gotRefs := []string{report.DesignIR.Parts[0].Ref, report.DesignIR.Parts[1].Ref, report.DesignIR.Parts[2].Ref}
	wantRefs := []string{"C1", "R1", "U1"}
	for i := range wantRefs {
		if gotRefs[i] != wantRefs[i] {
			t.Fatalf("expected refs %v, got %v", wantRefs, gotRefs)
		}
	}
}

func TestScan_DirectoryInputWithoutInputsReturnsExitCodeThree(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := runScanCommand(t, tmpDir, ".")
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}
	if exitErr.Err == nil || exitErr.Err.Error() != noScanInputsFoundInProjectDirMessage {
		t.Fatalf("expected exact error %q, got %v", noScanInputsFoundInProjectDirMessage, exitErr.Err)
	}
}

func TestFindBOMCandidates(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "kicad_bom.csv"), "Ref,Qty\nR1,1\n")
	writeScanTestFile(t, filepath.Join(tmpDir, "alpha.bom.csv"), "Ref,Qty\nR2,1\n")
	writeScanTestFile(t, filepath.Join(tmpDir, "notes.csv"), "Ref,Qty\nR3,1\n")

	got, err := findBOMCandidates(tmpDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := []string{
		filepath.Join(tmpDir, "alpha.bom.csv"),
		filepath.Join(tmpDir, "kicad_bom.csv"),
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d candidates, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected candidate %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestScanExitCode(t *testing.T) {
	tests := []struct {
		name   string
		report reportpkg.VerificationReport
		want   int
	}{
		{
			name: "malformed bom",
			report: reportpkg.VerificationReport{
				Summary: reportpkg.Summary{ParseErrorsCount: 1},
			},
			want: 3,
		},
		{
			name: "rule failure",
			report: reportpkg.VerificationReport{
				Rules: []reportpkg.RuleResult{
					{ID: "BOM_RULE", Severity: "ERROR", Message: "bad part"},
				},
			},
			want: 2,
		},
		{
			name: "warning only",
			report: reportpkg.VerificationReport{
				Rules: []reportpkg.RuleResult{
					{ID: "BOM_RULE", Severity: "WARN", Message: "check part"},
				},
			},
			want: 1,
		},
		{
			name:   "clean scan",
			report: reportpkg.VerificationReport{},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scanExitCode(tt.report); got != tt.want {
				t.Fatalf("expected exit code %d, got %d", tt.want, got)
			}
		})
	}
}
