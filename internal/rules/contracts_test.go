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
