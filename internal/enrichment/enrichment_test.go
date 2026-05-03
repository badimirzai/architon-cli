package enrichment

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
	"github.com/badimirzai/architon-cli/internal/rules"
)

func TestContractEnricher_MetaAndNetVoltagesCatchOvervoltageThroughContracts(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "J1"}, {Ref: "U1"}},
		Nets: []ir.Net{
			{Name: "/+5V", Pins: []ir.PinRef{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "VCC"}}},
		},
	}
	metaObj := &meta.Meta{
		Components: []meta.Component{{Ref: "U1", MaxVoltage: 3.3}},
	}

	contractIR, err := (ContractEnricher{Sources: []ContractSource{
		NewNetVoltageSource("test-voltage", []NetVoltage{{Net: "/+5V", Voltage: 5.0, Source: "inferred"}}),
		NewMetaYAMLSource(metaObj),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich failed: %v", err)
	}

	if pin, ok := contractIR.Pin("U1", "VCC"); !ok || pin.Role != contracts.RolePowerIn || pin.VoltageMax == nil {
		t.Fatalf("expected U1 VCC power input contract, got %+v ok=%v", pin, ok)
	}
	got := rules.SupplyContractRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected contract-level overvoltage, got %+v", got)
	}
	if got[0].RuleID != rules.RuleSupplyContract {
		t.Fatalf("expected supply contract rule, got %+v", got[0])
	}
}

func TestContractEnricher_RecordsMissingContractDataWarnings(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/+5V", Pins: []ir.PinRef{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "VCC"}}},
		},
	}

	contractIR, err := (ContractEnricher{Sources: []ContractSource{
		NewNetVoltageSource("test-voltage", []NetVoltage{{Net: "/+5V", Voltage: 5.0, Source: "inferred"}}),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich failed: %v", err)
	}

	if len(contractIR.MissingContractData) != 2 {
		t.Fatalf("expected missing contract warnings for both pins, got %+v", contractIR.MissingContractData)
	}
	coverage := contracts.SummarizeCoverage(design, contractIR)
	if len(coverage.MissingWarnings) != 2 {
		t.Fatalf("expected coverage warnings, got %+v", coverage)
	}
}

func TestContractEnricher_MetaOverridesBuiltInLibrary(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", Value: "STM32F103C8T6"}},
		Nets: []ir.Net{
			{Name: "5V", Pins: []ir.PinRef{{Ref: "U1", Pin: "VDD"}}},
		},
	}
	metaObj := &meta.Meta{
		Components: []meta.Component{{Ref: "U1", MaxVoltage: 3.3}},
	}

	contractIR, err := (ContractEnricher{Sources: []ContractSource{
		NewMetaYAMLSource(metaObj),
		NewPartsLibrarySource(nil),
		NewNetVoltageSource("test-voltage", []NetVoltage{{Net: "5V", Voltage: 5.0, Source: "inferred"}}),
	}}).Enrich(design)
	if err != nil {
		t.Fatalf("enrich failed: %v", err)
	}

	pin, ok := contractIR.Pin("U1", "VDD")
	if !ok {
		t.Fatalf("expected U1 VDD contract")
	}
	if pin.VoltageMax == nil || *pin.VoltageMax != 3.3 {
		t.Fatalf("expected meta voltage max to win, got %+v", pin)
	}
	if pin.Source != "meta.yaml" {
		t.Fatalf("expected meta source to win, got %+v", pin)
	}
	if pin.AbsVoltageMax == nil {
		t.Fatalf("expected built-in to fill non-conflicting richer fields, got %+v", pin)
	}
}
