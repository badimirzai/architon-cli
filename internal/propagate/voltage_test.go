package propagate

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
)

func designWithRegulator() *ir.DesignIR {
	return &ir.DesignIR{
		// Only fields used by your topology helpers are needed:
		Nets: []ir.Net{
			{
				Name: "VBAT",
				Pins: []ir.PinRef{
					{Ref: "U3", Pin: "VIN"},
				},
			},
			{
				Name: "+5V",
				Pins: []ir.PinRef{
					{Ref: "U3", Pin: "VOUT"},
				},
			},
		},
	}
}

func TestPropagate_SourceSetsVoltage(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "VBAT", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
		},
	}

	m := &meta.Meta{
		Sources: []meta.Source{
			{Net: "VBAT", Voltage: 12.0},
		},
	}

	res := Propagate(*design, *m, map[string]float64{"VBAT": 12.0})
	nv, ok := res.NetVoltages["VBAT"]
	if !ok {
		t.Fatalf("expected VBAT voltage to be set")
	}
	if nv.Voltage != 12.0 {
		t.Fatalf("expected 12.0, got %v", nv.Voltage)
	}
}

func TestPropagate_RegulatorPropagatesOutVoltage(t *testing.T) {
	design := designWithRegulator()

	m := &meta.Meta{
		Sources: []meta.Source{
			{Net: "VBAT", Voltage: 24.0},
		},
		Regulators: []meta.Regulator{
			{Ref: "U3", InPin: "VIN", OutPin: "VOUT", OutVoltage: 5.0},
		},
	}

	res := Propagate(*design, *m, map[string]float64{"VBAT": 24.0})

	out, ok := res.NetVoltages["+5V"]
	if !ok {
		t.Fatalf("expected +5V voltage to be set")
	}
	if out.Voltage != 5.0 {
		t.Fatalf("expected 5.0, got %v", out.Voltage)
	}
}

func TestPropagate_DetectsSourceConflict(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{{Name: "VBAT", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}}},
	}
	m := &meta.Meta{
		Sources: []meta.Source{
			{Net: "VBAT", Voltage: 12.0},
			{Net: "VBAT", Voltage: 24.0},
		},
	}

	res := Propagate(*design, *m, map[string]float64{"VBAT": 24.0})
	if len(res.Conflicts) == 0 {
		t.Fatalf("expected conflicts")
	}
}

func TestPropagate_MetaSourcesOverrideInitialVoltages(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/+5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
		},
	}
	m := &meta.Meta{
		Sources: []meta.Source{
			{Net: "/+5V", Voltage: 4.75},
		},
	}

	res := Propagate(*design, *m, map[string]float64{"/+5V": 5.0})
	got := res.NetVoltages["/+5V"]
	if got.Voltage != 4.75 {
		t.Fatalf("expected meta voltage to override inferred initial value, got %v", got.Voltage)
	}
	if got.Source != "source" {
		t.Fatalf("expected source to be metadata source, got %q", got.Source)
	}
}
