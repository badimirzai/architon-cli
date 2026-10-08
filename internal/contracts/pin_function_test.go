package contracts

import (
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestPinFunctionMismatchOnSwappedNet(t *testing.T) {
	design := pinFunctionDesign("U1", "MPU-6050", []ir.Net{
		{Name: "I2C_SCL", Pins: []ir.PinRef{{Ref: "U1", Pin: "SDA", Name: "SDA"}}},
	})
	findings := Evaluate(design, pinFunctionIR(design, busPair()))
	found := false
	for _, finding := range findings {
		if finding.RuleID != RulePinFunctionMismatch {
			continue
		}
		found = true
		if finding.Severity != "ERROR" || finding.Pin != "SDA" || finding.Net != "I2C_SCL" {
			t.Fatalf("unexpected mismatch: %+v", finding)
		}
		if finding.Expected == nil || finding.Expected.Text != "SDA" || finding.Observed == nil || finding.Observed.Text != "I2C_SCL" {
			t.Fatalf("expected signal and net evidence, got %+v %+v", finding.Expected, finding.Observed)
		}
		if len(finding.Citations) != 1 || finding.Citations[0].Datasheet == "" {
			t.Fatalf("expected a datasheet citation, got %+v", finding.Citations)
		}
	}
	if !found {
		t.Fatalf("expected pin_function_mismatch, got %+v", findings)
	}
}

func TestPinFunctionMatchingNetsDoNotMismatchOrShort(t *testing.T) {
	design := pinFunctionDesign("U1", "MPU-6050", []ir.Net{
		{Name: "/I2C_SDA", Pins: []ir.PinRef{{Ref: "U1", Pin: "24", Name: "SDA"}}},
		{Name: "/I2C_SCL", Pins: []ir.PinRef{{Ref: "U1", Pin: "23", Name: "SCL"}}},
	})
	findings := Evaluate(design, pinFunctionIR(design, busPair()))
	for _, finding := range findings {
		if finding.RuleID == RulePinFunctionMismatch || finding.RuleID == RulePinBusShort {
			t.Fatalf("expected no bus findings, got %+v", finding)
		}
	}
	analysis := AnalyzePinFunctions(design, pinFunctionIR(design, busPair()))
	if analysis.Proved.Count != 3 {
		t.Fatalf("expected two name checks and one short check, got %+v", analysis.Proved)
	}
}

func TestPinBusShortWhenDedicatedPinsShareANet(t *testing.T) {
	design := pinFunctionDesign("U1", "MPU-6050", []ir.Net{
		{Name: "I2C", Pins: []ir.PinRef{
			{Ref: "U1", Pin: "SDA", Name: "SDA"},
			{Ref: "U1", Pin: "SCL", Name: "SCL"},
		}},
	})
	findings := Evaluate(design, pinFunctionIR(design, busPair()))
	if !hasRuleFinding(findings, RulePinBusShort) {
		t.Fatalf("expected pin_bus_short, got %+v", findings)
	}
}

func TestGPIOCandidateOnI2CNetIsNotAMismatch(t *testing.T) {
	functions := []PinFunction{{
		Name:     "IO21",
		Kind:     PinFunctionGPIOCandidate,
		Citation: testCitation(),
	}}
	design := pinFunctionDesign("U1", "ESP32-WROOM-32", []ir.Net{
		{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "U1", Pin: "IO21", Name: "IO21"}}},
	})
	contractIR := pinFunctionIR(design, functions)
	findings := Evaluate(design, contractIR)
	for _, finding := range findings {
		if finding.RuleID == RulePinFunctionMismatch || finding.RuleID == RulePinBusShort {
			t.Fatalf("gpio_candidate produced a bus finding: %+v", finding)
		}
	}
	analysis := AnalyzePinFunctions(design, contractIR)
	if len(analysis.NotChecked) != 1 || analysis.NotChecked[0].Reason != uncheckedGPIOCandidate || analysis.NotChecked[0].Net != "I2C_SDA" {
		t.Fatalf("expected the I2C connection under not_checked, got %+v", analysis.NotChecked)
	}
	if analysis.Proved.Count != 0 {
		t.Fatalf("expected the GPIO connection not to count as proved, got %+v", analysis.Proved)
	}
}

func TestMissingCitationProducesNoFinding(t *testing.T) {
	functions := []PinFunction{{
		Name:   "SDA",
		Kind:   PinFunctionBus,
		Signal: "SDA",
	}}
	design := pinFunctionDesign("U1", "MPU-6050", []ir.Net{
		{Name: "I2C_SCL", Pins: []ir.PinRef{{Ref: "U1", Pin: "SDA", Name: "SDA"}}},
	})
	findings := Evaluate(design, pinFunctionIR(design, functions))
	for _, finding := range findings {
		if finding.RuleID == RulePinFunctionMismatch {
			t.Fatalf("uncited function produced a finding: %+v", finding)
		}
	}
	analysis := AnalyzePinFunctions(design, pinFunctionIR(design, functions))
	if len(analysis.NotChecked) != 1 || analysis.NotChecked[0].Reason != uncheckedNoPinFunction {
		t.Fatalf("expected no_pin_function, got %+v", analysis.NotChecked)
	}
}

func TestUnmatchedPartIsNotChecked(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "R1", Value: "10k"}},
		Nets:  []ir.Net{{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "R1", Pin: "1"}}}},
	}
	analysis := AnalyzePinFunctions(design, NewContractIR())
	if len(analysis.NotChecked) != 1 || analysis.NotChecked[0].Ref != "R1" || analysis.NotChecked[0].Reason != uncheckedUnmatchedPart {
		t.Fatalf("expected unmatched part, got %+v", analysis.NotChecked)
	}
	if len(analysis.Findings) != 0 || analysis.Proved.Count != 0 {
		t.Fatalf("unmatched part should not be a violation, got %+v", analysis)
	}
}

func TestPinNumberMatchWithoutInventingName(t *testing.T) {
	functions := []PinFunction{{
		Name:     "SDA",
		Number:   "24",
		Kind:     PinFunctionBus,
		Signal:   "SDA",
		Citation: testCitation(),
	}}
	design := pinFunctionDesign("U1", "MPU-6050", []ir.Net{
		{Name: "/SCL", Pins: []ir.PinRef{{Ref: "U1", Pin: "24"}}},
	})
	findings := Evaluate(design, pinFunctionIR(design, functions))
	if len(findings) != 1 || findings[0].RuleID != RulePinFunctionMismatch || findings[0].Pin != "24" {
		t.Fatalf("expected pin number 24 mismatch, got %+v", findings)
	}
	if findings[0].Observed == nil || findings[0].Observed.Text != "/SCL" {
		t.Fatalf("expected observed net, got %+v", findings[0].Observed)
	}
}

func TestBuiltinDriversDoNotGainBusPins(t *testing.T) {
	for _, mpn := range []string{"DRV8833", "TB6612FNG", "L298N", "PCA9306", "TXS0108E"} {
		match := MatchPart(ir.Part{MPN: mpn}, BuiltinContracts())
		if !match.Matched {
			t.Fatalf("expected %s to stay in the catalog", mpn)
		}
		if len(match.Contract.PinFunctions) != 0 {
			t.Fatalf("%s should keep voltage contracts only, got %+v", mpn, match.Contract.PinFunctions)
		}
		for _, req := range match.Contract.Requirements {
			if req.Type != ContractSupplyAbsMax && req.Type != ContractSupplyRecommendedRange && req.Type != ContractGPIOAbsMax && req.Type != ContractMotorDriverVMRange {
				t.Fatalf("%s requirement changed: %+v", mpn, req.Type)
			}
		}
	}
}

func TestBuiltinPinFunctionsStayWithinCitedKinds(t *testing.T) {
	for _, contract := range BuiltinContracts() {
		for _, fn := range contract.PinFunctions {
			if !fn.Citation.cited() {
				t.Fatalf("%s function %s has no citation", contract.MPN, fn.Name)
			}
			switch contract.MPN {
			case "ESP32-WROOM-32", "STM32F103C8T6", "RP2040":
				if fn.Kind == PinFunctionBus || strings.EqualFold(fn.Signal, "SDA") || strings.EqualFold(fn.Signal, "SCL") {
					t.Fatalf("%s must not mark a dedicated SDA or SCL pin: %+v", contract.MPN, fn)
				}
			case "AMS1117-3.3", "AP2114H-3.3":
				if strings.EqualFold(fn.Signal, "SDA") || strings.EqualFold(fn.Signal, "SCL") || fn.Kind == PinFunctionBus {
					t.Fatalf("%s must not gain an I2C function: %+v", contract.MPN, fn)
				}
			}
		}
	}
}

func testCitation() Citation {
	return Citation{Datasheet: "Test Datasheet", Revision: "1", Table: "Pin Functions"}
}

func busPair() []PinFunction {
	return []PinFunction{
		{Name: "SDA", Number: "24", Kind: PinFunctionBus, Signal: "SDA", Citation: testCitation()},
		{Name: "SCL", Number: "23", Kind: PinFunctionBus, Signal: "SCL", Citation: testCitation()},
		{Name: "VDD", Kind: PinFunctionPower, Signal: "VDD", Citation: testCitation()},
		{Name: "GND", Kind: PinFunctionGround, Citation: testCitation()},
	}
}

func pinFunctionDesign(ref string, mpn string, nets []ir.Net) *ir.DesignIR {
	return &ir.DesignIR{
		Parts: []ir.Part{{Ref: ref, MPN: mpn, Value: mpn}},
		Nets:  nets,
	}
}

func pinFunctionIR(design *ir.DesignIR, functions []PinFunction) *ContractIR {
	contractIR := NewContractIR()
	for _, part := range design.Parts {
		contractIR.PartMatches = append(contractIR.PartMatches, PartMatch{Ref: part.Ref, ContractMPN: part.MPN})
		component := contractIR.EnsureComponent(part.Ref)
		component.MPN = part.MPN
		component.PinFunctions = clonePinFunctions(functions)
		contractIR.PutComponent(component)
	}
	return contractIR
}

func hasRuleFinding(findings []Finding, ruleID string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			return true
		}
	}
	return false
}
