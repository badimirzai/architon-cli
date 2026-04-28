package rules

import (
	"testing"

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
