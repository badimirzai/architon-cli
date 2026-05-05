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

func TestFieldOverridesBuiltInContract(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{
			Ref: "U1",
			MPN: "ESP32-WROOM-32",
			Fields: map[string]string{
				"architon_supply_pins":      "VDD",
				"architon_supply_abs_max_v": "5.5",
			},
		}},
		Nets: []ir.Net{
			{Name: "5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}}},
		},
	}

	contractIR, err := (enrichment.ContractEnricher{Sources: []contracts.ContractSource{
		contracts.FieldContractSource{},
		contracts.NewBuiltinPartsSource(),
		enrichment.NewNetVoltageSource("inferred", []enrichment.NetVoltage{{Net: "5V", Voltage: 5.0, Source: "inferred"}}),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}

	req := requireAppliedRequirement(t, contractIR, "U1", contracts.ContractSupplyAbsMax)
	if req.Source != "schematic-bom-fields" {
		t.Fatalf("expected field source to win, got %+v", req)
	}
	if req.MaxVoltage == nil || *req.MaxVoltage != 5.5 {
		t.Fatalf("expected field max voltage 5.5, got %+v", req)
	}
	if findings := contracts.Evaluate(design, contractIR); len(findings) != 0 {
		t.Fatalf("expected field override to avoid built-in finding, got %+v", findings)
	}
}

func TestBuiltInOverridesInferredVoltageForEvaluation(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "ESP32-WROOM-32"}},
		Nets: []ir.Net{
			{Name: "5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}}},
		},
	}

	contractIR, err := (enrichment.ContractEnricher{Sources: []contracts.ContractSource{
		contracts.NewBuiltinPartsSource(),
		enrichment.NewNetVoltageSource("inferred", []enrichment.NetVoltage{{Net: "5V", Voltage: 5.0, Source: "inferred"}}),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}

	req := requireAppliedRequirement(t, contractIR, "U1", contracts.ContractSupplyAbsMax)
	if req.Source != "built-in" {
		t.Fatalf("expected built-in source to win over inferred voltage data, got %+v", req)
	}
	requireFinding(t, contracts.Evaluate(design, contractIR), string(contracts.ContractSupplyAbsMax), "ERROR")
}

func TestMultiSourceMergeRespectsPrecedenceAndEvaluates(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U1", MPN: "ESP32-WROOM-32"},
			{Ref: "U2", MPN: "ESP32-WROOM-32"},
		},
		Nets: []ir.Net{
			{Name: "5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}, {Ref: "U2", Pin: "VDD"}}},
		},
	}

	contractIR, err := (enrichment.ContractEnricher{Sources: []contracts.ContractSource{
		enrichment.NewMetaYAMLSource(&meta.Meta{
			Sources:    []meta.Source{{Net: "5V", Voltage: 5.0}},
			Components: []meta.Component{{Ref: "U1", MaxVoltage: 5.5}},
		}),
		contracts.NewBuiltinPartsSource(),
		enrichment.NewNetVoltageSource("inferred", []enrichment.NetVoltage{{Net: "5V", Voltage: 5.0, Source: "inferred"}}),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}

	if _, ok := contractIR.Components["U1"]; !ok {
		t.Fatal("expected meta component contract for U1")
	}
	req := requireAppliedRequirement(t, contractIR, "U2", contracts.ContractSupplyAbsMax)
	if req.Source != "built-in" {
		t.Fatalf("expected built-in contract for U2, got %+v", req)
	}
	for _, req := range contractIR.AppliedRequirements {
		if req.ComponentRef == "U1" && req.Type == contracts.ContractSupplyAbsMax {
			t.Fatalf("expected meta to suppress U1 built-in requirement, got %+v", req)
		}
	}

	findings := contracts.Evaluate(design, contractIR)
	requireFinding(t, findings, string(contracts.ContractSupplyAbsMax), "ERROR")
	if len(findings) != 1 || findings[0].ComponentRef != "U2" {
		t.Fatalf("expected only U2 built-in finding, got %+v", findings)
	}
}

func TestEvaluateSupplyAboveAbsMaxErrors(t *testing.T) {
	design := onePinDesign("U1", "VDD", "5V")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("5V", contracts.NetContract{VoltageNominal: contracts.Float64(5.0)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractSupplyAbsMax, "VDD", nil, contracts.Float64(3.6)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractSupplyAbsMax), "ERROR")
}

func TestEvaluateRecommendedRangeWarns(t *testing.T) {
	design := onePinDesign("U1", "VDD", "3V7")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("3V7", contracts.NetContract{VoltageNominal: contracts.Float64(3.7)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractSupplyAbsMax, "VDD", nil, contracts.Float64(4.0)))
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractSupplyRecommendedRange, "VDD", contracts.Float64(3.0), contracts.Float64(3.6)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractSupplyRecommendedRange), "WARN")
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
	requireFinding(t, got, string(contracts.ContractGPIOAbsMax), "ERROR")
}

func TestEvaluateMotorVMRangeErrors(t *testing.T) {
	design := onePinDesign("U1", "VM", "VBAT")
	contractIR := contracts.NewContractIR()
	contractIR.PutNet("VBAT", contracts.NetContract{VoltageNominal: contracts.Float64(24.0)})
	contractIR.PutAppliedRequirement(appliedVoltageRequirement("U1", contracts.ContractMotorDriverVMRange, "VM", contracts.Float64(2.5), contracts.Float64(13.5)))

	got := contracts.Evaluate(design, contractIR)
	requireFinding(t, got, string(contracts.ContractMotorDriverVMRange), "ERROR")
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
	requireFinding(t, got, string(contracts.ContractRegulatorOutputCurrent), "ERROR")
}

func TestBuiltInTB6612VMNumberedPinsTriggerRange(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "TB6612FNG"}},
		Nets: []ir.Net{
			{Name: "+3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "VM1"}, {Ref: "U1", Pin: "VM2"}, {Ref: "U1", Pin: "VM3"}}},
		},
	}
	contractIR := enrichBuiltInForTest(t, design, enrichment.NetVoltage{Net: "+3V3", Voltage: 3.3, Source: "test"})

	findings := contracts.Evaluate(design, contractIR)
	for _, pin := range []string{"VM1", "VM2", "VM3"} {
		requireFindingOnPin(t, findings, string(contracts.ContractMotorDriverVMRange), "U1", pin, "+3V3")
	}
	if countFindings(findings, string(contracts.ContractMotorDriverVMRange)) != 3 {
		t.Fatalf("expected one motor VM finding per VM pin, got %+v", findings)
	}
}

func TestBuiltInTB6612SingleNumberedVMEvaluates(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "TB6612FNG"}},
		Nets: []ir.Net{
			{Name: "+3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "VM1"}}},
		},
	}
	contractIR := enrichBuiltInForTest(t, design, enrichment.NetVoltage{Net: "+3V3", Voltage: 3.3, Source: "test"})

	findings := contracts.Evaluate(design, contractIR)
	requireFindingOnPin(t, findings, string(contracts.ContractMotorDriverVMRange), "U1", "VM1", "+3V3")
	if countFindings(findings, string(contracts.ContractMotorDriverVMRange)) != 1 {
		t.Fatalf("expected only connected VM1 to be evaluated, got %+v", findings)
	}
}

func TestBuiltInDRV8833VMAndLogicSupplyAreSeparate(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "DRV8833"}},
		Nets: []ir.Net{
			{Name: "+2V0", Pins: []ir.PinRef{{Ref: "U1", Pin: "VM"}}},
			{Name: "+3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "VCC"}}},
		},
	}
	contractIR := enrichBuiltInForTest(t, design,
		enrichment.NetVoltage{Net: "+2V0", Voltage: 2.0, Source: "test"},
		enrichment.NetVoltage{Net: "+3V3", Voltage: 3.3, Source: "test"},
	)

	if pin, ok := contractIR.Pin("U1", "VCC"); !ok || pin.Role != contracts.RolePowerIn {
		t.Fatalf("expected VCC logic supply pin contract, got %+v ok=%v", pin, ok)
	}
	findings := contracts.Evaluate(design, contractIR)
	requireFindingOnPin(t, findings, string(contracts.ContractMotorDriverVMRange), "U1", "VM", "+2V0")
	if hasFindingOnPin(findings, string(contracts.ContractMotorDriverVMRange), "VCC") {
		t.Fatalf("did not expect VCC to be evaluated as motor VM, got %+v", findings)
	}
}

func TestBuiltInAMS1117AliasesRecognized(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "AMS1117-3.3"}},
		Nets: []ir.Net{
			{Name: "VIN", Pins: []ir.PinRef{{Ref: "U1", Pin: "VI"}, {Ref: "U1", Pin: "IN"}}},
			{Name: "3V3", Pins: []ir.PinRef{{Ref: "U1", Pin: "VO"}, {Ref: "U1", Pin: "OUT"}}},
		},
	}
	contractIR := enrichBuiltInForTest(t, design)

	for _, pinName := range []string{"VI", "IN"} {
		if pin, ok := contractIR.Pin("U1", pinName); !ok || pin.Role != contracts.RolePowerIn {
			t.Fatalf("expected %s input pin contract, got %+v ok=%v", pinName, pin, ok)
		}
	}
	for _, pinName := range []string{"VO", "OUT"} {
		if pin, ok := contractIR.Pin("U1", pinName); !ok || pin.Role != contracts.RoleRegulatorOut {
			t.Fatalf("expected %s regulator output pin contract, got %+v ok=%v", pinName, pin, ok)
		}
	}
}

func TestBuiltInESP32SupplyGroundAndGPIOAliases(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "ESP32-WROOM-32"}},
		Nets: []ir.Net{
			{Name: "+5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}, {Ref: "U1", Pin: "GPIO0"}}},
			{Name: "GND", Pins: []ir.PinRef{{Ref: "U1", Pin: "GND"}}},
		},
	}
	contractIR := enrichBuiltInForTest(t, design, enrichment.NetVoltage{Net: "+5V", Voltage: 5.0, Source: "test"})

	if pin, ok := contractIR.Pin("U1", "VDD"); !ok || pin.Role != contracts.RolePowerIn {
		t.Fatalf("expected VDD supply pin contract, got %+v ok=%v", pin, ok)
	}
	if pin, ok := contractIR.Pin("U1", "GND"); !ok || pin.Role != contracts.RoleGround {
		t.Fatalf("expected GND ground pin contract, got %+v ok=%v", pin, ok)
	}
	findings := contracts.Evaluate(design, contractIR)
	requireFindingOnPin(t, findings, string(contracts.ContractSupplyAbsMax), "U1", "VDD", "+5V")
	requireFindingOnPin(t, findings, string(contracts.ContractGPIOAbsMax), "U1", "GPIO0", "+5V")
}

func TestBuiltInMPU6050SupplyAndI2CAliases(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "MPU-6050"}},
		Nets: []ir.Net{
			{Name: "+5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}, {Ref: "U1", Pin: "VLOGIC"}, {Ref: "U1", Pin: "SDA/SDI"}, {Ref: "U1", Pin: "SCL/SPC"}}},
		},
	}
	contractIR := enrichBuiltInForTest(t, design, enrichment.NetVoltage{Net: "+5V", Voltage: 5.0, Source: "test"})

	for _, pinName := range []string{"VDD", "VLOGIC"} {
		if pin, ok := contractIR.Pin("U1", pinName); !ok || pin.Role != contracts.RolePowerIn {
			t.Fatalf("expected %s supply pin contract, got %+v ok=%v", pinName, pin, ok)
		}
	}
	findings := contracts.Evaluate(design, contractIR)
	requireFindingOnPin(t, findings, string(contracts.ContractSupplyAbsMax), "U1", "VDD", "+5V")
	requireFindingOnPin(t, findings, string(contracts.ContractSupplyAbsMax), "U1", "VLOGIC", "+5V")
	if hasFindingOnPin(findings, string(contracts.ContractSupplyAbsMax), "SDA/SDI") ||
		hasFindingOnPin(findings, string(contracts.ContractSupplyAbsMax), "SCL/SPC") {
		t.Fatalf("did not expect I2C aliases to be evaluated as supplies, got %+v", findings)
	}
	requireFindingOnPin(t, findings, string(contracts.ContractGPIOAbsMax), "U1", "SDA/SDI", "+5V")
	requireFindingOnPin(t, findings, string(contracts.ContractGPIOAbsMax), "U1", "SCL/SPC", "+5V")
}

func TestBuiltInLSM9DS1RemainsUnknown(t *testing.T) {
	got := contracts.MatchPart(ir.Part{Ref: "U1", MPN: "LSM9DS1"}, contracts.BuiltinContracts())
	if got.Matched || got.Ambiguous {
		t.Fatalf("expected LSM9DS1 to remain unknown, got %+v", got)
	}
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

func enrichBuiltInForTest(t *testing.T, design *ir.DesignIR, voltages ...enrichment.NetVoltage) *contracts.ContractIR {
	t.Helper()
	contractIR, err := (enrichment.ContractEnricher{Sources: []contracts.ContractSource{
		contracts.NewBuiltinPartsSource(),
		enrichment.NewNetVoltageSource("test", voltages),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	return contractIR
}

func onePinDesign(ref string, pin string, net string) *ir.DesignIR {
	return &ir.DesignIR{
		Parts: []ir.Part{{Ref: ref}},
		Nets:  []ir.Net{{Name: net, Pins: []ir.PinRef{{Ref: ref, Pin: pin}}}},
	}
}

func requireFindingOnPin(t *testing.T, findings []contracts.Finding, ruleID string, ref string, pin string, net string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.ComponentRef == ref && finding.Pin == pin && finding.Net == net {
			if finding.Source == "" || finding.Provenance.Source == "" || finding.Fix == "" {
				t.Fatalf("expected source/provenance/fix in finding, got %+v", finding)
			}
			return
		}
	}
	t.Fatalf("expected %s finding on %s/%s/%s, got %+v", ruleID, ref, pin, net, findings)
}

func hasFindingOnPin(findings []contracts.Finding, ruleID string, pin string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Pin == pin {
			return true
		}
	}
	return false
}

func countFindings(findings []contracts.Finding, ruleID string) int {
	count := 0
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			count++
		}
	}
	return count
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
		Provenance:   contracts.Provenance{Source: "test", SourceID: string(typ)},
	}
}

func requireAppliedRequirement(t *testing.T, contractIR *contracts.ContractIR, ref string, typ contracts.ContractType) contracts.AppliedRequirement {
	t.Helper()
	for _, req := range contractIR.AppliedRequirements {
		if req.ComponentRef == ref && req.Type == typ {
			return req
		}
	}
	t.Fatalf("expected applied requirement %s/%s, got %+v", ref, typ, contractIR.AppliedRequirements)
	return contracts.AppliedRequirement{}
}

func requireFinding(t *testing.T, findings []contracts.Finding, ruleID string, severity string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity {
			if finding.ComponentRef == "" || finding.Net == "" || finding.Pin == "" {
				t.Fatalf("expected component/net/pin in finding, got %+v", finding)
			}
			if finding.Source == "" || finding.Provenance.Source == "" || finding.Fix == "" {
				t.Fatalf("expected source/provenance/fix in finding, got %+v", finding)
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
