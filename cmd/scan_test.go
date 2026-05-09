package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	contractspkg "github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
	reportpkg "github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/ui"
)

type scanReport struct {
	ReportVersion string `json:"report_version"`
	Summary       struct {
		Parts                          int      `json:"parts"`
		Nets                           int      `json:"nets"`
		HasFailures                    bool     `json:"has_failures"`
		ParseErrorsCount               int      `json:"parse_errors_count"`
		ParseWarnings                  []string `json:"parse_warnings"`
		ParseErrors                    []string `json:"parse_errors"`
		PartsMatched                   int      `json:"parts_matched"`
		UserContractsLoaded            int      `json:"user_contracts_loaded"`
		BuiltInContractsLoaded         int      `json:"built_in_contracts_loaded"`
		ActiveUserRequirements         int      `json:"active_user_requirements"`
		AvailableContractRules         int      `json:"available_contract_rules"`
		RequirementsEnabled            int      `json:"requirements_enabled"`
		PartContractCoveragePercentage float64  `json:"part_contract_coverage_percentage"`
		UnknownPowerCriticalRefs       []string `json:"unknown_power_critical_refs"`
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
	Findings []scanRuleFinding `json:"findings"`
	Rules    []scanRuleFinding `json:"rules"`
	Derived  *struct {
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

type scanRuleFinding struct {
	ID           string `json:"id"`
	RuleID       string `json:"rule_id"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ComponentRef string `json:"component_ref"`
	Net          string `json:"net"`
	Pin          string `json:"pin"`
	BusID        string `json:"bus_id"`
	BusType      string `json:"bus_type"`
	BusNets      *struct {
		SDA string `json:"sda"`
		SCL string `json:"scl"`
	} `json:"bus_nets"`
	EffectivePullupOhms *float64 `json:"effective_pullup_ohms"`
	MinPullupOhms       *float64 `json:"min_pullup_ohms"`
	MaxPullupOhms       *float64 `json:"max_pullup_ohms"`
	PullupResistors     []string `json:"pullup_resistors"`
	Source              string   `json:"source"`
	ContractID          string   `json:"contract_id"`
	ContractSource      string   `json:"contract_source"`
	ContractFile        string   `json:"contract_file"`
	Requirement         string   `json:"requirement"`
	Provenance          *struct {
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
}

type scanCIOutput struct {
	ReportVersion string `json:"report_version"`
	RVVersion     string `json:"rv_version"`
	Summary       struct {
		InputPath              string   `json:"input_path"`
		Source                 string   `json:"source"`
		Violations             int      `json:"violations"`
		Warnings               int      `json:"warnings"`
		Infos                  int      `json:"infos"`
		HasFailures            bool     `json:"has_failures"`
		ContractsLoaded        int      `json:"contracts_loaded"`
		UserContractsLoaded    int      `json:"user_contracts_loaded"`
		BuiltInContractsLoaded int      `json:"built_in_contracts_loaded"`
		ContractCoveragePct    float64  `json:"contract_coverage_pct"`
		RulesEnabled           []string `json:"rules_enabled"`
	} `json:"summary"`
	Findings []scanCIFindingOutput `json:"findings"`
}

type scanCIFindingOutput struct {
	ID             string `json:"id"`
	RuleID         string `json:"rule_id"`
	ContractID     string `json:"contract_id"`
	ContractSource string `json:"contract_source"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	ComponentRef   string `json:"component_ref"`
	Net            string `json:"net"`
	Pin            string `json:"pin"`
	Requirement    string `json:"requirement"`
	Fix            string `json:"fix"`
	Provenance     string `json:"provenance"`
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

func TestScan_FormatJSONEmitsCIReport(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "full-report.json")
	netlist := rootFixturePath(t, filepath.Join("esp32_overvoltage", "netlist.net"))
	metaPath := rootFixturePath(t, filepath.Join("esp32_overvoltage", "meta.yaml"))

	stdout, err := runScanCommand(t, tmpDir, netlist, "--meta", metaPath, "--out", reportPath, "--format", "json")
	if err == nil {
		t.Fatal("expected overvoltage exit")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2 to be preserved for violations, got %d", exitErr.Code)
	}
	if strings.Contains(stdout, "ARCHITON SCAN") || strings.Contains(stdout, "Wrote ") {
		t.Fatalf("expected clean JSON stdout, got %q", stdout)
	}

	var payload scanCIOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("expected JSON output to parse: %v\n%s", err, stdout)
	}
	if payload.ReportVersion != "1" {
		t.Fatalf("expected report_version 1, got %q", payload.ReportVersion)
	}
	if payload.RVVersion == "" {
		t.Fatalf("expected rv_version, got %+v", payload)
	}
	if payload.Summary.InputPath != netlist {
		t.Fatalf("expected input path %q, got %q", netlist, payload.Summary.InputPath)
	}
	if payload.Summary.Violations != 1 || !payload.Summary.HasFailures {
		t.Fatalf("expected one failed violation summary, got %+v", payload.Summary)
	}
	if len(payload.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", payload.Findings)
	}
	finding := payload.Findings[0]
	if finding.ContractID != "ESP32-WROOM-32" {
		t.Fatalf("expected contract_id ESP32-WROOM-32, got %+v", finding)
	}
	if finding.ContractSource != "built_in" {
		t.Fatalf("expected built_in contract source, got %+v", finding)
	}
	if finding.ComponentRef != "U1" || finding.Net != "/+5V" || finding.Pin != "VDD" {
		t.Fatalf("expected component/net/pin context, got %+v", finding)
	}
}

func TestScan_FormatMarkdownContainsViolatedContractID(t *testing.T) {
	tmpDir := t.TempDir()
	netlist := rootFixturePath(t, filepath.Join("esp32_overvoltage", "netlist.net"))
	metaPath := rootFixturePath(t, filepath.Join("esp32_overvoltage", "meta.yaml"))

	stdout, err := runScanCommand(t, tmpDir, netlist, "--meta", metaPath, "--format", "markdown")
	if err == nil {
		t.Fatal("expected overvoltage exit")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %v", err, err)
	}
	if !strings.Contains(stdout, "# Architon Hardware Contract Review\n") {
		t.Fatalf("expected markdown title, got %q", stdout)
	}
	if !strings.Contains(stdout, "| ERROR | ESP32-WROOM-32 | U1 | /+5V |") {
		t.Fatalf("expected violated contract ID in markdown table, got %q", stdout)
	}
	if !strings.Contains(stdout, "Exit codes: 0 clean/info only, 1 warnings, 2 violations, 3 tool/import/internal failure.") {
		t.Fatalf("expected exit-code footer, got %q", stdout)
	}
}

func TestScan_FormatGitHubEmitsAnnotations(t *testing.T) {
	tmpDir := t.TempDir()
	netlist := rootFixturePath(t, filepath.Join("esp32_overvoltage", "netlist.net"))
	metaPath := rootFixturePath(t, filepath.Join("esp32_overvoltage", "meta.yaml"))

	stdout, err := runScanCommand(t, tmpDir, netlist, "--meta", metaPath, "--format", "github")
	if err == nil {
		t.Fatal("expected overvoltage exit")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %v", err, err)
	}
	if !strings.Contains(stdout, "::error title=ARCHITON CONTRACT VIOLATION::") {
		t.Fatalf("expected GitHub error annotation, got %q", stdout)
	}
	if !strings.Contains(stdout, "contract_id=ESP32-WROOM-32") || !strings.Contains(stdout, "component=U1") || !strings.Contains(stdout, "net=/+5V") {
		t.Fatalf("expected annotation context, got %q", stdout)
	}
	if strings.Contains(stdout, "ARCHITON SCAN") || strings.Contains(stdout, "Wrote ") {
		t.Fatalf("expected annotation-only stdout, got %q", stdout)
	}
}

func TestScan_KiCadBuiltInPinAliasesAcrossParts(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	netlistPath := filepath.Join(tmpDir, "pin_aliases.net")
	metaPath := filepath.Join(tmpDir, "meta.yaml")

	writeScanTestFile(t, netlistPath, `(export (version "E")
  (design
    (source "pin_aliases.kicad_sch")
    (date "2026-05-05T00:00:00+0000")
    (tool "Eeschema")
    (sheet (number "1") (name "/") (tstamps "/")))
  (components
    (comp (ref "J1")
      (value "Power")
      (fields (field (name "MPN") "POWER_HEADER"))
      (libsource (lib "Connector") (part "Conn_01x03")))
    (comp (ref "U1")
      (value "ESP32-WROOM-32")
      (fields (field (name "MPN") "ESP32-WROOM-32"))
      (libsource (lib "RF_Module") (part "ESP32-WROOM-32")))
    (comp (ref "U2")
      (value "TB6612FNG")
      (fields (field (name "MPN") "TB6612FNG"))
      (libsource (lib "Driver_Motor") (part "TB6612FNG")))
    (comp (ref "U3")
      (value "MPU-6050")
      (fields (field (name "MPN") "MPU-6050"))
      (libsource (lib "Sensor_Motion") (part "MPU-6050")))
    (comp (ref "U4")
      (value "LSM9DS1")
      (fields (field (name "MPN") "LSM9DS1"))
      (libsource (lib "Sensor_Motion") (part "LSM9DS1"))))
  (libparts
    (libpart (lib "Connector") (part "Conn_01x03")
      (pins
        (pin (num "1") (name "Pin_1") (type "passive"))
        (pin (num "2") (name "Pin_2") (type "passive"))
        (pin (num "3") (name "Pin_3") (type "passive"))))
    (libpart (lib "RF_Module") (part "ESP32-WROOM-32")
      (pins
        (pin (num "1") (name "VDD") (type "power_in"))
        (pin (num "2") (name "GND") (type "power_in"))))
    (libpart (lib "Driver_Motor") (part "TB6612FNG")
      (pins
        (pin (num "1") (name "VM1") (type "power_in"))
        (pin (num "2") (name "VM2") (type "power_in"))
        (pin (num "3") (name "VM3") (type "power_in"))
        (pin (num "4") (name "GND") (type "power_in"))))
    (libpart (lib "Sensor_Motion") (part "MPU-6050")
      (pins
        (pin (num "1") (name "VDD") (type "power_in"))
        (pin (num "2") (name "VLOGIC") (type "power_in"))
        (pin (num "3") (name "GND") (type "power_in"))))
    (libpart (lib "Sensor_Motion") (part "LSM9DS1")
      (pins
        (pin (num "1") (name "VDD") (type "power_in"))
        (pin (num "2") (name "GND") (type "power_in")))))
  (libraries)
  (nets
    (net (code "1") (name "/+5V") (class "Default")
      (node (ref "J1") (pin "1") (pinfunction "Pin_1") (pintype "passive"))
      (node (ref "U1") (pin "1") (pinfunction "VDD") (pintype "power_in")))
    (net (code "2") (name "+3V3") (class "Default")
      (node (ref "J1") (pin "2") (pinfunction "Pin_2") (pintype "passive"))
      (node (ref "U2") (pin "1") (pinfunction "VM1") (pintype "power_in"))
      (node (ref "U2") (pin "2") (pinfunction "VM2") (pintype "power_in"))
      (node (ref "U2") (pin "3") (pinfunction "VM3") (pintype "power_in"))
      (node (ref "U3") (pin "1") (pinfunction "VDD") (pintype "power_in"))
      (node (ref "U3") (pin "2") (pinfunction "VLOGIC") (pintype "power_in"))
      (node (ref "U4") (pin "1") (pinfunction "VDD") (pintype "power_in")))
    (net (code "3") (name "GND") (class "Default")
      (node (ref "J1") (pin "3") (pinfunction "Pin_3") (pintype "passive"))
      (node (ref "U1") (pin "2") (pinfunction "GND") (pintype "power_in"))
      (node (ref "U2") (pin "4") (pinfunction "GND") (pintype "power_in"))
      (node (ref "U3") (pin "3") (pinfunction "GND") (pintype "power_in"))
      (node (ref "U4") (pin "2") (pinfunction "GND") (pintype "power_in")))))`)
	writeScanTestFile(t, metaPath, `version: "0"

sources:
  - net: /+5V
    voltage: 5.0
  - net: +3V3
    voltage: 3.3
`)

	_, err := runScanCommand(t, tmpDir, netlistPath, "--meta", metaPath, "--out", reportPath)
	if err == nil {
		t.Fatal("expected contract violations")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}

	report := readScanReport(t, reportPath)
	if report.Summary.PartsMatched != 3 {
		t.Fatalf("expected ESP32, TB6612FNG, and MPU-6050 to match, got parts_matched=%d", report.Summary.PartsMatched)
	}
	if !stringSliceContains(report.Summary.UnknownPowerCriticalRefs, "U4") {
		t.Fatalf("expected LSM9DS1 U4 to remain unknown power-critical, got %+v", report.Summary.UnknownPowerCriticalRefs)
	}
	requireReportFinding(t, report, "supply_abs_max", "U1", "/+5V")
	if got := countReportFindings(report, "motor_driver_vm_range", "U2", "+3V3"); got != 3 {
		t.Fatalf("expected TB6612 VM1/VM2/VM3 findings, got %d in %+v", got, report.Rules)
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
	if strings.Contains(stdout, "Errors:") {
		t.Fatalf("expected errors line to be hidden by default, got %q", stdout)
	}
	if strings.Contains(stdout, "Warnings:") {
		t.Fatalf("expected warnings line to be hidden by default, got %q", stdout)
	}
	if !strings.Contains(stdout, "Part contract coverage:") {
		t.Fatalf("expected part contract coverage line, got %q", stdout)
	}
	if strings.Contains(stdout, "Available contract rules:") {
		t.Fatalf("expected available contract rules to be hidden by default, got %q", stdout)
	}
	if strings.Contains(stdout, "Enabled contract rules:") {
		t.Fatalf("expected enabled contract rules to be hidden by default, got %q", stdout)
	}
	if strings.Contains(stdout, "Unknown power-critical refs:") {
		t.Fatalf("expected unknown power-critical refs to be hidden by default, got %q", stdout)
	}
	if strings.Contains(stdout, "Inferred voltages:") {
		t.Fatalf("expected inferred voltages to be hidden by default, got %q", stdout)
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

func TestScan_VerboseShowsInternalSummaryLines(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "bom_minimal.csv"), "--verbose")
	if err != nil {
		t.Fatalf("expected clean scan to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Available contract rules:") {
		t.Fatalf("expected available contract rules in verbose output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Errors: 0\n") {
		t.Fatalf("expected errors line in verbose output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Warnings: 0\n") {
		t.Fatalf("expected warnings line in verbose output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Enabled contract rules:") {
		t.Fatalf("expected enabled contract rules in verbose output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Unknown power-critical refs:") {
		t.Fatalf("expected unknown power-critical refs in verbose output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Inferred voltages:") {
		t.Fatalf("expected inferred voltages in verbose output, got %q", stdout)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Summary.AvailableContractRules == 0 {
		t.Fatalf("expected available contract rules to remain in JSON summary, got %+v", report.Summary)
	}
}

func requireReportFinding(t *testing.T, report scanReport, ruleID string, ref string, net string) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.RuleID == ruleID && finding.ComponentRef == ref && finding.Net == net {
			if finding.Pin == "" || finding.Source == "" || finding.Provenance == nil || finding.Fix == "" {
				t.Fatalf("expected complete report finding, got %+v", finding)
			}
			return
		}
	}
	t.Fatalf("expected %s finding for %s on %s, got %+v", ruleID, ref, net, report.Findings)
}

func countReportFindings(report scanReport, ruleID string, ref string, net string) int {
	count := 0
	for _, finding := range report.Findings {
		if finding.RuleID == ruleID && finding.ComponentRef == ref && finding.Net == net {
			count++
		}
	}
	return count
}

func requireReportFindingByRule(t *testing.T, report scanReport, ruleID string) scanRuleFinding {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.RuleID == ruleID {
			return finding
		}
	}
	t.Fatalf("expected %s finding, got %+v", ruleID, report.Findings)
	return scanRuleFinding{}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
	if strings.Contains(stdout, "Detected BOM: ") {
		t.Fatalf("expected detected BOM message to be hidden by default, got %q", stdout)
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
	if strings.Contains(stdout, "Errors:") {
		t.Fatalf("expected errors line to be hidden by default, got %q", stdout)
	}
	if strings.Contains(stdout, "Warnings:") {
		t.Fatalf("expected warnings line to be hidden by default, got %q", stdout)
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

func TestScan_WithoutContractsYAMLStillWorks(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, "netlist_simple.net"))
	if err != nil {
		t.Fatalf("expected scan without contracts.yaml to succeed, got %v", err)
	}
	if strings.Contains(stdout, "contracts load failed") {
		t.Fatalf("did not expect contracts load attempt to fail, got %q", stdout)
	}
}

func TestScan_DirectoryAutoLoadsArchitonContractsYAML(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "design.net"), graphPullupNetlist(nil))
	writeScanTestFile(t, filepath.Join(tmpDir, ".architon", "contracts.yaml"), graphPullupContracts())

	stdout, err := runScanCommand(t, tmpDir, ".", "--format", "json", "--out", "scan.json")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected auto-loaded contract violation exit 2, got err=%v stdout=%s", err, stdout)
	}
	var scan scanCIOutput
	if err := json.Unmarshal([]byte(stdout), &scan); err != nil {
		t.Fatalf("scan output is not valid JSON: %v\n%s", err, stdout)
	}
	if scan.Summary.UserContractsLoaded != 1 || scan.Summary.Violations == 0 || len(scan.Findings) == 0 {
		t.Fatalf("expected .architon/contracts.yaml findings, got %+v", scan)
	}
}

func TestScan_ContractsFlagOverridesArchitonContractsYAML(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "design.net"), graphPullupNetlist(nil))
	writeScanTestFile(t, filepath.Join(tmpDir, ".architon", "contracts.yaml"), graphPullupContracts())
	writeScanTestFile(t, filepath.Join(tmpDir, "custom.yaml"), `contracts:
  - id: address_policy
    scope:
      bus_type: i2c
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	stdout, err := runScanCommand(t, tmpDir, ".", "--contracts", "custom.yaml", "--format", "json", "--out", "scan.json")
	if err != nil {
		t.Fatalf("expected custom contracts override to scan cleanly, got %v\n%s", err, stdout)
	}
	var scan scanCIOutput
	if err := json.Unmarshal([]byte(stdout), &scan); err != nil {
		t.Fatalf("scan output is not valid JSON: %v\n%s", err, stdout)
	}
	if scan.Summary.UserContractsLoaded != 1 || scan.Summary.Violations != 0 || len(scan.Findings) != 0 {
		t.Fatalf("expected custom contracts to override default .architon contract, got %+v", scan)
	}
}

func TestScan_DirectoryWithoutArchitonContractsYAMLIgnoresRootContractsYAML(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "design.net"), graphPullupNetlist(nil))
	writeScanTestFile(t, filepath.Join(tmpDir, "contracts.yaml"), graphPullupContracts())

	stdout, err := runScanCommand(t, tmpDir, ".", "--format", "json", "--out", "scan.json")
	if err != nil {
		t.Fatalf("expected scan without .architon/contracts.yaml to ignore root contracts.yaml, got %v\n%s", err, stdout)
	}
	var scan scanCIOutput
	if err := json.Unmarshal([]byte(stdout), &scan); err != nil {
		t.Fatalf("scan output is not valid JSON: %v\n%s", err, stdout)
	}
	if scan.Summary.UserContractsLoaded != 0 || scan.Summary.Violations != 0 || len(scan.Findings) != 0 {
		t.Fatalf("expected no auto-loaded root contracts or findings, got %+v", scan)
	}
}

func TestScan_UserYAMLContractFindingProvenance(t *testing.T) {
	tmpDir := t.TempDir()
	netlistPath := filepath.Join(tmpDir, "i2c.net")
	contractsPath := filepath.Join(tmpDir, "contracts.yaml")
	reportPath := filepath.Join(tmpDir, "report.json")
	writeScanTestFile(t, contractsPath, `contracts:
  - id: i2c_policy
    scope:
      bus_type: i2c
      bus_id: i2c_main
      nets:
        sda: I2C_SDA
        scl: I2C_SCL
    require:
      pullup_ohms:
        min: 2200
        max: 10000
      no_i2c_address_conflict: true
    severity: error
`)
	writeScanTestFile(t, netlistPath, `(export (version "E")
  (design
    (source "i2c.kicad_sch")
    (date "2026-05-05T00:00:00+0000")
    (tool "Eeschema")
    (sheet (number "1") (name "/") (tstamps "/")))
  (components
    (comp (ref "U1")
      (value "SensorA")
      (fields
        (field (name "i2c_address") "0x68"))
      (libsource (lib "Device") (part "SensorA")))
    (comp (ref "U2")
      (value "SensorB")
      (fields
        (field (name "i2c_address") "104"))
      (libsource (lib "Device") (part "SensorB"))))
  (libparts
    (libpart (lib "Device") (part "SensorA")
      (pins
        (pin (num "1") (name "SDA") (type "bidirectional"))
        (pin (num "2") (name "SCL") (type "bidirectional"))
        (pin (num "3") (name "GND") (type "power_in"))))
    (libpart (lib "Device") (part "SensorB")
      (pins
        (pin (num "1") (name "SDA") (type "bidirectional"))
        (pin (num "2") (name "SCL") (type "bidirectional"))
        (pin (num "3") (name "GND") (type "power_in")))))
  (libraries)
  (nets
    (net (code "1") (name "I2C_SDA") (class "Default")
      (node (ref "U1") (pin "1") (pinfunction "SDA") (pintype "bidirectional"))
      (node (ref "U2") (pin "1") (pinfunction "SDA") (pintype "bidirectional")))
    (net (code "2") (name "I2C_SCL") (class "Default")
      (node (ref "U1") (pin "2") (pinfunction "SCL") (pintype "bidirectional"))
      (node (ref "U2") (pin "2") (pinfunction "SCL") (pintype "bidirectional")))
    (net (code "3") (name "GND") (class "Default")
      (node (ref "U1") (pin "3") (pinfunction "GND") (pintype "power_in"))
      (node (ref "U2") (pin "3") (pinfunction "GND") (pintype "power_in")))))`)

	_, err := runScanCommand(t, tmpDir, netlistPath, "--contracts", contractsPath, "--out", reportPath)
	if err == nil {
		t.Fatal("expected user contract findings")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %v", err, err)
	}

	report := readScanReport(t, reportPath)
	if !reflect.DeepEqual(report.Rules, report.Findings) {
		t.Fatalf("expected deprecated rules alias to equal canonical findings\nrules=%+v\nfindings=%+v", report.Rules, report.Findings)
	}
	if report.Summary.UserContractsLoaded != 1 {
		t.Fatalf("expected one user contract loaded, got %+v", report.Summary)
	}
	if report.Summary.ActiveUserRequirements != 2 {
		t.Fatalf("expected two active user requirements, got %+v", report.Summary)
	}
	if report.Summary.AvailableContractRules != len(contractspkg.EnabledRuleIDs()) {
		t.Fatalf("expected available contract rules to match engine rules, got %+v", report.Summary)
	}
	if report.Summary.RequirementsEnabled != 2 {
		t.Fatalf("expected legacy requirements_enabled alias to match active user requirements, got %+v", report.Summary)
	}
	if report.Summary.PartContractCoveragePercentage < 0 {
		t.Fatalf("expected part contract coverage field, got %+v", report.Summary)
	}
	finding := requireReportFindingByRule(t, report, "pullup_ohms")
	if finding.ContractID != "i2c_policy" {
		t.Fatalf("expected contract_id i2c_policy, got %+v", finding)
	}
	if finding.ContractSource != "user_yaml" {
		t.Fatalf("expected user_yaml contract_source, got %+v", finding)
	}
	if finding.ContractFile != contractsPath {
		t.Fatalf("expected contract file %q, got %+v", contractsPath, finding)
	}
	if finding.Requirement != "pullup_ohms" {
		t.Fatalf("expected requirement pullup_ohms, got %+v", finding)
	}
	if finding.BusID != "i2c_main" || finding.BusType != "i2c" {
		t.Fatalf("expected explicit bus fields, got %+v", finding)
	}
	if finding.BusNets == nil || finding.BusNets.SDA != "I2C_SDA" || finding.BusNets.SCL != "I2C_SCL" {
		t.Fatalf("expected explicit bus nets, got %+v", finding)
	}
}

func TestScan_NetlistReportsInferredAndUnknownVoltageNets(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runScanCommand(t, tmpDir, kicadFixturePath(t, filepath.Join("overvoltage", "netlist_overvoltage.net")), "--verbose")
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
	if !strings.Contains(stdout, "Rule findings:\n") {
		t.Fatalf("expected default output to include finding section, got %q", stdout)
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
	if strings.Contains(stdout, "Detected BOM: ") {
		t.Fatalf("expected detected BOM message to be hidden by default, got %q", stdout)
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
			name: "rule failure with parse warning",
			report: reportpkg.VerificationReport{
				Summary: reportpkg.Summary{ParseWarningsCount: 1},
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
			name: "parse warning only",
			report: reportpkg.VerificationReport{
				Summary: reportpkg.Summary{ParseWarningsCount: 1},
			},
			want: 1,
		},
		{
			name: "info only",
			report: reportpkg.VerificationReport{
				Rules: []reportpkg.RuleResult{
					{ID: "BOM_RULE", Severity: "INFO", Message: "note"},
				},
			},
			want: 0,
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
