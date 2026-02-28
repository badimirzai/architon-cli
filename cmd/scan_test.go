package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type scanReport struct {
	Summary struct {
		Parts            int      `json:"parts"`
		ParseErrorsCount int      `json:"parse_errors_count"`
		ParseWarnings    []string `json:"parse_warnings"`
		ParseErrors      []string `json:"parse_errors"`
	} `json:"summary"`
	DesignIR struct {
		Parts []struct {
			Ref string `json:"ref"`
		} `json:"parts"`
	} `json:"design_ir"`
}

func kicadFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file path")
	}
	return filepath.Join(filepath.Dir(file), "..", "internal", "importers", "kicad", "testdata", name)
}

func runScanCommand(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
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

func readScanReport(t *testing.T, dir string) scanReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, defaultScanReportPath))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report scanReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	return report
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
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}

	report := readScanReport(t, tmpDir)
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
}
