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

type dummyImporter struct {
	design *ir.DesignIR
}

func (dummyImporter) Name() string { return "dummy" }

func (dummyImporter) Detect(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".dummy")
}

func (i dummyImporter) Import(path string) (*ir.DesignIR, error) {
	design := i.design
	if design == nil {
		design = &ir.DesignIR{Version: ir.SchemaVersion}
	}
	design.SourceInfo = ir.SourceInfo{
		Importer: "dummy",
		Format:   "test",
		Input:    path,
	}
	return design, nil
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

func TestDummyImporterReturningSameDesignIRNeedsNoRuleChanges(t *testing.T) {
	sharedDesign := &ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "dummy_project",
		Nets: []ir.Net{
			{Name: "DUMMY_5V", Pins: []ir.PinRef{{Ref: "SRC", Pin: "OUT"}, {Ref: "LOAD", Pin: "VIN"}}},
		},
	}

	design, importer, err := importers.Import("board.dummy", []importers.Importer{
		dummyImporter{design: sharedDesign},
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if importer.Name() != "dummy" {
		t.Fatalf("expected dummy importer, got %s", importer.Name())
	}
	if design != sharedDesign {
		t.Fatalf("expected dummy importer to return the same DesignIR pointer")
	}

	contractIR := contracts.NewContractIR()
	contractIR.PutPin("SRC", "OUT", contracts.PinContract{
		Role:           contracts.RolePowerOut,
		VoltageNominal: contracts.Float64(5.0),
		Direction:      contracts.DirectionOutput,
	})
	contractIR.PutPin("LOAD", "VIN", contracts.PinContract{
		Role:       contracts.RolePowerIn,
		VoltageMax: contracts.Float64(3.3),
		Direction:  contracts.DirectionInput,
	})

	got := rules.CheckAll(design, contractIR, rules.DefaultRules())
	if len(got) != 1 || got[0].RuleID != rules.RuleSupplyContract {
		t.Fatalf("expected dummy-imported DesignIR to produce supply finding, got %+v", got)
	}
}
