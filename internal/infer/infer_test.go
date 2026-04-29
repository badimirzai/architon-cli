package infer

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestInferVoltagesFromNetNames(t *testing.T) {
	tests := []struct {
		net     string
		voltage float64
	}{
		{net: "/+5V", voltage: 5.0},
		{net: "3V3", voltage: 3.3},
		{net: "/+3.3V", voltage: 3.3},
		{net: "VBAT_24V", voltage: 24.0},
		{net: "/+4.24554V", voltage: 4.24554},
		{net: "GND", voltage: 0.0},
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
			if got.Source != "net_name" {
				t.Fatalf("expected source net_name, got %q", got.Source)
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
		if result.Unknowns[i].Reason != "ambiguous power net name" {
			t.Fatalf("expected ambiguous reason, got %q", result.Unknowns[i].Reason)
		}
	}
}

func TestInferVoltagesFromNetNames_DoesNotInferUnlistedNames(t *testing.T) {
	result := InferVoltagesFromNetNames(&ir.DesignIR{
		Nets: []ir.Net{
			{Name: "SCL"},
			{Name: "POWER_GOOD"},
			{Name: "+5V_USB"},
		},
	})

	if len(result.Voltages) != 0 {
		t.Fatalf("expected no inferred voltages, got %+v", result.Voltages)
	}
	if len(result.Unknowns) != 0 {
		t.Fatalf("expected no unknown voltage nets, got %+v", result.Unknowns)
	}
}
