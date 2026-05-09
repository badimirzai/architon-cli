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
	ReportVersion string          `json:"report_version"`
	RVVersion     string          `json:"rv_version"`
	Summary       scanCISummary   `json:"summary"`
	Findings      []scanCIFinding `json:"findings"`
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
	Provenance     string `json:"provenance"`
}

func scanRenderCIJSON(result report.VerificationReport, inputPath string) ([]byte, error) {
	payload := scanBuildCIReport(result, inputPath)
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
		Provenance:     scanFindingProvenance(finding),
	}
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
