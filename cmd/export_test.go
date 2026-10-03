package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ui"
)

func TestExport_AgentLoopFixtures(t *testing.T) {
	broken := examplePath(t, filepath.Join("agent-loop", "broken"))
	fixed := examplePath(t, filepath.Join("agent-loop", "fixed"))

	stdout, err := runExportCommand(t, broken, ".")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected broken fixture to exit 2, got err=%v stdout=%s", err, stdout)
	}
	reportPath, graphPath := studioExportPaths(broken)
	report := readExportReport(t, reportPath)
	pin := requireExportFinding(t, report, "interface_pin_mismatch")
	if pin.Expected == nil || pin.Expected.Text != "PB15" || pin.Observed == nil || pin.Observed.Text != "PA7" {
		t.Fatalf("pin mismatch evidence = %+v %+v", pin.Expected, pin.Observed)
	}
	termination := requireExportFinding(t, report, "termination_count_low")
	if termination.Expected == nil || termination.Expected.Text != "2" || termination.Observed == nil || termination.Observed.Text != "1" {
		t.Fatalf("termination evidence = %+v %+v", termination.Expected, termination.Observed)
	}
	assertExportGraph(t, graphPath)

	stdout, err = runExportCommand(t, fixed, ".")
	if err != nil {
		t.Fatalf("expected fixed fixture to exit 0, got err=%v stdout=%s", err, stdout)
	}
	reportPath, graphPath = studioExportPaths(fixed)
	report = readExportReport(t, reportPath)
	for _, finding := range report.Findings {
		if finding.Severity == "ERROR" {
			t.Fatalf("expected no error findings, got %+v", report.Findings)
		}
	}
	assertExportGraph(t, graphPath)
}

func TestExport_EmptyDirectoryWritesNothing(t *testing.T) {
	dir := t.TempDir()
	stdout, err := runExportCommand(t, dir, ".")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected empty directory to exit 3, got err=%v stdout=%s", err, stdout)
	}
	reportPath, _ := studioExportPaths(dir)
	if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("empty export must not write a report, stat err=%v", statErr)
	}
}

func runExportCommand(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	ui.EnableColors(false)
	t.Cleanup(func() {
		ui.EnableColors(ui.DefaultColorEnabled())
		_ = os.RemoveAll(filepath.Join(cwd, ".architon", "studio"))
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

	cmd := newExportCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout.String(), err
}

func studioExportPaths(project string) (string, string) {
	dir := filepath.Join(project, ".architon", "studio")
	return filepath.Join(dir, "report.json"), filepath.Join(dir, "graph.json")
}

type exportReport struct {
	ReportVersion string          `json:"report_version"`
	Findings      []exportFinding `json:"findings"`
}

type exportFinding struct {
	RuleID   string          `json:"rule_id"`
	Severity string          `json:"severity"`
	Expected *exportEvidence `json:"expected"`
	Observed *exportEvidence `json:"observed"`
}

type exportEvidence struct {
	Text string `json:"text"`
}

type exportGraph struct {
	GraphVersion  string                     `json:"graph_version"`
	Nodes         []json.RawMessage          `json:"nodes"`
	Edges         []json.RawMessage          `json:"edges"`
	FindingsIndex map[string]json.RawMessage `json:"findings_index"`
}

func readExportReport(t *testing.T, path string) exportReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report %s: %v", path, err)
	}
	var report exportReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, data)
	}
	if report.ReportVersion == "" {
		t.Fatalf("report missing report_version: %s", data)
	}
	return report
}

func requireExportFinding(t *testing.T, report exportReport, ruleID string) exportFinding {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.RuleID == ruleID {
			return finding
		}
	}
	t.Fatalf("missing finding %s in %+v", ruleID, report.Findings)
	return exportFinding{}
}

func assertExportGraph(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read graph %s: %v", path, err)
	}
	var graph exportGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatalf("unmarshal graph: %v\n%s", err, data)
	}
	if graph.GraphVersion == "" || graph.Nodes == nil || graph.Edges == nil || graph.FindingsIndex == nil {
		t.Fatalf("graph missing graph_version, nodes, edges, or findings_index: %s", data)
	}
}
