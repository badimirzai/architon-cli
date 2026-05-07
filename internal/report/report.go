package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/rails"
)

const SchemaVersion = "1"

// RuleResult is the report-facing shape of a rule finding.
// The legacy id field is kept alongside rule_id for compatibility.
type RuleResult struct {
	ID                  string                `json:"id"`
	RuleID              string                `json:"rule_id,omitempty"`
	Severity            string                `json:"severity"`
	Net                 string                `json:"net,omitempty"`
	Message             string                `json:"message"`
	Provider            string                `json:"provider,omitempty"`
	Consumer            string                `json:"consumer,omitempty"`
	Ref                 string                `json:"ref,omitempty"`
	ComponentRef        string                `json:"component_ref,omitempty"`
	Pin                 string                `json:"pin,omitempty"`
	BusID               string                `json:"bus_id,omitempty"`
	BusType             string                `json:"bus_type,omitempty"`
	BusNets             *contracts.I2CBusNets `json:"bus_nets,omitempty"`
	EffectivePullupOhms *float64              `json:"effective_pullup_ohms,omitempty"`
	MinPullupOhms       *float64              `json:"min_pullup_ohms,omitempty"`
	MaxPullupOhms       *float64              `json:"max_pullup_ohms,omitempty"`
	PullupResistors     []string              `json:"pullup_resistors,omitempty"`
	Source              string                `json:"source,omitempty"`
	ContractID          string                `json:"contract_id,omitempty"`
	ContractSource      string                `json:"contract_source,omitempty"`
	ContractFile        string                `json:"contract_file,omitempty"`
	Requirement         string                `json:"requirement,omitempty"`
	Provenance          *contracts.Provenance `json:"provenance,omitempty"`
	Fix                 string                `json:"fix,omitempty"`
	Inference           *InferenceProvenance  `json:"inference,omitempty"`
}

// InferenceProvenance links a voltage-based finding back to rail inference.
type InferenceProvenance struct {
	NetName         string  `json:"net_name"`
	Source          string  `json:"source"`
	ConfidenceScore float64 `json:"confidence_score"`
	ConfidenceLevel string  `json:"confidence_level"`
	Reason          string  `json:"reason,omitempty"`
}

// Summary is the compact report header used by both JSON and CLI output.
type Summary struct {
	Source                         string   `json:"source"`
	SourceImporter                 string   `json:"source_importer,omitempty"`
	InputFile                      string   `json:"input_file"`
	Parts                          int      `json:"parts"`
	Pins                           int      `json:"pins,omitempty"`
	Rules                          int      `json:"rules"`
	HasFailures                    bool     `json:"has_failures"`
	Delimiter                      string   `json:"delimiter,omitempty"`
	ParseErrorsCount               int      `json:"parse_errors_count"`
	ParseWarningsCount             int      `json:"parse_warnings_count"`
	ParseErrors                    []string `json:"parse_errors"`
	ParseWarnings                  []string `json:"parse_warnings"`
	NextSteps                      []string `json:"next_steps,omitempty"`
	Nets                           int      `json:"nets,omitempty"`
	PartsMatched                   int      `json:"parts_matched"`
	UserContractsLoaded            int      `json:"user_contracts_loaded"`
	BuiltInContractsLoaded         int      `json:"built_in_contracts_loaded"`
	RequirementsEnabled            int      `json:"requirements_enabled"`
	PartContractCoveragePercentage float64  `json:"part_contract_coverage_percentage"`
	ContractsApplied               int      `json:"contracts_applied"`
	ContractCoveragePercentage     float64  `json:"contract_coverage_percentage"`
	UnknownPowerCriticalRefs       []string `json:"unknown_power_critical_refs,omitempty"`
	EnabledContractRules           []string `json:"enabled_contract_rules,omitempty"`
}

// VerificationReport is the output schema for scan results.
type VerificationReport struct {
	ReportVersion string       `json:"report_version"`
	Summary       Summary      `json:"summary"`
	DesignIR      *ir.DesignIR `json:"design_ir"`
	Findings      []RuleResult `json:"findings"`
	// Rules is a deprecated compatibility alias for Findings.
	Rules            []RuleResult               `json:"rules"`
	ContractCoverage *contracts.CoverageSummary `json:"contract_coverage,omitempty"`
	Derived          *Derived                   `json:"derived,omitempty"`
}

// Derived stores non-authoritative analysis data that supports findings.
type Derived struct {
	NetVoltages         []NetVoltage        `json:"net_voltages,omitempty"`
	InferredNetVoltages []NetVoltage        `json:"inferred_net_voltages"`
	UnknownVoltageNets  []UnknownVoltageNet `json:"unknown_voltage_nets"`
	// RailInferences records deterministic voltage provenance and confidence for rails.
	RailInferences []infer.VoltageInference  `json:"rail_inferences"`
	RailCoverage   rails.RailCoverageSummary `json:"rail_coverage"`
	Conflicts      []string                  `json:"conflicts,omitempty"`
}

// NetVoltage is report-facing voltage evidence for a net.
type NetVoltage struct {
	Net        string  `json:"net"`
	Voltage    float64 `json:"voltage"`
	Source     string  `json:"source"`
	Confidence string  `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

// UnknownVoltageNet explains why a rail-like net could not be assigned voltage.
type UnknownVoltageNet struct {
	Net    string `json:"net"`
	Reason string `json:"reason"`
}

// NewVerificationReport builds the deterministic JSON payload for scan.
func NewVerificationReport(design *ir.DesignIR) VerificationReport {
	if design == nil {
		design = &ir.DesignIR{Version: ir.SchemaVersion}
	}
	if design.Version == "" {
		design.Version = ir.SchemaVersion
	}

	rules := make([]RuleResult, 0)
	return VerificationReport{
		ReportVersion: SchemaVersion,
		Summary: Summary{
			Source:             design.Source,
			SourceImporter:     design.SourceInfo.Importer,
			InputFile:          design.Metadata.InputFile,
			Parts:              len(design.Parts),
			Pins:               countPins(design),
			Rules:              len(rules),
			HasFailures:        len(design.ParseErrors) > 0 || hasRuleFailures(rules),
			Delimiter:          design.Metadata.Delimiter,
			ParseErrorsCount:   len(design.ParseErrors),
			ParseWarningsCount: len(design.ParseWarnings),
			ParseErrors:        cappedMessages(design.ParseErrors, 20),
			ParseWarnings:      cappedMessages(design.ParseWarnings, 20),
			NextSteps:          nextSteps(design.ParseErrors),
			Nets:               len(design.Nets),
		},
		DesignIR: design,
		Findings: rules,
		Rules:    rules,
	}
}

// WriteVerificationReport writes report JSON to a file with stable formatting.
func WriteVerificationReport(path string, result VerificationReport) error {
	result = CanonicalizeVerificationReport(result)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}
	return nil
}

// CanonicalizeVerificationReport keeps findings canonical while preserving
// rules as a deprecated alias for one compatibility window.
func CanonicalizeVerificationReport(result VerificationReport) VerificationReport {
	if len(result.Findings) == 0 && len(result.Rules) > 0 {
		result.Findings = append([]RuleResult{}, result.Rules...)
	}
	result.Rules = append([]RuleResult{}, result.Findings...)
	return result
}

func hasRuleFailures(rules []RuleResult) bool {
	for _, rule := range rules {
		severity := strings.TrimSpace(rule.Severity)
		if severity == "" || strings.EqualFold(severity, "error") {
			return true
		}
	}
	return false
}

// countPins accepts either explicit DesignIR.Pins or net-derived PinRefs.
func countPins(design *ir.DesignIR) int {
	if design == nil {
		return 0
	}
	seen := map[string]struct{}{}
	for _, pin := range design.Pins {
		if pin.Ref == "" || pin.Pin == "" {
			continue
		}
		seen[pin.Ref+"\x00"+pin.Pin] = struct{}{}
	}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if pin.Ref == "" || pin.Pin == "" {
				continue
			}
			seen[pin.Ref+"\x00"+pin.Pin] = struct{}{}
		}
	}
	return len(seen)
}

func nextSteps(parseErrors []string) []string {
	if len(parseErrors) == 0 {
		return nil
	}

	steps := make([]string, 0, 3)
	addStep := func(step string) {
		if step == "" || len(steps) == 3 {
			return
		}
		for _, existing := range steps {
			if existing == step {
				return
			}
		}
		steps = append(steps, step)
	}

	for _, err := range parseErrors {
		if strings.Contains(err, "malformed CSV row") {
			addStep("Re-export BOM (CSV) and check missing delimiters/quotes")
			break
		}
	}
	for _, err := range parseErrors {
		if strings.Contains(err, "missing required BOM column") {
			addStep("Use --map mapping.yaml to map headers")
			break
		}
	}
	addStep("Run rv scan <bom.csv> --out report.json and inspect summary.parse_errors")

	return steps
}

func cappedMessages(messages []string, limit int) []string {
	if len(messages) == 0 {
		return []string{}
	}
	if len(messages) > limit {
		messages = messages[:limit]
	}
	out := make([]string, len(messages))
	copy(out, messages)
	return out
}
