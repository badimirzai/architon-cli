package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/badimirzai/robotics-verifier-cli/internal/ir"
)

const SchemaVersion = "0"

// RuleResult is reserved for deterministic verification rules over DesignIR.
type RuleResult struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Summary struct {
	Source             string   `json:"source"`
	InputFile          string   `json:"input_file"`
	Parts              int      `json:"parts"`
	Rules              int      `json:"rules"`
	HasFailures        bool     `json:"has_failures"`
	ParseErrorsCount   int      `json:"parse_errors_count"`
	ParseWarningsCount int      `json:"parse_warnings_count"`
	ParseErrors        []string `json:"parse_errors"`
	ParseWarnings      []string `json:"parse_warnings"`
}

// VerificationReport is the output schema for BOM scan results.
type VerificationReport struct {
	ReportVersion string       `json:"report_version"`
	Summary       Summary      `json:"summary"`
	DesignIR      *ir.DesignIR `json:"design_ir"`
	Rules         []RuleResult `json:"rules"`
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
			InputFile:          design.Metadata.InputFile,
			Parts:              len(design.Parts),
			Rules:              len(rules),
			HasFailures:        len(design.ParseErrors) > 0 || hasRuleFailures(rules),
			ParseErrorsCount:   len(design.ParseErrors),
			ParseWarningsCount: len(design.ParseWarnings),
			ParseErrors:        cappedMessages(design.ParseErrors, 20),
			ParseWarnings:      cappedMessages(design.ParseWarnings, 20),
		},
		DesignIR: design,
		Rules:    rules,
	}
}

// WriteVerificationReport writes report JSON to a file with stable formatting.
func WriteVerificationReport(path string, report VerificationReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}
	return nil
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
