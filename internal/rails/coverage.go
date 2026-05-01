package rails

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
)

const (
	OverallHigh    = "HIGH"
	OverallMedium  = "MEDIUM"
	OverallLow     = "LOW"
	OverallUnknown = "UNKNOWN"
)

const (
	WarningLowConfidence       = "some rails have low-confidence voltage inference"
	WarningUnknownVoltage      = "some rails have unknown voltage and cannot be checked by voltage rules"
	WarningIncompleteCoverage  = "rail voltage coverage is incomplete; voltage checks may miss issues"
	WarningConflictingEvidence = "some rails have conflicting voltage evidence"
)

type RailCoverageSummary struct {
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
}

func SummarizeRailCoverage(design *ir.DesignIR, inferences []infer.VoltageInference) RailCoverageSummary {
	totalNets := 0
	netNames := make([]string, 0)
	if design != nil {
		totalNets = len(design.Nets)
		netNames = make([]string, 0, len(design.Nets))
		for _, net := range design.Nets {
			netNames = append(netNames, strings.TrimSpace(net.Name))
		}
	}

	byNet := make(map[string]infer.VoltageInference, len(inferences))
	for _, inference := range inferences {
		netName := strings.TrimSpace(inference.NetName)
		if netName == "" {
			continue
		}
		byNet[netName] = inference
	}

	summary := RailCoverageSummary{
		TotalNets:    totalNets,
		OverallLevel: OverallUnknown,
		Warnings:     []string{},
	}

	hasLowConfidenceInference := false
	hasConflictingEvidence := false
	usableForRules := 0

	for _, netName := range netNames {
		inference, ok := byNet[netName]
		if !ok {
			summary.RailsUnknown++
			summary.UnknownConfidence++
			continue
		}

		if inference.Voltage == nil {
			summary.RailsUnknown++
		} else {
			summary.RailsWithVoltage++
		}

		switch strings.ToUpper(strings.TrimSpace(inference.ConfidenceLevel)) {
		case infer.ConfidenceHigh:
			summary.HighConfidence++
			if inference.Voltage != nil {
				usableForRules++
			}
		case infer.ConfidenceMedium:
			summary.MediumConfidence++
			if inference.Voltage != nil {
				usableForRules++
			}
		case infer.ConfidenceLow:
			summary.LowConfidence++
			hasLowConfidenceInference = true
		default:
			summary.UnknownConfidence++
		}

		if hasConflictWarning(inference.Warnings) {
			hasConflictingEvidence = true
		}
	}

	if totalNets > 0 {
		summary.CoverageRatio = ratio(summary.RailsWithVoltage, totalNets)
		summary.HighConfidenceRatio = ratio(summary.HighConfidence, totalNets)
		summary.UsableForRulesRatio = ratio(usableForRules, totalNets)
		summary.OverallLevel = overallLevel(summary.UsableForRulesRatio)
	}

	if hasLowConfidenceInference {
		summary.Warnings = append(summary.Warnings, WarningLowConfidence)
	}
	if summary.RailsUnknown > 0 {
		summary.Warnings = append(summary.Warnings, WarningUnknownVoltage)
	}
	if totalNets > 0 && summary.UsableForRulesRatio < 0.70 {
		summary.Warnings = append(summary.Warnings, WarningIncompleteCoverage)
	}
	if hasConflictingEvidence {
		summary.Warnings = append(summary.Warnings, WarningConflictingEvidence)
	}

	return summary
}

func FormatRailCoverage(summary RailCoverageSummary) string {
	return fmt.Sprintf("%s %s", normalizeOverallLevel(summary.OverallLevel), formatPercent(summary.UsableForRulesRatio))
}

func FormatRailExplanations(inferences []infer.VoltageInference, summary RailCoverageSummary) string {
	var b strings.Builder

	sorted := append([]infer.VoltageInference(nil), inferences...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].NetName != sorted[j].NetName {
			return sorted[i].NetName < sorted[j].NetName
		}
		return sorted[i].Source < sorted[j].Source
	})

	b.WriteString("Rail inference:\n")
	for _, inference := range sorted {
		voltage := "UNKNOWN"
		if inference.Voltage != nil {
			voltage = fmt.Sprintf("%.2fV", *inference.Voltage)
		}
		b.WriteString(fmt.Sprintf("- %s%s%-6s %-6s %-4.2f  %s\n",
			formatRailNetLabel(inference.NetName),
			railNetLabelSpacing(inference.NetName),
			voltage,
			normalizeConfidenceLevel(inference.ConfidenceLevel),
			inference.ConfidenceScore,
			inference.Source,
		))
		for _, warning := range stableWarningLines(inference.Warnings) {
			b.WriteString(fmt.Sprintf("  warning: %s\n", warning))
		}
	}

	usableForRules := int(math.Round(summary.UsableForRulesRatio * float64(summary.TotalNets)))
	b.WriteString("\n")
	b.WriteString("Rail coverage:\n")
	b.WriteString(fmt.Sprintf("- Total nets: %d\n", summary.TotalNets))
	b.WriteString(fmt.Sprintf("- Usable for rules: %d/%d\n", usableForRules, summary.TotalNets))
	b.WriteString(fmt.Sprintf("- Coverage: %s\n", FormatRailCoverage(summary)))

	return b.String()
}

func formatRailNetLabel(netName string) string {
	return strings.TrimSpace(netName) + ":"
}

func railNetLabelSpacing(netName string) string {
	labelWidth := len(formatRailNetLabel(netName))
	spaces := 1
	if labelWidth <= 4 {
		spaces = 3
	}
	return strings.Repeat(" ", spaces)
}

func ratio(count int, total int) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round((float64(count)/float64(total))*10000) / 10000
}

func stableWarningLines(warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		out = append(out, warning)
	}
	sort.Strings(out)
	return out
}

func overallLevel(usableForRulesRatio float64) string {
	switch {
	case usableForRulesRatio >= 0.90:
		return OverallHigh
	case usableForRulesRatio >= 0.70:
		return OverallMedium
	case usableForRulesRatio >= 0.40:
		return OverallLow
	default:
		return OverallUnknown
	}
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.0f%%", math.Round(value*100))
}

func normalizeOverallLevel(level string) string {
	level = strings.ToUpper(strings.TrimSpace(level))
	switch level {
	case OverallHigh, OverallMedium, OverallLow:
		return level
	default:
		return OverallUnknown
	}
}

func normalizeConfidenceLevel(level string) string {
	level = strings.ToUpper(strings.TrimSpace(level))
	switch level {
	case infer.ConfidenceHigh, infer.ConfidenceMedium, infer.ConfidenceLow:
		return level
	default:
		return infer.ConfidenceUnknown
	}
}

func hasConflictWarning(warnings []string) bool {
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning), "conflicting voltage evidence") {
			return true
		}
	}
	return false
}
