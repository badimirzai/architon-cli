package contracts_test

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/enrichment"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
)

func TestMatchPartExactMPN(t *testing.T) {
	got := contracts.MatchPart(ir.Part{Ref: "U1", MPN: "ESP32-WROOM-32"}, contracts.BuiltinContracts())
	if !got.Matched || got.Ambiguous {
		t.Fatalf("expected exact match, got %+v", got)
	}
	if got.Contract.MPN != "ESP32-WROOM-32" {
		t.Fatalf("expected ESP32-WROOM-32, got %q", got.Contract.MPN)
	}
	if got.Kind != "exact_mpn" {
		t.Fatalf("expected exact_mpn kind, got %q", got.Kind)
	}
}

func TestMatchPartAlias(t *testing.T) {
	got := contracts.MatchPart(ir.Part{Ref: "U1", MPN: "GY-521"}, contracts.BuiltinContracts())
	if !got.Matched || got.Ambiguous {
		t.Fatalf("expected alias match, got %+v", got)
	}
	if got.Contract.MPN != "MPU-6050" {
		t.Fatalf("expected MPU-6050, got %q", got.Contract.MPN)
	}
	if got.Kind != "alias" {
		t.Fatalf("expected alias kind, got %q", got.Kind)
	}
}

func TestMatchPartAmbiguousReturnsNoMatch(t *testing.T) {
	catalog := []contracts.SystemContract{
		{MPN: "PART-A", Aliases: []string{"DUPLICATE"}},
		{MPN: "PART-B", Aliases: []string{"DUPLICATE"}},
	}
	got := contracts.MatchPart(ir.Part{Ref: "U1", MPN: "DUPLICATE"}, catalog)
	if got.Matched {
		t.Fatalf("expected ambiguous result to have no match, got %+v", got)
	}
	if !got.Ambiguous {
		t.Fatalf("expected ambiguous result, got %+v", got)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("expected two candidates, got %+v", got.Candidates)
	}
}

func TestMetaYAMLOverridesBuiltInContract(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "ESP32-WROOM-32"}},
		Nets: []ir.Net{
			{Name: "5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}}},
		},
	}

	contractIR, err := (enrichment.ContractEnricher{Sources: []contracts.ContractSource{
		enrichment.NewMetaYAMLSource(&meta.Meta{
			Components: []meta.Component{{Ref: "U1", MaxVoltage: 5.5}},
		}),
		contracts.NewBuiltinPartsSource(),
		enrichment.NewNetVoltageSource("test", []enrichment.NetVoltage{{Net: "5V", Voltage: 5.0, Source: "test"}}),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}

	for _, req := range contractIR.AppliedRequirements {
		if req.ComponentRef == "U1" && req.Type == contracts.ContractSupplyAbsMax {
			t.Fatalf("expected meta.yaml to suppress built-in supply_abs_max, got %+v", req)
		}
	}
	if findings := contracts.Evaluate(design, contractIR); len(findings) != 0 {
		t.Fatalf("expected no built-in findings after meta override, got %+v", findings)
	}
}

func TestEvaluateSupplyAboveAbsMaxErrors(t *testing.T) {
	design := onePinDesign("U1", "VDD", "5V")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("5V", contracts.NetContract{VoltageNominal: contracts.Float64(5.0)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractSupplyAbsMax, "VDD", nil, contracts.Float64(3.6)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractSupplyAbsMax), "error")
}

func TestEvaluateRecommendedRangeWarns(t *testing.T) {
	design := onePinDesign("U1", "VDD", "3V7")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("3V7", contracts.NetContract{VoltageNominal: contracts.Float64(3.7)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractSupplyAbsMax, "VDD", nil, contracts.Float64(4.0)))
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractSupplyRecommendedRange, "VDD", contracts.Float64(3.0), contracts.Float64(3.6)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractSupplyRecommendedRange), "warning")
	if hasFinding(got, string(contracts.ContractSupplyAbsMax)) {
		t.Fatalf("did not expect abs max error, got %+v", got)
	}
}

func TestEvaluateGPIOOvervoltageErrors(t *testing.T) {
	design := onePinDesign("U1", "GPIO0", "GPIO_5V")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("GPIO_5V", contracts.NetContract{VoltageNominal: contracts.Float64(5.0)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractGPIOAbsMax, "GPIO*", nil, contracts.Float64(3.6)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractGPIOAbsMax), "error")
}

func TestEvaluateMotorVMRangeErrors(t *testing.T) {
	design := onePinDesign("U1", "VM", "VBAT")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("VBAT", contracts.NetContract{VoltageNominal: contracts.Float64(24.0)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractMotorDriverVMRange, "VM", contracts.Float64(2.5), contracts.Float64(13.5)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractMotorDriverVMRange), "error")
}

func TestEvaluateRegulatorOverloadErrors(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1"}, {Ref: "U2"}},
		Nets: []ir.Net{
			{Name: "3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "OUT"}, {Ref: "U2", Pin: "VCC"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutAppliedRequirement(contracts.AppliedRequirement{
		Requirement: contracts.Requirement{
			Type:       contracts.ContractRegulatorOutputCurrent,
			Scope:      contracts.ContractScope{Pins: []string{"OUT"}, Role: contracts.RoleRegulatorOut},
			MaxCurrent: contracts.Float64(0.5),
		},
		ComponentRef: "U1",
		Source:       "test",
	})
	contractIR.PutPin("U2", "VCC", contracts.PinContract{CurrentMax: contracts.Float64(0.8)})

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractRegulatorOutputCurrent), "error")
}

func TestEvaluateMissingCurrentDataDoesNotCrash(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1"}, {Ref: "U2"}},
		Nets: []ir.Net{
			{Name: "3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "OUT"}, {Ref: "U2", Pin: "VCC"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutAppliedRequirement(contracts.AppliedRequirement{
		Requirement: contracts.Requirement{
			Type:       contracts.ContractRegulatorOutputCurrent,
			Scope:      contracts.ContractScope{Pins: []string{"OUT"}, Role: contracts.RoleRegulatorOut},
			MaxCurrent: contracts.Float64(0.5),
		},
		ComponentRef: "U1",
		Source:       "test",
	})

	if got := contracts.Evaluate(design, contractIR); len(got) != 0 {
		t.Fatalf("expected missing current data to produce no finding, got %+v", got)
	}
}

func onePinDesign(ref string, pin string, net string) *ir.DesignIR {
	return &ir.DesignIR{
		Parts: []ir.Part{{Ref: ref}},
		Nets:  []ir.Net{{Name: net, Pins: []ir.PinRef{{Ref: ref, Pin: pin}}}},
	}
}

func appliedVoltageRequirement(ref string, typ contracts.ContractType, pin string, minV *float64, maxV *float64) contracts.AppliedRequirement {
	return contracts.AppliedRequirement{
		Requirement: contracts.Requirement{
			Type:       typ,
			Scope:      contracts.ContractScope{Pins: []string{pin}, Role: contracts.RolePowerIn},
			MinVoltage: minV,
			MaxVoltage: maxV,
		},
		ComponentRef: ref,
		Source:       "test",
	}
}

func requireFinding(t *testing.T, findings []contracts.Finding, ruleID string, severity string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity {
			if finding.ComponentRef == "" || finding.Net == "" || finding.Pin == "" {
				t.Fatalf("expected component/net/pin in finding, got %+v", finding)
			}
			return
		}
	}
	t.Fatalf("expected %s/%s finding, got %+v", ruleID, severity, findings)
}

func hasFinding(findings []contracts.Finding, ruleID string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			return true
		}
	}
	return false
}
