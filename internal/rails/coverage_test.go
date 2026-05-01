package rails

import (
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestSummarizeRailCoverage_AllHighIsOverallHigh(t *testing.T) {
	summary := SummarizeRailCoverage(designWithNets("GND", "/+5V"), []infer.VoltageInference{
		inference("GND", ptr(0), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
		inference("/+5V", ptr(5), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
	})

	if summary.OverallLevel != OverallHigh {
		t.Fatalf("expected overall HIGH, got %s", summary.OverallLevel)
	}
	if summary.UsableForRulesRatio != 1 {
		t.Fatalf("expected usable ratio 1, got %v", summary.UsableForRulesRatio)
	}
	if len(summary.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", summary.Warnings)
	}
}

func TestSummarizeRailCoverage_MixedHighMediumUsableRatio(t *testing.T) {
	summary := SummarizeRailCoverage(designWithNets("/+5V", "VBUS", "VDD"), []infer.VoltageInference{
		inference("/+5V", ptr(5), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
		inference("VBUS", ptr(5), infer.SourceSemanticAlias, 0.80, infer.ConfidenceMedium),
		inference("VDD", nil, infer.SourceWeakSemanticAlias, 0.45, infer.ConfidenceLow),
	})

	if summary.UsableForRulesRatio != 0.6667 {
		t.Fatalf("expected usable ratio 0.6667, got %v", summary.UsableForRulesRatio)
	}
	if summary.HighConfidence != 1 || summary.MediumConfidence != 1 || summary.LowConfidence != 1 {
		t.Fatalf("unexpected confidence counts: %+v", summary)
	}
	if summary.OverallLevel != OverallLow {
		t.Fatalf("expected overall LOW, got %s", summary.OverallLevel)
	}
}

func TestSummarizeRailCoverage_LowConfidenceVoltageWarns(t *testing.T) {
	summary := SummarizeRailCoverage(designWithNets("AUX_5V"), []infer.VoltageInference{
		inference("AUX_5V", ptr(5), infer.SourceAmbiguousNumericToken, 0.40, infer.ConfidenceLow),
	})

	if !contains(summary.Warnings, WarningLowConfidence) {
		t.Fatalf("expected low confidence warning, got %+v", summary.Warnings)
	}
}

func TestSummarizeRailCoverage_UnknownVoltageWarns(t *testing.T) {
	summary := SummarizeRailCoverage(designWithNets("VIN"), []infer.VoltageInference{
		inference("VIN", nil, infer.SourceUnknown, 0, infer.ConfidenceUnknown),
	})

	if summary.RailsUnknown != 1 {
		t.Fatalf("expected one unknown rail, got %+v", summary)
	}
	if !contains(summary.Warnings, WarningUnknownVoltage) {
		t.Fatalf("expected unknown voltage warning, got %+v", summary.Warnings)
	}
}

func TestSummarizeRailCoverage_ZeroNetsDoesNotDivideByZero(t *testing.T) {
	summary := SummarizeRailCoverage(&ir.DesignIR{}, nil)

	if summary.TotalNets != 0 {
		t.Fatalf("expected zero total nets, got %d", summary.TotalNets)
	}
	if summary.CoverageRatio != 0 || summary.HighConfidenceRatio != 0 || summary.UsableForRulesRatio != 0 {
		t.Fatalf("expected zero ratios, got %+v", summary)
	}
	if summary.OverallLevel != OverallUnknown {
		t.Fatalf("expected UNKNOWN, got %s", summary.OverallLevel)
	}
}

func TestSummarizeRailCoverage_RatiosAreDeterministic(t *testing.T) {
	summary := SummarizeRailCoverage(designWithNets("/+5V", "VBUS", "VIN"), []infer.VoltageInference{
		inference("VIN", nil, infer.SourceUnknown, 0, infer.ConfidenceUnknown),
		inference("VBUS", ptr(5), infer.SourceSemanticAlias, 0.80, infer.ConfidenceMedium),
		inference("/+5V", ptr(5), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
	})

	if summary.CoverageRatio != 0.6667 {
		t.Fatalf("expected coverage ratio 0.6667, got %v", summary.CoverageRatio)
	}
	if summary.HighConfidenceRatio != 0.3333 {
		t.Fatalf("expected high confidence ratio 0.3333, got %v", summary.HighConfidenceRatio)
	}
	if summary.UsableForRulesRatio != 0.6667 {
		t.Fatalf("expected usable ratio 0.6667, got %v", summary.UsableForRulesRatio)
	}
}

func TestSummarizeRailCoverage_ConflictWarning(t *testing.T) {
	inf := inference("/+5V", ptr(4.7), infer.SourceUserOverride, 0.30, infer.ConfidenceUnknown)
	inf.Warnings = []string{"conflicting voltage evidence detected"}

	summary := SummarizeRailCoverage(designWithNets("/+5V"), []infer.VoltageInference{inf})

	if !contains(summary.Warnings, WarningConflictingEvidence) {
		t.Fatalf("expected conflict warning, got %+v", summary.Warnings)
	}
}

func TestFormatRailExplanations_StableSnapshot(t *testing.T) {
	got := FormatRailExplanations([]infer.VoltageInference{
		inference("GND", ptr(0), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
		inference("/VBAT", ptr(24), infer.SourceUserOverride, 1.00, infer.ConfidenceHigh),
		inference("/+5V", ptr(5), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
	}, RailCoverageSummary{
		TotalNets:           3,
		UsableForRulesRatio: 1,
		OverallLevel:        OverallHigh,
	})

	want := `Rail inference:
- /+5V: 5.00V  HIGH   0.95  NET_NAME_EXACT
- /VBAT: 24.00V HIGH   1.00  USER_OVERRIDE
- GND:   0.00V  HIGH   0.95  NET_NAME_EXACT

Rail coverage:
- Total nets: 3
- Usable for rules: 3/3
- Coverage: HIGH 100%
`
	if got != want {
		t.Fatalf("unexpected rail explanation:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestFormatRailExplanations_UnknownVoltageAndWarnings(t *testing.T) {
	vin := inference("VIN", nil, "HEURISTIC", 0.35, infer.ConfidenceLow)
	vin.Warnings = []string{
		"weak semantic alias does not identify a unique voltage",
		"generic rail name reduces confidence",
	}

	got := FormatRailExplanations([]infer.VoltageInference{vin}, RailCoverageSummary{
		TotalNets:           1,
		UsableForRulesRatio: 0,
		OverallLevel:        OverallUnknown,
	})

	if !strings.Contains(got, "- VIN:   UNKNOWN LOW    0.35  HEURISTIC\n") {
		t.Fatalf("expected unknown voltage rail line, got:\n%s", got)
	}
	if !strings.Contains(got, "  warning: generic rail name reduces confidence\n") {
		t.Fatalf("expected generic rail warning, got:\n%s", got)
	}
	if !strings.Contains(got, "  warning: weak semantic alias does not identify a unique voltage\n") {
		t.Fatalf("expected weak alias warning, got:\n%s", got)
	}
	if strings.Index(got, "generic rail name") > strings.Index(got, "weak semantic alias") {
		t.Fatalf("expected warning lines to be sorted, got:\n%s", got)
	}
}

func TestFormatRailExplanations_CoveragePercentRounding(t *testing.T) {
	got := FormatRailExplanations([]infer.VoltageInference{
		inference("/+5V", ptr(5), infer.SourceNetNameExact, 0.95, infer.ConfidenceHigh),
	}, RailCoverageSummary{
		TotalNets:           3,
		UsableForRulesRatio: 0.6667,
		OverallLevel:        OverallMedium,
	})

	if !strings.Contains(got, "- Usable for rules: 2/3\n") {
		t.Fatalf("expected rounded usable count, got:\n%s", got)
	}
	if !strings.Contains(got, "- Coverage: MEDIUM 67%\n") {
		t.Fatalf("expected rounded coverage percent, got:\n%s", got)
	}
}

func designWithNets(names ...string) *ir.DesignIR {
	nets := make([]ir.Net, 0, len(names))
	for _, name := range names {
		nets = append(nets, ir.Net{Name: name})
	}
	return &ir.DesignIR{Nets: nets}
}

func inference(net string, voltage *float64, source string, score float64, level string) infer.VoltageInference {
	return infer.VoltageInference{
		NetName:         net,
		Voltage:         voltage,
		Source:          source,
		ConfidenceScore: score,
		ConfidenceLevel: level,
		Evidence:        []string{},
		Warnings:        []string{},
	}
}

func ptr(value float64) *float64 {
	return &value
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
