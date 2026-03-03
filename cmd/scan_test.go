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
)

type scanReport struct {
	ReportVersion string `json:"report_version"`
	Summary       struct {
		Parts            int      `json:"parts"`
		ParseErrorsCount int      `json:"parse_errors_count"`
		ParseWarnings    []string `json:"parse_warnings"`
		ParseErrors      []string `json:"parse_errors"`
	} `json:"summary"`
	DesignIR struct {
		Version string `json:"version"`
		Parts   []struct {
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
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
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

func TestScan_CleanScanReturnsExitCodeZero(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "bom_minimal.csv"))
	if err != nil {
		t.Fatalf("expected clean scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Wrote "+defaultScanReportPath) {
		t.Fatalf("expected stdout to mention written report, got %q", stdout)
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

		got, discovered, err := resolveScanInput(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("exports/bom.csv wins over exports/project-bom.csv", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "exports", "bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "exports", "project-bom.csv"), "Ref,Qty\nR2,1\n")

		got, discovered, err := resolveScanInput(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("root bom.csv remains canonical", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "demo1.bom.csv"), "Ref,Qty\nR2,1\n")

		got, discovered, err := resolveScanInput(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("bom/project-bom.csv is detected when no canonical file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "bom", "project-bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "exports", "kicad_bom.csv"), "Ref,Qty\nR2,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "demo1.bom.csv"), "Ref,Qty\nR3,1\n")

		got, discovered, err := resolveScanInput(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("root demo1.bom.csv is detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "demo1.bom.csv")
		writeScanTestFile(t, expected, "Ref,Qty\nR1,1\n")

		got, discovered, err := resolveScanInput(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("multiple matches in same folder pick lexical first", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeScanTestFile(t, filepath.Join(tmpDir, "bom", "zeta-bom.csv"), "Ref,Qty\nR2,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "bom", "alpha-bom.csv"), "Ref,Qty\nR1,1\n")
		writeScanTestFile(t, filepath.Join(tmpDir, "nested", "ignored.bom.csv"), "Ref,Qty\nR3,1\n")

		got, discovered, err := resolveScanInput(tmpDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		expected := filepath.Join(tmpDir, "bom", "alpha-bom.csv")
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("random CSV does not match", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeScanTestFile(t, filepath.Join(tmpDir, "notes.csv"), "Ref,Qty\nR1,1\n")

		got, discovered, err := resolveScanInput(tmpDir)
		if err == nil {
			t.Fatal("expected error")
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != "" {
			t.Fatalf("expected empty resolved path, got %q", got)
		}
		if err.Error() != noBOMFoundInProjectDirMessage {
			t.Fatalf("expected exact error %q, got %q", noBOMFoundInProjectDirMessage, err.Error())
		}
	})

	t.Run("directory with no BOM returns exact error", func(t *testing.T) {
		tmpDir := t.TempDir()

		got, discovered, err := resolveScanInput(tmpDir)
		if err == nil {
			t.Fatal("expected error")
		}
		if !discovered {
			t.Fatalf("expected directory discovery")
		}
		if got != "" {
			t.Fatalf("expected empty resolved path, got %q", got)
		}
		if err.Error() != noBOMFoundInProjectDirMessage {
			t.Fatalf("expected exact error %q, got %q", noBOMFoundInProjectDirMessage, err.Error())
		}
	})

	t.Run("file input remains unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()
		inputPath := filepath.Join(tmpDir, "input.csv")
		writeScanTestFile(t, inputPath, "Ref,Qty\nR1,1\n")

		got, discovered, err := resolveScanInput(inputPath)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if discovered {
			t.Fatalf("expected file input to skip discovery")
		}
		if got != inputPath {
			t.Fatalf("expected %q, got %q", inputPath, got)
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
	detectedLine := strings.Split(strings.TrimSpace(stdout), "\n")[0]
	if !strings.HasPrefix(detectedLine, "Detected BOM: ") {
		t.Fatalf("expected detected BOM message, got %q", stdout)
	}
	if !strings.HasSuffix(detectedLine, filepath.Join("bom", "bom.csv")) {
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

func TestScan_DirectoryInputWithoutBOMReturnsExitCodeThree(t *testing.T) {
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
	if exitErr.Err == nil || exitErr.Err.Error() != noBOMFoundInProjectDirMessage {
		t.Fatalf("expected exact error %q, got %v", noBOMFoundInProjectDirMessage, exitErr.Err)
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
			want: 2,
		},
		{
			name: "rule failure",
			report: reportpkg.VerificationReport{
				Rules: []reportpkg.RuleResult{
					{ID: "BOM_RULE", Severity: "ERROR", Message: "bad part"},
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
