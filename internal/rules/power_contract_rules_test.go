package rules

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestSupplyAbsMaxRule_AboveAbsMaxErrors(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("5V", contracts.NetContract{VoltageNominal: contracts.Float64(5.0), Source: "inferred"})
	contractIR.PutPin("U1", "VDD", contracts.PinContract{
		Role:          contracts.RolePowerIn,
		AbsVoltageMax: contracts.Float64(3.6),
		Direction:     contracts.DirectionInput,
		Source:        "parts-library",
	})

	got := SupplyAbsMaxRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != "error" || got[0].RuleID != RuleSupplyAbsMax {
		t.Fatalf("expected supply abs max error, got %+v", got[0])
	}
	if got[0].Source != "parts-library" {
		t.Fatalf("expected provenance, got %+v", got[0])
	}
}

func TestSupplyRecommendedRangeRule_OutsideRecommendedWarns(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "3V8", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("3V8", contracts.NetContract{VoltageNominal: contracts.Float64(3.8), Source: "inferred"})
	contractIR.PutPin("U1", "VDD", contracts.PinContract{
		Role:                  contracts.RolePowerIn,
		RecommendedVoltageMin: contracts.Float64(3.0),
		RecommendedVoltageMax: contracts.Float64(3.6),
		AbsVoltageMax:         contracts.Float64(4.0),
		Direction:             contracts.DirectionInput,
		Source:                "parts-library",
	})

	got := SupplyRecommendedRangeRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != "warning" || got[0].RuleID != RuleSupplyRange {
		t.Fatalf("expected recommended range warning, got %+v", got[0])
	}
}

func TestGPIOAbsMaxRule_OvervoltageErrors(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "SIG_5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "GPIO0"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("SIG_5V", contracts.NetContract{VoltageNominal: contracts.Float64(5.0), Source: "inferred"})
	contractIR.PutPin("U1", "GPIO0", contracts.PinContract{
		Role:          contracts.RoleGPIO,
		AbsVoltageMax: contracts.Float64(3.6),
		Direction:     contracts.DirectionInput,
		Source:        "parts-library",
	})

	got := GPIOAbsMaxRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != "error" || got[0].RuleID != RuleGPIOAbsMax {
		t.Fatalf("expected GPIO abs max error, got %+v", got[0])
	}
}

func TestLogicLevelMarginRule_HighMarginWarnsOrErrors(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "GPIO", Pins: []ir.PinRef{{Ref: "U1", Pin: "TX"}, {Ref: "U2", Pin: "RX"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("U1", "TX", contracts.PinContract{
		Role:      contracts.RoleGPIO,
		VOHMin:    contracts.Float64(2.0),
		Direction: contracts.DirectionOutput,
		Source:    "parts-library",
	})
	contractIR.PutPin("U2", "RX", contracts.PinContract{
		Role:      contracts.RoleGPIO,
		VIHMin:    contracts.Float64(2.1),
		Direction: contracts.DirectionInput,
		Source:    "parts-library",
	})

	got := LogicLevelMarginRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != "warning" || got[0].RuleID != RuleLogicLevelMargin {
		t.Fatalf("expected logic margin warning, got %+v", got[0])
	}
}

func TestRegulatorOutputCurrentRule_OverloadErrors(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "OUT"}, {Ref: "U2", Pin: "VDD"}, {Ref: "U3", Pin: "VDD"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("U1", "OUT", contracts.PinContract{
		Role:             contracts.RoleRegulatorOut,
		OutputCurrentMax: contracts.Float64(0.5),
		Direction:        contracts.DirectionOutput,
		Source:           "parts-library",
	})
	contractIR.PutPin("U2", "VDD", contracts.PinContract{Role: contracts.RolePowerIn, TypicalCurrent: contracts.Float64(0.3), Direction: contracts.DirectionInput})
	contractIR.PutPin("U3", "VDD", contracts.PinContract{Role: contracts.RolePowerIn, TypicalCurrent: contracts.Float64(0.25), Direction: contracts.DirectionInput})

	got := RegulatorOutputCurrentRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != "error" || got[0].RuleID != RuleRegulatorCurrent {
		t.Fatalf("expected regulator current error, got %+v", got[0])
	}
}

func TestMotorDriverVMRangeRule_AboveAbsMaxErrors(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "24V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VM"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("24V", contracts.NetContract{VoltageNominal: contracts.Float64(24.0), Source: "inferred"})
	contractIR.PutPin("U1", "VM", contracts.PinContract{
		Role:          contracts.RolePowerIn,
		MotorSupply:   true,
		AbsVoltageMax: contracts.Float64(15.0),
		Direction:     contracts.DirectionInput,
		Source:        "parts-library",
	})

	got := MotorDriverVMRangeRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != "error" || got[0].RuleID != RuleMotorDriverVMRange {
		t.Fatalf("expected motor VM error, got %+v", got[0])
	}
}

func TestRegulatorOutputCurrentRule_MissingCurrentDataDoesNotCrash(t *testing.T) {
	design := &ir.DesignIR{Nets: []ir.Net{{Name: "3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "OUT"}, {Ref: "U2", Pin: "VDD"}}}}}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("U1", "OUT", contracts.PinContract{
		Role:             contracts.RoleRegulatorOut,
		OutputCurrentMax: contracts.Float64(0.5),
		Direction:        contracts.DirectionOutput,
		Source:           "parts-library",
	})
	contractIR.PutPin("U2", "VDD", contracts.PinContract{Role: contracts.RolePowerIn, Direction: contracts.DirectionInput})

	got := RegulatorOutputCurrentRule{}.Check(design, contractIR)
	if len(got) != 0 {
		t.Fatalf("expected no finding without current data, got %+v", got)
	}
}
