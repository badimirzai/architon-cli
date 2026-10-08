package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/version"
)

type scanCIReport struct {
	ReportVersion string                  `json:"report_version"`
	RVVersion     string                  `json:"rv_version"`
	Summary       scanCISummary           `json:"summary"`
	Findings      []scanCIFinding         `json:"findings"`
	Coverage      contracts.CheckCoverage `json:"coverage"`
}

type scanCISummary struct {
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
}

type scanCIFinding struct {
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
	WhyThisMatters string `json:"why_this_matters,omitempty"`
	Provenance     string `json:"provenance"`
	// Expected and Observed are copied from the scan finding. Nil stays omitted.
	Expected *contracts.Evidence `json:"expected,omitempty"`
	Observed *contracts.Evidence `json:"observed,omitempty"`
	// Citations are datasheet locations copied from a pin-function finding.
	Citations []contracts.Citation `json:"citations,omitempty"`
	// DesignFixable is true when a schematic or contract edit can clear the finding.
	// Parse and tool failures stay false.
	DesignFixable bool `json:"design_fixable"`
}

func scanRenderCIJSON(result report.VerificationReport, inputPath string) ([]byte, error) {
	payload := scanBuildCIReport(result, inputPath)
	payload.Findings = append(payload.Findings, scanParseFailureFindings(result)...)
	payload.Coverage.Refused = scanRefusedFromCI(payload.Findings)
	return json.MarshalIndent(payload, "", "  ")
}

func scanBuildCIReport(result report.VerificationReport, inputPath string) scanCIReport {
	result = report.CanonicalizeVerificationReport(result)
	violations, findingWarnings, infos := scanFindingSeverityCounts(result.Findings)
	warnings := findingWarnings + result.Summary.ParseWarningsCount
	rulesEnabled := append([]string{}, result.Summary.EnabledContractRules...)
	sort.Strings(rulesEnabled)

	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		inputPath = result.Summary.InputFile
	}

	findings := make([]scanCIFinding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, scanBuildCIFinding(finding))
	}

	return scanCIReport{
		ReportVersion: report.SchemaVersion,
		RVVersion:     version.Get().Version,
		Coverage:      scanCopyCoverage(result.Coverage),
		Summary: scanCISummary{
			InputPath:              inputPath,
			Source:                 result.Summary.Source,
			Violations:             violations,
			Warnings:               warnings,
			Infos:                  infos,
			HasFailures:            result.Summary.HasFailures || result.Summary.ParseErrorsCount > 0 || violations > 0,
			ContractsLoaded:        result.Summary.UserContractsLoaded + result.Summary.BuiltInContractsLoaded,
			UserContractsLoaded:    result.Summary.UserContractsLoaded,
			BuiltInContractsLoaded: result.Summary.BuiltInContractsLoaded,
			ContractCoveragePct:    result.Summary.ContractCoveragePercentage,
			RulesEnabled:           rulesEnabled,
		},
		Findings: findings,
	}
}

func scanBuildCIFinding(finding report.RuleResult) scanCIFinding {
	id := strings.TrimSpace(finding.ID)
	ruleID := strings.TrimSpace(finding.RuleID)
	if id == "" {
		id = ruleID
	}
	if ruleID == "" {
		ruleID = id
	}

	componentRef := strings.TrimSpace(finding.ComponentRef)
	if componentRef == "" {
		componentRef = strings.TrimSpace(finding.Ref)
	}

	return scanCIFinding{
		ID:             id,
		RuleID:         ruleID,
		ContractID:     scanFindingContractID(finding, ruleID),
		ContractSource: scanFindingContractSource(finding),
		Severity:       normalizeSeverity(finding.Severity),
		Message:        strings.TrimSpace(finding.Message),
		ComponentRef:   componentRef,
		Net:            strings.TrimSpace(finding.Net),
		Pin:            strings.TrimSpace(finding.Pin),
		Requirement:    scanFindingRequirement(finding, ruleID),
		Fix:            strings.TrimSpace(finding.Fix),
		WhyThisMatters: strings.TrimSpace(finding.WhyThisMatters),
		Provenance:     scanFindingProvenance(finding),
		Expected:       finding.Expected,
		Observed:       finding.Observed,
		Citations:      append([]contracts.Citation(nil), finding.Citations...),
		DesignFixable:  scanFindingDesignFixable(finding),
	}
}

// scanCopyCoverage copies proved and not_checked from the scan report.
// Refused is filled from the CI findings after parse errors are appended.
func scanCopyCoverage(coverage *contracts.CheckCoverage) contracts.CheckCoverage {
	out := contracts.CheckCoverage{
		Proved:     contracts.ProvedCoverage{RuleIDs: []string{}},
		Refused:    []contracts.RefusedFinding{},
		NotChecked: []contracts.UncheckedItem{},
	}
	if coverage == nil {
		return out
	}
	out.Proved.Count = coverage.Proved.Count
	if len(coverage.Proved.RuleIDs) > 0 {
		out.Proved.RuleIDs = append([]string{}, coverage.Proved.RuleIDs...)
	}
	if len(coverage.NotChecked) > 0 {
		out.NotChecked = append([]contracts.UncheckedItem{}, coverage.NotChecked...)
	}
	return out
}

// scanRefusedFindings copies ERROR and WARN report findings into coverage.
// INFO findings and unchecked pins stay out. This does not change the exit code.
func scanRefusedFindings(findings []report.RuleResult) []contracts.RefusedFinding {
	out := make([]contracts.RefusedFinding, 0)
	for _, finding := range findings {
		severity := normalizeSeverity(finding.Severity)
		if severity != "ERROR" && severity != "WARN" {
			continue
		}
		ruleID := strings.TrimSpace(finding.RuleID)
		if ruleID == "" {
			ruleID = strings.TrimSpace(finding.ID)
		}
		ref := strings.TrimSpace(finding.ComponentRef)
		if ref == "" {
			ref = strings.TrimSpace(finding.Ref)
		}
		out = append(out, contracts.RefusedFinding{
			RuleID:       ruleID,
			Severity:     severity,
			ComponentRef: ref,
			Net:          strings.TrimSpace(finding.Net),
			Pin:          strings.TrimSpace(finding.Pin),
			Message:      strings.TrimSpace(finding.Message),
			Expected:     finding.Expected,
			Observed:     finding.Observed,
			Citations:    append([]contracts.Citation(nil), finding.Citations...),
		})
	}
	return out
}

func scanRefusedFromCI(findings []scanCIFinding) []contracts.RefusedFinding {
	out := make([]contracts.RefusedFinding, 0)
	for _, finding := range findings {
		severity := normalizeSeverity(finding.Severity)
		if severity != "ERROR" && severity != "WARN" {
			continue
		}
		out = append(out, contracts.RefusedFinding{
			RuleID:       strings.TrimSpace(finding.RuleID),
			Severity:     severity,
			ComponentRef: strings.TrimSpace(finding.ComponentRef),
			Net:          strings.TrimSpace(finding.Net),
			Pin:          strings.TrimSpace(finding.Pin),
			Message:      strings.TrimSpace(finding.Message),
			Expected:     finding.Expected,
			Observed:     finding.Observed,
			Citations:    append([]contracts.Citation(nil), finding.Citations...),
		})
	}
	return out
}

// scanFindingDesignFixable reports whether a schematic or contract edit can clear the finding.
// Parse and tool failures cannot.
func scanFindingDesignFixable(finding report.RuleResult) bool {
	ruleID := strings.TrimSpace(finding.RuleID)
	if ruleID == "" {
		ruleID = strings.TrimSpace(finding.ID)
	}
	switch ruleID {
	case "parse_error", "tool_error", "PARSER_ERROR":
		return false
	}
	return true
}

// scanParseFailureFindings turns import parse errors into JSON findings.
// They are not contract violations. design_fixable stays false.
func scanParseFailureFindings(result report.VerificationReport) []scanCIFinding {
	messages := make([]string, 0, len(result.Summary.ParseErrors))
	for _, message := range result.Summary.ParseErrors {
		message = strings.TrimSpace(message)
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 && result.Summary.ParseErrorsCount > 0 {
		messages = append(messages, "parse error")
	}
	if len(messages) == 0 {
		return nil
	}
	out := make([]scanCIFinding, 0, len(messages))
	for _, message := range messages {
		out = append(out, scanCIFinding{
			ID:            "parse_error",
			RuleID:        "parse_error",
			Severity:      "ERROR",
			Message:       message,
			DesignFixable: false,
		})
	}
	return out
}

func scanRenderMarkdown(result report.VerificationReport, inputPath string) string {
	payload := scanBuildCIReport(result, inputPath)
	var b strings.Builder
	b.WriteString("# Architon Hardware Contract Review\n\n")
	b.WriteString(fmt.Sprintf("**Status:** %s - %s, %s, %s\n\n",
		scanMarkdownStatus(payload.Summary),
		scanPlural(payload.Summary.Violations, "violation"),
		scanPlural(payload.Summary.Warnings, "warning"),
		scanPlural(payload.Summary.Infos, "info"),
	))
	b.WriteString(fmt.Sprintf("**Contract coverage:** %.2f%% (%d applied, %d user contracts loaded, %d built-in contracts loaded)\n\n",
		payload.Summary.ContractCoveragePct,
		result.Summary.ContractsApplied,
		payload.Summary.UserContractsLoaded,
		payload.Summary.BuiltInContractsLoaded,
	))

	b.WriteString("## Violations\n\n")
	scanWriteMarkdownTable(&b, payload.Findings, "ERROR", "No violations.")
	b.WriteString("\n## Warnings\n\n")
	scanWriteMarkdownTable(&b, payload.Findings, "WARN", "No warnings.")
	b.WriteString("\n## Suggested Fixes\n\n")
	scanWriteMarkdownFixes(&b, payload.Findings)
	b.WriteString("\n---\n")
	b.WriteString("Exit codes: 0 clean/info only, 1 warnings, 2 violations, 3 tool/import/internal failure.\n")
	return b.String()
}

func scanRenderGitHub(result report.VerificationReport, inputPath string) string {
	payload := scanBuildCIReport(result, inputPath)
	var b strings.Builder
	for _, finding := range payload.Findings {
		switch normalizeSeverity(finding.Severity) {
		case "ERROR":
			fmt.Fprintf(&b, "::error title=ARCHITON CONTRACT VIOLATION::%s\n", scanEscapeGitHubAnnotation(scanGitHubAnnotationMessage(finding)))
		case "WARN":
			fmt.Fprintf(&b, "::warning title=ARCHITON CONTRACT WARNING::%s\n", scanEscapeGitHubAnnotation(scanGitHubAnnotationMessage(finding)))
		}
	}
	return b.String()
}

func scanReturnExit(exitCode int) error {
	if exitCode == 0 {
		return nil
	}
	if exitCode > 0 && exitCode <= 3 {
		return silentExit(exitCode)
	}
	return &ExitError{
		Code: 3,
		Err:  fmt.Errorf("scan failed with unexpected exit code %d", exitCode),
	}
}

func scanFindingSeverityCounts(findings []report.RuleResult) (violations int, warnings int, infos int) {
	for _, finding := range findings {
		switch normalizeSeverity(finding.Severity) {
		case "ERROR":
			violations++
		case "WARN":
			warnings++
		case "INFO":
			infos++
		}
	}
	return violations, warnings, infos
}

func scanFindingContractID(finding report.RuleResult, fallback string) string {
	if id := strings.TrimSpace(finding.ContractID); id != "" {
		return id
	}
	if finding.Provenance != nil {
		if id := strings.TrimSpace(finding.Provenance.SourceID); id != "" {
			return id
		}
	}
	return strings.TrimSpace(fallback)
}

func scanFindingContractSource(finding report.RuleResult) string {
	source := strings.TrimSpace(finding.ContractSource)
	switch source {
	case string(contracts.ContractSourceBuiltIn),
		string(contracts.ContractSourceUserYAML),
		string(contracts.ContractSourceMetaYAML),
		string(contracts.ContractSourceInferred):
		return source
	}
	return string(contracts.ReportContractSource(finding.Source))
}

func scanFindingRequirement(finding report.RuleResult, fallback string) string {
	if requirement := strings.TrimSpace(finding.Requirement); requirement != "" {
		return requirement
	}
	return strings.TrimSpace(fallback)
}

func scanFindingProvenance(finding report.RuleResult) string {
	if finding.Provenance == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if source := strings.TrimSpace(finding.Provenance.Source); source != "" {
		parts = append(parts, "source="+source)
	}
	if sourceID := strings.TrimSpace(finding.Provenance.SourceID); sourceID != "" {
		parts = append(parts, "source_id="+sourceID)
	}
	if detail := strings.TrimSpace(finding.Provenance.Detail); detail != "" {
		parts = append(parts, "detail="+detail)
	}
	return strings.Join(parts, "; ")
}

func scanMarkdownStatus(summary scanCISummary) string {
	if summary.Violations > 0 {
		return "FAIL"
	}
	if summary.Warnings > 0 {
		return "WARN"
	}
	return "OK"
}

func scanWriteMarkdownTable(b *strings.Builder, findings []scanCIFinding, severity string, empty string) {
	rows := make([]scanCIFinding, 0)
	for _, finding := range findings {
		if normalizeSeverity(finding.Severity) == severity {
			rows = append(rows, finding)
		}
	}
	if len(rows) == 0 {
		b.WriteString(empty)
		b.WriteString("\n")
		return
	}
	b.WriteString("| Severity | Contract | Component | Net | Finding | Fix |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, finding := range rows {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n",
			scanEscapeMarkdownCell(finding.Severity),
			scanEscapeMarkdownCell(finding.ContractID),
			scanEscapeMarkdownCell(finding.ComponentRef),
			scanEscapeMarkdownCell(finding.Net),
			scanEscapeMarkdownCell(finding.Message),
			scanEscapeMarkdownCell(finding.Fix),
		)
	}
}

func scanWriteMarkdownFixes(b *strings.Builder, findings []scanCIFinding) {
	seen := map[string]struct{}{}
	wrote := false
	for _, finding := range findings {
		severity := normalizeSeverity(finding.Severity)
		if severity != "ERROR" && severity != "WARN" {
			continue
		}
		fix := strings.TrimSpace(finding.Fix)
		if fix == "" {
			continue
		}
		context := scanFindingContext(finding)
		key := finding.ContractID + "\x00" + context + "\x00" + fix
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		wrote = true
		fmt.Fprintf(b, "- **%s**", scanEscapeMarkdownInline(finding.ContractID))
		if context != "" {
			fmt.Fprintf(b, " (%s)", scanEscapeMarkdownInline(context))
		}
		fmt.Fprintf(b, ": %s\n", scanEscapeMarkdownInline(fix))
	}
	if !wrote {
		b.WriteString("No fixes suggested.\n")
	}
}

func scanFindingContext(finding scanCIFinding) string {
	parts := make([]string, 0, 3)
	if finding.ComponentRef != "" {
		parts = append(parts, finding.ComponentRef)
	}
	if finding.Net != "" {
		parts = append(parts, finding.Net)
	}
	if finding.Pin != "" {
		parts = append(parts, "pin "+finding.Pin)
	}
	return strings.Join(parts, ", ")
}

func scanGitHubAnnotationMessage(finding scanCIFinding) string {
	fields := []string{
		"contract_id=" + scanValueOrNA(finding.ContractID),
		"component=" + scanValueOrNA(finding.ComponentRef),
		"net=" + scanValueOrNA(finding.Net),
	}
	if finding.Pin != "" {
		fields = append(fields, "pin="+finding.Pin)
	}
	if finding.RuleID != "" {
		fields = append(fields, "rule_id="+finding.RuleID)
	}
	message := strings.TrimSpace(finding.Message)
	if message != "" {
		fields = append(fields, message)
	}
	return strings.Join(fields, "; ")
}

func scanEscapeGitHubAnnotation(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

func scanEscapeMarkdownCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

func scanEscapeMarkdownInline(s string) string {
	return scanEscapeMarkdownCell(s)
}

func scanPlural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func scanValueOrNA(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "n/a"
	}
	return value
}
