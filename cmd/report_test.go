package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ui"
)

func runReportCommand(t *testing.T, cwd string, args ...string) (string, error) {
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

	cmd := newReportCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout.String(), err
}

func TestReportCommand_CreatesOfflineHTMLWithEmbeddedDataAndViolationExit(t *testing.T) {
	cwd := writeGraphPullupFixture(t, nil, false)

	stdout, err := runReportCommand(t, cwd, "design.net", "--contracts", "contracts.yaml", "--format", "html", "--out", "report.html")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected report command to return scan violation exit 2, got err=%v stdout=%s", err, stdout)
	}

	htmlPath := filepath.Join(cwd, "report.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("expected report file to be created: %v", err)
	}
	html := string(data)
	for _, want := range []string{
		"Architon Offline Report",
		"FAIL",
		"i2c_pullups",
		"no pull-up resistor",
		`id="architon-scan-json"`,
		`id="architon-graph-json"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected HTML report to contain %q\n%s", want, html)
		}
	}
	if strings.Contains(html, "https://") || strings.Contains(html, "http://") {
		t.Fatalf("expected report to avoid external network references")
	}

	var scan scanReport
	if err := json.Unmarshal([]byte(extractEmbeddedJSON(t, html, "architon-scan-json")), &scan); err != nil {
		t.Fatalf("embedded scan JSON is invalid: %v", err)
	}
	if len(scan.Findings) == 0 || scan.Findings[0].ContractID != "i2c_pullups" {
		t.Fatalf("expected embedded scan JSON to contain contract finding, got %+v", scan.Findings)
	}

	graph := parseGraphOutput(t, extractEmbeddedJSON(t, html, "architon-graph-json"))
	if graph.Summary.Violations == 0 || len(graph.Findings) == 0 {
		t.Fatalf("expected embedded GraphIR JSON to contain findings, got %+v", graph)
	}
}

func TestReportCommand_WorksWithZeroFindings(t *testing.T) {
	cwd := writeGraphPullupFixture(t, []graphResistor{
		{Ref: "R1", Value: "4.7k", A: "/I2C_SDA", B: "/+3V3"},
		{Ref: "R2", Value: "4.7k", A: "/I2C_SCL", B: "/+3V3"},
	}, false)

	stdout, err := runReportCommand(t, cwd, "design.net", "--contracts", "contracts.yaml", "--format", "html", "--out", "clean.html")
	if err != nil {
		t.Fatalf("expected report command to succeed for zero findings, got %v\n%s", err, stdout)
	}

	data, err := os.ReadFile(filepath.Join(cwd, "clean.html"))
	if err != nil {
		t.Fatalf("expected clean report file to be created: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "PASS") {
		t.Fatalf("expected clean report status PASS\n%s", html)
	}
	if !strings.Contains(html, "No findings.") {
		t.Fatalf("expected clean report to render empty findings state\n%s", html)
	}
	var scan scanReport
	if err := json.Unmarshal([]byte(extractEmbeddedJSON(t, html, "architon-scan-json")), &scan); err != nil {
		t.Fatalf("embedded clean scan JSON is invalid: %v", err)
	}
	if len(scan.Findings) != 0 {
		t.Fatalf("expected zero embedded scan findings, got %+v", scan.Findings)
	}
	graph := parseGraphOutput(t, extractEmbeddedJSON(t, html, "architon-graph-json"))
	if graph.Summary.Findings != 0 {
		t.Fatalf("expected zero embedded graph findings, got %+v", graph.Summary)
	}
}

func TestReportCommand_RejectsUnsupportedFormat(t *testing.T) {
	cwd := writeGraphPullupFixture(t, nil, false)
	stdout, err := runReportCommand(t, cwd, "design.net", "--format", "json")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected unsupported format exit 3, got err=%v stdout=%s", err, stdout)
	}
}

func TestReportCommand_ProjectDirectoryArtifactsMatchEmbeddedJSON(t *testing.T) {
	cwd := writeGraphPullupFixture(t, nil, true)

	scanStdout, scanErr := runScanCommand(t, cwd, ".", "--contracts", ".architon/contracts.yaml", "--format", "json", "--out", "scan.json")
	requireExitCode(t, scanErr, 2, scanStdout)
	scan := readScanReport(t, filepath.Join(cwd, "scan.json"))
	if !scan.Summary.HasFailures || scan.Summary.UserContractsLoaded != 1 || len(scan.Findings) != 2 {
		t.Fatalf("expected scan to report two contract failures, got summary=%+v findings=%+v", scan.Summary, scan.Findings)
	}

	graphStdout, err := runGraphCommand(t, cwd, ".", "--contracts", ".architon/contracts.yaml", "--format", "json", "--out", "graph.json")
	if err != nil {
		t.Fatalf("expected graph command to succeed, got %v\n%s", err, graphStdout)
	}
	graph := parseGraphOutput(t, graphStdout)
	if graph.Summary.Findings != 2 || graph.Summary.Violations != 2 || graph.Summary.UserContractsLoaded != 1 || !graph.Summary.HasFailures {
		t.Fatalf("expected graph to report two violations, got %+v", graph.Summary)
	}

	reportStdout, reportErr := runReportCommand(t, cwd, ".", "--contracts", ".architon/contracts.yaml", "--format", "html", "--out", "report.html", "--scan-out", "report.json", "--graph-out", "report-graph.json")
	requireExitCode(t, reportErr, 2, reportStdout)
	for _, want := range []string{
		"Wrote report.html",
		"Embedded scan findings: 2",
		"Embedded graph findings: 2",
		"User contracts loaded: 1",
		"exit code: 2",
	} {
		if !strings.Contains(reportStdout, want) {
			t.Fatalf("expected report stdout to contain %q, got %q", want, reportStdout)
		}
	}
	html := readTestFileString(t, filepath.Join(cwd, "report.html"))
	embeddedScan := extractEmbeddedJSON(t, html, "architon-scan-json")
	embeddedGraph := extractEmbeddedJSON(t, html, "architon-graph-json")
	if got := readTestFileString(t, filepath.Join(cwd, "report.json")); got != embeddedScan {
		t.Fatalf("--scan-out JSON must equal embedded scan JSON")
	}
	if got := readTestFileString(t, filepath.Join(cwd, "report-graph.json")); got != embeddedGraph {
		t.Fatalf("--graph-out JSON must equal embedded GraphIR JSON")
	}
	var reportScan scanReport
	if err := json.Unmarshal([]byte(embeddedScan), &reportScan); err != nil {
		t.Fatalf("embedded scan JSON invalid: %v", err)
	}
	reportGraph := parseGraphOutput(t, embeddedGraph)
	if len(reportScan.Findings) != 2 || reportScan.Summary.UserContractsLoaded != 1 || !reportScan.Summary.HasFailures {
		t.Fatalf("expected embedded scan to match scan result, got summary=%+v findings=%+v", reportScan.Summary, reportScan.Findings)
	}
	if reportGraph.Summary.Findings != 2 || reportGraph.Summary.Violations != 2 || reportGraph.Summary.UserContractsLoaded != 1 || !reportGraph.Summary.HasFailures {
		t.Fatalf("expected embedded graph to match graph result, got %+v", reportGraph.Summary)
	}
}

func TestReportCommand_DirectGeneratedNetlistDiscoversProjectContracts(t *testing.T) {
	cwd := t.TempDir()
	writeScanTestFile(t, filepath.Join(cwd, ".architon", "generated.net"), graphPullupNetlist(nil))
	writeScanTestFile(t, filepath.Join(cwd, ".architon", "contracts.yaml"), graphPullupContracts())

	scanStdout, scanErr := runScanCommand(t, cwd, ".architon/generated.net", "--format", "json", "--out", "scan.json")
	requireExitCode(t, scanErr, 2, scanStdout)
	scan := readScanReport(t, filepath.Join(cwd, "scan.json"))
	if scan.Summary.UserContractsLoaded != 1 || len(scan.Findings) != 2 {
		t.Fatalf("expected direct generated netlist to auto-load project contracts, got summary=%+v findings=%+v", scan.Summary, scan.Findings)
	}

	graphStdout, err := runGraphCommand(t, cwd, ".architon/generated.net", "--format", "json")
	if err != nil {
		t.Fatalf("expected graph command to preserve default success behavior, got %v\n%s", err, graphStdout)
	}
	if graph := parseGraphOutput(t, graphStdout); graph.Summary.Findings != 2 || graph.Summary.Violations != 2 || graph.Summary.UserContractsLoaded != 1 || !graph.Summary.HasFailures {
		t.Fatalf("expected direct generated netlist graph findings, got %+v", graph.Summary)
	}

	reportStdout, reportErr := runReportCommand(t, cwd, ".architon/generated.net", "--format", "html", "--out", "report.html")
	requireExitCode(t, reportErr, 2, reportStdout)
	html := readTestFileString(t, filepath.Join(cwd, "report.html"))
	var reportScan scanReport
	if err := json.Unmarshal([]byte(extractEmbeddedJSON(t, html, "architon-scan-json")), &reportScan); err != nil {
		t.Fatalf("embedded scan JSON invalid: %v", err)
	}
	reportGraph := parseGraphOutput(t, extractEmbeddedJSON(t, html, "architon-graph-json"))
	if reportScan.Summary.UserContractsLoaded != 1 || len(reportScan.Findings) != 2 || reportGraph.Summary.Findings != 2 || reportGraph.Summary.UserContractsLoaded != 1 {
		t.Fatalf("expected report embedded generated-netlist findings, scan=%+v graph=%+v", reportScan.Summary, reportGraph.Summary)
	}
}

func TestReportCommand_StandaloneNetlistAndNoContractsAgreeCleanly(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, cwd string) string
	}{
		{
			name: "standalone netlist",
			setup: func(t *testing.T, cwd string) string {
				writeScanTestFile(t, filepath.Join(cwd, "standalone.net"), graphPullupNetlist(nil))
				return "standalone.net"
			},
		},
		{
			name: "project without contracts file",
			setup: func(t *testing.T, cwd string) string {
				writeScanTestFile(t, filepath.Join(cwd, "design.net"), graphPullupNetlist(nil))
				return "."
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cwd := t.TempDir()
			input := tt.setup(t, cwd)
			stdout, err := runScanCommand(t, cwd, input, "--format", "json", "--out", "scan.json")
			if err != nil {
				t.Fatalf("expected scan to be clean, got %v\n%s", err, stdout)
			}
			scan := readScanReport(t, filepath.Join(cwd, "scan.json"))
			if scan.Summary.UserContractsLoaded != 0 || scan.Summary.ActiveUserRequirements != 0 || len(scan.Findings) != 0 || scan.Summary.HasFailures {
				t.Fatalf("expected no contract findings, got summary=%+v findings=%+v", scan.Summary, scan.Findings)
			}
			graphStdout, err := runGraphCommand(t, cwd, input, "--format", "json")
			if err != nil {
				t.Fatalf("expected graph to be clean, got %v\n%s", err, graphStdout)
			}
			if graph := parseGraphOutput(t, graphStdout); graph.Summary.Findings != 0 || graph.Summary.Violations != 0 || graph.Summary.UserContractsLoaded != 0 || graph.Summary.ActiveUserRequirements != 0 || graph.Summary.HasFailures {
				t.Fatalf("expected graph to be clean, got %+v", graph.Summary)
			}
			reportStdout, err := runReportCommand(t, cwd, input, "--format", "html", "--out", "report.html")
			if err != nil {
				t.Fatalf("expected report to be clean, got %v\n%s", err, reportStdout)
			}
			html := readTestFileString(t, filepath.Join(cwd, "report.html"))
			var embedded scanReport
			if err := json.Unmarshal([]byte(extractEmbeddedJSON(t, html, "architon-scan-json")), &embedded); err != nil {
				t.Fatalf("embedded scan JSON invalid: %v", err)
			}
			if embedded.Summary.UserContractsLoaded != 0 || embedded.Summary.ActiveUserRequirements != 0 || len(embedded.Findings) != 0 {
				t.Fatalf("expected embedded scan to be clean, got summary=%+v findings=%+v", embedded.Summary, embedded.Findings)
			}
		})
	}
}

func TestReportCommand_PullupFindingVariantsStayConsistent(t *testing.T) {
	tests := []struct {
		name      string
		resistors []graphResistor
		wantNets  []string
	}{
		{
			name: "too strong",
			resistors: []graphResistor{
				{Ref: "R1", Value: "1k", A: "/I2C_SDA", B: "/+3V3"},
				{Ref: "R2", Value: "1k", A: "/I2C_SCL", B: "/+3V3"},
			},
			wantNets: []string{"/I2C_SCL", "/I2C_SDA"},
		},
		{
			name: "one line broken",
			resistors: []graphResistor{
				{Ref: "R2", Value: "4.7k", A: "/I2C_SCL", B: "/+3V3"},
			},
			wantNets: []string{"/I2C_SDA"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := writeGraphPullupFixture(t, tt.resistors, true)
			scanStdout, scanErr := runScanCommand(t, cwd, ".", "--format", "json", "--out", "scan.json")
			requireExitCode(t, scanErr, 2, scanStdout)
			scan := readScanReport(t, filepath.Join(cwd, "scan.json"))
			if len(scan.Findings) != len(tt.wantNets) {
				t.Fatalf("expected %d scan findings, got %+v", len(tt.wantNets), scan.Findings)
			}
			graphStdout, err := runGraphCommand(t, cwd, ".", "--format", "json")
			if err != nil {
				t.Fatalf("expected graph command to succeed, got %v\n%s", err, graphStdout)
			}
			graph := parseGraphOutput(t, graphStdout)
			if graph.Summary.Findings != len(tt.wantNets) || graph.Summary.Violations != len(tt.wantNets) {
				t.Fatalf("expected graph findings to match scan, got %+v", graph.Summary)
			}
			reportStdout, reportErr := runReportCommand(t, cwd, ".", "--format", "html", "--out", "report.html")
			requireExitCode(t, reportErr, 2, reportStdout)
			html := readTestFileString(t, filepath.Join(cwd, "report.html"))
			reportGraph := parseGraphOutput(t, extractEmbeddedJSON(t, html, "architon-graph-json"))
			if reportGraph.Summary.Findings != len(tt.wantNets) || reportGraph.Summary.Violations != len(tt.wantNets) {
				t.Fatalf("expected embedded graph findings to match, got %+v", reportGraph.Summary)
			}
			for _, net := range tt.wantNets {
				requireViolatingEdgeForNet(t, reportGraph, net)
			}
			if len(tt.wantNets) == 1 {
				for _, edge := range reportGraph.Edges {
					if edge.Net != tt.wantNets[0] && edge.Violations != 0 {
						t.Fatalf("expected only %s to carry violation linkage, got edge %+v", tt.wantNets[0], edge)
					}
				}
			}
		})
	}
}

func requireExitCode(t *testing.T, err error, want int, stdout string) {
	t.Helper()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != want {
		t.Fatalf("expected exit code %d, got err=%v stdout=%s", want, err, stdout)
	}
}

func readTestFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func extractEmbeddedJSON(t *testing.T, html string, id string) string {
	t.Helper()
	startTag := `<script type="application/json" id="` + id + `">`
	start := strings.Index(html, startTag)
	if start < 0 {
		t.Fatalf("missing embedded JSON tag %s", id)
	}
	start += len(startTag)
	end := strings.Index(html[start:], "</script>")
	if end < 0 {
		t.Fatalf("missing embedded JSON closing tag %s", id)
	}
	return html[start : start+end]
}
