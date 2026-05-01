package importers_test

import (
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/importers"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/rules"
)

type fakeAltiumImporter struct{}

func (fakeAltiumImporter) Name() string { return "altium" }

func (fakeAltiumImporter) Detect(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".prjpcb")
}

func (fakeAltiumImporter) Import(path string) (*ir.DesignIR, error) {
	return &ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "altium_project",
		SourceInfo: ir.SourceInfo{
			Importer: "altium",
			Format:   "project",
			Input:    path,
		},
		Nets: []ir.Net{
			{Name: "VBUS", Pins: []ir.PinRef{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "VCC"}}},
		},
	}, nil
}

func TestFakeAltiumImporterFeedsRulesWithoutRuleChanges(t *testing.T) {
	design, importer, err := importers.Import("robot.PrjPcb", []importers.Importer{fakeAltiumImporter{}})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if importer.Name() != "altium" {
		t.Fatalf("expected fake altium importer, got %s", importer.Name())
	}

	contractIR := contracts.NewContractIR()
	contractIR.PutPin("J1", "1", contracts.PinContract{
		Role:           contracts.RolePowerOut,
		VoltageNominal: contracts.Float64(5.0),
		Direction:      contracts.DirectionOutput,
	})
	contractIR.PutPin("U1", "VCC", contracts.PinContract{
		Role:       contracts.RolePowerIn,
		VoltageMax: contracts.Float64(3.3),
		Direction:  contracts.DirectionInput,
	})

	got := rules.CheckAll(design, contractIR, rules.DefaultRules())
	if len(got) != 1 || got[0].RuleID != rules.RuleSupplyContract {
		t.Fatalf("expected rules to run on fake altium DesignIR, got %+v", got)
	}
}
