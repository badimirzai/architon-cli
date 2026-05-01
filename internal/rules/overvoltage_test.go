package rules

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
	"github.com/badimirzai/architon-cli/internal/propagate"
)

func TestOvervoltage_Triggers(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "VBAT", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
		},
	}

	m := &meta.Meta{
		Components: []meta.Component{
			{Ref: "U1", MaxVoltage: 5.5},
		},
	}

	netV := map[string]propagate.NetVoltage{
		"VBAT": {Net: "VBAT", Voltage: 24.0, Source: "source"},
	}

	got := Overvoltage(design, m, netV)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].ID != "RULE_OVERVOLTAGE" {
		t.Fatalf("expected RULE_OVERVOLTAGE, got %s", got[0].ID)
	}
}

func TestOvervoltage_IncludesInferenceProvenance(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/+5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
		},
	}
	m := &meta.Meta{
		Components: []meta.Component{
			{Ref: "U1", MaxVoltage: 3.3},
		},
	}
	netV := map[string]propagate.NetVoltage{
		"/+5V": {Net: "/+5V", Voltage: 5.0, Source: "initial"},
	}
	inferences := map[string]infer.VoltageInference{
		"/+5V": {
			NetName:         "/+5V",
			Source:          infer.SourceNetNameExact,
			ConfidenceScore: 0.95,
			ConfidenceLevel: infer.ConfidenceHigh,
		},
	}

	got := OvervoltageWithInferences(design, m, netV, inferences)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Inference == nil {
		t.Fatalf("expected inference provenance, got %+v", got[0])
	}
	if got[0].Inference.NetName != "/+5V" {
		t.Fatalf("expected net /+5V, got %+v", got[0].Inference)
	}
	if got[0].Inference.Source != infer.SourceNetNameExact {
		t.Fatalf("expected source %s, got %+v", infer.SourceNetNameExact, got[0].Inference)
	}
	if got[0].Inference.ConfidenceLevel != infer.ConfidenceHigh || got[0].Inference.ConfidenceScore != 0.95 {
		t.Fatalf("expected high confidence provenance, got %+v", got[0].Inference)
	}
}

func TestOvervoltage_NoVoltage_NoResult(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "VBAT", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
		},
	}

	m := &meta.Meta{
		Components: []meta.Component{
			{Ref: "U1", MaxVoltage: 5.5},
		},
	}

	got := Overvoltage(design, m, map[string]propagate.NetVoltage{})
	if len(got) != 0 {
		t.Fatalf("expected 0 results, got %d", len(got))
	}
}
