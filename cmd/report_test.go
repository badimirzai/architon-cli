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
