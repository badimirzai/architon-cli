package rules

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestRulesPackageDoesNotImportKiCad(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatalf("read rules dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(wd, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, importSpec := range file.Imports {
			path, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import %s: %v", importSpec.Path.Value, err)
			}
			if strings.Contains(path, "/internal/importers/") {
				t.Fatalf("rules package must not import EDA importers; %s imports %s", entry.Name(), path)
			}
		}
	}
}

func TestSupplyContract_OvervoltageThroughContracts(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/+5V", Pins: []ir.PinRef{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "VCC"}}},
		},
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

	got := SupplyContractRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].RuleID != RuleSupplyContract {
		t.Fatalf("expected %s, got %s", RuleSupplyContract, got[0].RuleID)
	}
	if got[0].Severity != "error" {
		t.Fatalf("expected error, got %+v", got[0])
	}
}

func TestSupplyContract_SyntheticDesignWithoutImporter(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "VBUS", Pins: []ir.PinRef{{Ref: "SRC", Pin: "OUT"}, {Ref: "U1", Pin: "VIN"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("SRC", "OUT", contracts.PinContract{
		Role:           contracts.RoleSource,
		VoltageNominal: contracts.Float64(5.0),
		Direction:      contracts.DirectionOutput,
	})
	contractIR.PutPin("U1", "VIN", contracts.PinContract{
		Role:       contracts.RolePowerIn,
		VoltageMin: contracts.Float64(4.5),
		VoltageMax: contracts.Float64(5.5),
		Direction:  contracts.DirectionInput,
	})

	got := CheckAll(design, contractIR, DefaultRules())
	if len(got) != 0 {
		t.Fatalf("expected clean synthetic design, got %+v", got)
	}
}

func TestRulesRunOnSyntheticDesignWithoutImporterProducesFinding(t *testing.T) {
	design := &ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "synthetic_test_design",
		Nets: []ir.Net{
			{Name: "SYNTH_5V", Pins: []ir.PinRef{{Ref: "PWR", Pin: "OUT"}, {Ref: "MCU", Pin: "VDD"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("PWR", "OUT", contracts.PinContract{
		Role:           contracts.RolePowerOut,
		VoltageNominal: contracts.Float64(5.0),
		Direction:      contracts.DirectionOutput,
	})
	contractIR.PutPin("MCU", "VDD", contracts.PinContract{
		Role:       contracts.RolePowerIn,
		VoltageMax: contracts.Float64(3.3),
		Direction:  contracts.DirectionInput,
	})

	got := CheckAll(design, contractIR, DefaultRules())
	if len(got) != 1 {
		t.Fatalf("expected synthetic DesignIR to produce one finding, got %+v", got)
	}
	if got[0].RuleID != RuleSupplyContract || got[0].Severity != "error" {
		t.Fatalf("expected supply contract error from synthetic DesignIR, got %+v", got[0])
	}
}

func TestSupplyContractMutationChangesOutcome(t *testing.T) {
	design := &ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "mutation_test_design",
		Nets: []ir.Net{
			{Name: "LOGIC_PWR", Pins: []ir.PinRef{{Ref: "REG", Pin: "OUT"}, {Ref: "U1", Pin: "VCC"}}},
		},
	}

	buildContracts := func(consumerMax float64) *contracts.ContractIR {
		contractIR := contracts.NewContractIR()
		contractIR.PutPin("REG", "OUT", contracts.PinContract{
			Role:           contracts.RolePowerOut,
			VoltageNominal: contracts.Float64(5.0),
			Direction:      contracts.DirectionOutput,
		})
		contractIR.PutPin("U1", "VCC", contracts.PinContract{
			Role:       contracts.RolePowerIn,
			VoltageMax: contracts.Float64(consumerMax),
			Direction:  contracts.DirectionInput,
		})
		return contractIR
	}

	clean := SupplyContractRule{}.Check(design, buildContracts(5.5))
	if len(clean) != 0 {
		t.Fatalf("expected 5V provider into 5.5V max consumer to be clean, got %+v", clean)
	}

	mutated := SupplyContractRule{}.Check(design, buildContracts(3.3))
	if len(mutated) != 1 {
		t.Fatalf("expected mutated 3.3V max consumer to fail, got %+v", mutated)
	}
	if mutated[0].Severity != "error" || mutated[0].RuleID != RuleSupplyContract {
		t.Fatalf("expected mutation to trigger supply contract error, got %+v", mutated[0])
	}
}

func TestSupplyContract_MissingLimitsWarns(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/+5V", Pins: []ir.PinRef{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "VCC"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("J1", "1", contracts.PinContract{
		Role:           contracts.RolePowerOut,
		VoltageNominal: contracts.Float64(5.0),
		Direction:      contracts.DirectionOutput,
	})
	contractIR.PutPin("U1", "VCC", contracts.PinContract{
		Role:      contracts.RolePowerIn,
		Direction: contracts.DirectionInput,
	})

	got := SupplyContractRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got %+v", got)
	}
	if got[0].Severity != "warning" {
		t.Fatalf("expected warning for missing limits, got %+v", got[0])
	}
}

func TestLogicLevelContract_CatchesFiveVoltOutputIntoThreeVoltInput(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "GPIO", Pins: []ir.PinRef{{Ref: "U1", Pin: "TX"}, {Ref: "U2", Pin: "RX"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("U1", "TX", contracts.PinContract{
		Role:           contracts.RoleGPIO,
		VoltageNominal: contracts.Float64(5.0),
		Direction:      contracts.DirectionOutput,
	})
	contractIR.PutPin("U2", "RX", contracts.PinContract{
		Role:       contracts.RoleGPIO,
		VoltageMax: contracts.Float64(3.3),
		Direction:  contracts.DirectionInput,
	})

	got := LogicLevelContractRule{}.Check(design, contractIR)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %+v", got)
	}
	if got[0].RuleID != RuleLogicLevelContract {
		t.Fatalf("expected %s, got %+v", RuleLogicLevelContract, got[0])
	}
}

func TestBusRoleContract_I2CMixedSDAAndSCL(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "I2C", Pins: []ir.PinRef{{Ref: "U1", Pin: "SDA"}, {Ref: "U2", Pin: "SCL"}}},
		},
	}
	contractIR := contracts.NewContractIR()
	contractIR.PutPin("U1", "SDA", contracts.PinContract{
		Role:      contracts.RoleI2CSDA,
		Direction: contracts.DirectionBidirectional,
	})
	contractIR.PutPin("U2", "SCL", contracts.PinContract{
		Role:      contracts.RoleI2CSCL,
		Direction: contracts.DirectionBidirectional,
	})

	got := BusRoleContractRule{}.Check(design, contractIR)
	if len(got) == 0 {
		t.Fatal("expected I2C role conflict")
	}
	if got[0].Severity != "error" {
		t.Fatalf("expected error, got %+v", got[0])
	}
}
