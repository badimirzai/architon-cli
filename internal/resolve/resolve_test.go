package resolve_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/badimirzai/architon-cli/internal/model"
	"github.com/badimirzai/architon-cli/internal/parts"
	"github.com/badimirzai/architon-cli/internal/resolve"
)

func testPartsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "parts"))
}

func TestResolveAll_FillsDefaultsFromParts(t *testing.T) {
	store := parts.NewStore(testPartsDir(t))

	raw := model.RobotSpec{
		Power: model.PowerSpec{
			Battery: model.Battery{
				VoltageV: 12.0,
			},
			Rail: model.Rail{
				VoltageV:    3.3,
				MaxCurrentA: 1.0,
			},
		},
		MCU: model.MCU{
			Part:          "mcus/esp32s3",
			LogicVoltageV: 0.0, // deliberately unset → must be filled by resolver
		},
		Driver: model.MotorDriver{
			Part: "drivers/tb6612fng", // resolver must pull channels/voltages/currents from parts lib
		},
		Motors: []model.Motor{
			{
				Part:            "motors/generic_dc_12v_gearmotor",
				Name:            "",
				Count:           2,
				NominalCurrentA: 0.0,
				StallCurrentA:   0.0,
			},
		},
	}

	resolved, err := resolve.ResolveAll(raw, store)
	if err != nil {
		t.Fatalf("ResolveAll returned error: %v", err)
	}

	// MCU
	if resolved.MCU.LogicVoltageV == 0 {
		t.Errorf("expected MCU.LogicVoltageV to be filled from part, got 0")
	}

	// Driver
	if resolved.Driver.Channels <= 0 {
		t.Errorf("expected Driver.Channels > 0 after resolving, got %d", resolved.Driver.Channels)
	}
	if resolved.Driver.MotorSupplyMinV == 0 || resolved.Driver.MotorSupplyMaxV == 0 {
		t.Errorf("expected driver motor supply range to be set after resolving, got [%.2f, %.2f]",
			resolved.Driver.MotorSupplyMinV, resolved.Driver.MotorSupplyMaxV)
	}
	if resolved.Driver.PeakPerChA == 0 {
		t.Errorf("expected driver peak current > 0 after resolving, got %.2f", resolved.Driver.PeakPerChA)
	}

	// Motors
	if len(resolved.Motors) != 1 {
		t.Fatalf("expected 1 motor after resolving, got %d", len(resolved.Motors))
	}
	m := resolved.Motors[0]
	if m.StallCurrentA == 0 {
		t.Errorf("expected motor stall current > 0 after resolving, got %.2f", m.StallCurrentA)
	}
	if m.NominalCurrentA == 0 {
		t.Errorf("expected motor nominal current > 0 after resolving, got %.2f", m.NominalCurrentA)
	}
	if m.Name == "" {
		t.Errorf("expected motor name to be filled from part defaults")
	}
}

func TestResolveAll_ExplicitOverrideBeatsPartDefault(t *testing.T) {
	store := parts.NewStore(testPartsDir(t))

	raw := model.RobotSpec{
		Power: model.PowerSpec{
			Battery: model.Battery{
				VoltageV: 12.0,
			},
			Rail: model.Rail{
				VoltageV:    3.3,
				MaxCurrentA: 1.0,
			},
		},
		// Give resolver a valid driver via part so it doesn't fail sanity checks.
		Driver: model.MotorDriver{
			Part: "drivers/tb6612fng",
		},
		// No motors → resolveAll will simply skip the motor loop.
		MCU: model.MCU{
			Part:          "mcus/esp32s3",
			LogicVoltageV: 5.0, // unrealistic but good for asserting the override
		},
	}

	resolved, err := resolve.ResolveAll(raw, store)
	if err != nil {
		t.Fatalf("ResolveAll returned error: %v", err)
	}

	if resolved.MCU.LogicVoltageV != 5.0 {
		t.Errorf("expected explicit MCU.LogicVoltageV override (5.0) to win, got %.2f", resolved.MCU.LogicVoltageV)
	}
}

func TestResolveAll_MotorCountZeroIsError(t *testing.T) {
	store := parts.NewStore(testPartsDir(t))

	raw := model.RobotSpec{
		Power: model.PowerSpec{
			Battery: model.Battery{VoltageV: 12},
			Rail:    model.Rail{VoltageV: 3.3, MaxCurrentA: 1.0},
		},
		MCU: model.MCU{LogicVoltageV: 3.3},
		Driver: model.MotorDriver{
			Part: "drivers/tb6612fng",
		},
		Motors: []model.Motor{
			{
				Count:           0, // invalid
				NominalCurrentA: 1,
				StallCurrentA:   5,
			},
		},
	}

	_, err := resolve.ResolveAll(raw, store)
	if err == nil {
		t.Fatalf("expected ResolveAll to return error when motor count is zero, got nil")
	}
}
