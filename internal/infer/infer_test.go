package infer

import (
	"math"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestInferVoltagesFromNetNames(t *testing.T) {
	tests := []struct {
		net     string
		voltage float64
		source  string
	}{
		{net: "/+5V", voltage: 5.0, source: SourceNetNameExact},
		{net: "3V3", voltage: 3.3, source: SourceNetNameExact},
		{net: "/+3.3V", voltage: 3.3, source: SourceNetNameExact},
		{net: "VBAT_24V", voltage: 24.0, source: SourceAmbiguousNumericToken},
		{net: "/+4.24554V", voltage: 4.24554, source: SourceNetNameExact},
		{net: "GND", voltage: 0.0, source: SourceNetNameExact},
	}

	for _, tt := range tests {
		t.Run(tt.net, func(t *testing.T) {
			result := InferVoltagesFromNetNames(&ir.DesignIR{
				Nets: []ir.Net{{Name: tt.net}},
			})

			got, ok := result.Voltages[tt.net]
			if !ok {
				t.Fatalf("expected inferred voltage for %s", tt.net)
			}
			if got.Voltage != tt.voltage {
				t.Fatalf("expected %v, got %v", tt.voltage, got.Voltage)
			}
			if got.Source != tt.source {
				t.Fatalf("expected source %s, got %q", tt.source, got.Source)
			}
			if len(result.Unknowns) != 0 {
				t.Fatalf("expected no unknowns, got %+v", result.Unknowns)
			}
		})
	}
}

func TestInferVoltagesFromNetNames_ReportsAmbiguousPowerNets(t *testing.T) {
	result := InferVoltagesFromNetNames(&ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/VBAT"},
			{Name: "VBAT"},
			{Name: "VCC"},
		},
	})

	if len(result.Voltages) != 0 {
		t.Fatalf("expected no inferred voltages, got %+v", result.Voltages)
	}

	want := []string{"/VBAT", "VBAT", "VCC"}
	if len(result.Unknowns) != len(want) {
		t.Fatalf("expected %d unknowns, got %d (%+v)", len(want), len(result.Unknowns), result.Unknowns)
	}
	for i, net := range want {
		if result.Unknowns[i].Net != net {
			t.Fatalf("expected unknown %d net %q, got %q", i, net, result.Unknowns[i].Net)
		}
		if result.Unknowns[i].Reason == "" {
			t.Fatalf("expected unknown reason")
		}
	}
}

func TestInferVoltagesFromNetNames_DoesNotInferUnlistedNames(t *testing.T) {
	result := InferVoltagesFromNetNames(&ir.DesignIR{
		Nets: []ir.Net{
			{Name: "SCL"},
			{Name: "POWER_GOOD"},
		},
	})

	if len(result.Voltages) != 0 {
		t.Fatalf("expected no inferred voltages, got %+v", result.Voltages)
	}
	if len(result.Unknowns) != 0 {
		t.Fatalf("expected no unknown voltage nets, got %+v", result.Unknowns)
	}
}

func TestInferVoltageFromNetName_ConfidenceScoring(t *testing.T) {
	tests := []struct {
		name     string
		net      string
		voltage  *float64
		source   string
		score    float64
		level    string
		evidence string
		warning  string
	}{
		{
			name:     "slash plus 5V",
			net:      "/+5V",
			voltage:  ptr(5.0),
			source:   SourceNetNameExact,
			score:    0.95,
			level:    ConfidenceHigh,
			evidence: `matched exact voltage pattern "/+5V"`,
		},
		{
			name:     "plus 5V",
			net:      "+5V",
			voltage:  ptr(5.0),
			source:   SourceNetNameExact,
			score:    0.95,
			level:    ConfidenceHigh,
			evidence: `matched exact voltage pattern "+5V"`,
		},
		{
			name:     "bare 5V",
			net:      "5V",
			voltage:  ptr(5.0),
			source:   SourceNetNameExact,
			score:    0.95,
			level:    ConfidenceHigh,
			evidence: `matched exact voltage pattern "5V"`,
		},
		{
			name:     "slash 3V3",
			net:      "/3V3",
			voltage:  ptr(3.3),
			source:   SourceNetNameExact,
			score:    0.95,
			level:    ConfidenceHigh,
			evidence: `matched exact voltage pattern "/3V3"`,
		},
		{
			name:     "VCC 3V3",
			net:      "VCC_3V3",
			voltage:  ptr(3.3),
			source:   SourceAmbiguousNumericToken,
			score:    0.00,
			level:    ConfidenceUnknown,
			evidence: `matched ambiguous voltage token "3V3" in net name "VCC_3V3"`,
			warning:  "heuristic-only voltage extraction used",
		},
		{
			name:     "VBUS",
			net:      "VBUS",
			voltage:  ptr(5.0),
			source:   SourceSemanticAlias,
			score:    0.80,
			level:    ConfidenceMedium,
			evidence: `matched known semantic alias "VBUS" = 5.00V`,
		},
		{
			name:    "VIN",
			net:     "VIN",
			source:  SourceUnknown,
			score:   0.00,
			level:   ConfidenceUnknown,
			warning: `generic rail name "VIN" reduces confidence`,
		},
		{
			name:     "VDD",
			net:      "VDD",
			source:   SourceWeakSemanticAlias,
			score:    0.45,
			level:    ConfidenceLow,
			evidence: `matched weak semantic alias "VDD"`,
			warning:  `weak semantic alias "VDD" does not identify a unique voltage`,
		},
		{
			name:     "SUPPLY 4V7",
			net:      "SUPPLY_4V7",
			voltage:  ptr(4.7),
			source:   SourceAmbiguousNumericToken,
			score:    0.00,
			level:    ConfidenceUnknown,
			evidence: `matched ambiguous voltage token "4V7" in net name "SUPPLY_4V7"`,
			warning:  `generic rail name "SUPPLY_4V7" reduces confidence`,
		},
		{
			name:    "multiple candidates",
			net:     "RAIL_3V3_5V",
			source:  SourceAmbiguousNumericToken,
			score:   0.00,
			level:   ConfidenceUnknown,
			warning: "multiple candidate voltages detected",
		},
		{
			name:    "unknown",
			net:     "SCL",
			source:  SourceUnknown,
			score:   0.00,
			level:   ConfidenceUnknown,
			warning: `no voltage evidence found in net name "SCL"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferVoltageFromNetName(tt.net)
			assertVoltageInference(t, got, tt.net, tt.voltage, tt.source, tt.score, tt.level)
			if tt.evidence != "" && !containsString(got.Evidence, tt.evidence) {
				t.Fatalf("expected evidence %q, got %+v", tt.evidence, got.Evidence)
			}
			if tt.warning != "" && !containsString(got.Warnings, tt.warning) {
				t.Fatalf("expected warning %q, got %+v", tt.warning, got.Warnings)
			}
		})
	}
}

func TestInferVoltageFromNetName_ExactNumericExamples(t *testing.T) {
	tests := map[string]float64{
		"12V": 12.0,
		"1V8": 1.8,
	}
	for net, voltage := range tests {
		got := InferVoltageFromNetName(net)
		assertVoltageInference(t, got, net, ptr(voltage), SourceNetNameExact, 0.95, ConfidenceHigh)
	}
}

func TestInferVoltages_ExternalEvidenceScores(t *testing.T) {
	result := InferVoltages(&ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/+5V"},
			{Name: "VBAT"},
			{Name: "REG_OUT"},
		},
	}, VoltageInferenceOptions{
		Evidence: []VoltageEvidence{
			{
				NetName:  "VBAT",
				Voltage:  24.0,
				Source:   SourceUserOverride,
				Evidence: `user override set rail "VBAT" to 24.00V`,
			},
			{
				NetName:  "REG_OUT",
				Voltage:  3.3,
				Source:   SourceRegulatorOutput,
				Evidence: `regulator U1 output mapped to rail "REG_OUT" at 3.30V`,
			},
		},
	})

	assertVoltageInference(t, findInference(t, result, "VBAT"), "VBAT", ptr(24.0), SourceUserOverride, 1.00, ConfidenceHigh)
	assertVoltageInference(t, findInference(t, result, "REG_OUT"), "REG_OUT", ptr(3.3), SourceRegulatorOutput, 0.90, ConfidenceHigh)
}

func TestInferVoltages_ConflictingEvidenceDowngrades(t *testing.T) {
	result := InferVoltages(&ir.DesignIR{
		Nets: []ir.Net{{Name: "/+5V"}},
	}, VoltageInferenceOptions{
		Evidence: []VoltageEvidence{
			{
				NetName:  "/+5V",
				Voltage:  4.7,
				Source:   SourceUserOverride,
				Evidence: `user override set rail "/+5V" to 4.70V`,
			},
		},
	})

	got := findInference(t, result, "/+5V")
	assertVoltageInference(t, got, "/+5V", ptr(4.7), SourceUserOverride, 0.30, ConfidenceUnknown)
	if !containsString(got.Warnings, "conflicting voltage evidence detected") {
		t.Fatalf("expected conflicting evidence warning, got %+v", got.Warnings)
	}
}

func ptr(v float64) *float64 {
	return &v
}

func assertVoltageInference(t *testing.T, got VoltageInference, net string, voltage *float64, source string, score float64, level string) {
	t.Helper()
	if got.NetName != net {
		t.Fatalf("expected net %q, got %q", net, got.NetName)
	}
	if voltage == nil {
		if got.Voltage != nil {
			t.Fatalf("expected nil voltage, got %v", *got.Voltage)
		}
	} else {
		if got.Voltage == nil {
			t.Fatalf("expected voltage %v, got nil", *voltage)
		}
		if math.Abs(*got.Voltage-*voltage) > 1e-9 {
			t.Fatalf("expected voltage %v, got %v", *voltage, *got.Voltage)
		}
	}
	if got.Source != source {
		t.Fatalf("expected source %s, got %s", source, got.Source)
	}
	if math.Abs(got.ConfidenceScore-score) > 1e-9 {
		t.Fatalf("expected score %.2f, got %.2f", score, got.ConfidenceScore)
	}
	if got.ConfidenceLevel != level {
		t.Fatalf("expected level %s, got %s", level, got.ConfidenceLevel)
	}
}

func findInference(t *testing.T, result Result, net string) VoltageInference {
	t.Helper()
	for _, inference := range result.Inferences {
		if inference.NetName == net {
			return inference
		}
	}
	t.Fatalf("expected inference for %q, got %+v", net, result.Inferences)
	return VoltageInference{}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
