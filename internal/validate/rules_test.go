package validate

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/model"
)

func baseSpec() model.RobotSpec {
	return model.RobotSpec{
		Power: model.PowerSpec{
			Battery: model.Battery{
				VoltageV: 12,
			},
			Rail: model.Rail{
				VoltageV:    5,
				MaxCurrentA: 1,
			},
		},
		MCU: model.MCU{
			LogicVoltageV: 5,
		},
		Driver: model.MotorDriver{
			Channels:         2,
			MotorSupplyMinV:  6,
			MotorSupplyMaxV:  16,
			LogicVoltageMinV: 4.5,
			LogicVoltageMaxV: 5.5,
			ContinuousPerChA: 2,
			PeakPerChA:       6,
		},
		Motors: []model.Motor{
			{
				Name:            "M",
				Count:           2,
				NominalCurrentA: 1,
				StallCurrentA:   5,
			},
		},
	}
}

func reportCodes(r Report) map[string]bool {
	return findingCodes(r.Findings)
}

func findingCodes(findings []Finding) map[string]bool {
	codes := make(map[string]bool)
	for _, f := range findings {
		codes[f.Code] = true
	}
	return codes
}

func countSeverity(findings []Finding, severity Severity) int {
	count := 0
	for _, f := range findings {
		if f.Severity == severity {
			count++
		}
	}
	return count
}

func requireCodeSeverity(t *testing.T, findings []Finding, code string, severity Severity) {
	t.Helper()
	for _, f := range findings {
		if f.Code == code {
			if f.Severity != severity {
				t.Fatalf("expected code %q to have severity %q, got %q", code, severity, f.Severity)
			}
			return
		}
	}
	t.Fatalf("expected code %q, got %#v", code, findings)
}

func requireHasCode(t *testing.T, codes map[string]bool, code string) {
	t.Helper()
	if !codes[code] {
		t.Fatalf("expected code %q, got %#v", code, codes)
	}
}

func requireNoCode(t *testing.T, codes map[string]bool, code string) {
	t.Helper()
	if codes[code] {
		t.Fatalf("did not expect code %q, got %#v", code, codes)
	}
}

func TestRuleDriverChannels(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(*model.RobotSpec)
		want           []string
		not            []string
		wantSeverities map[string]Severity
		wantNotes      int
	}{
		{
			name: "invalid_channels",
			mutate: func(s *model.RobotSpec) {
				s.Driver.Channels = 0
			},
			want:           []string{"DRV_CHANNELS_INVALID"},
			not:            []string{"DRV_CHANNELS_OK", "DRV_CHANNELS_INSUFFICIENT"},
			wantSeverities: map[string]Severity{"DRV_CHANNELS_INVALID": SevError},
			wantNotes:      0,
		},
		{
			name: "insufficient_channels",
			mutate: func(s *model.RobotSpec) {
				s.Driver.Channels = 2
				s.Motors[0].Count = 3
			},
			want:           []string{"DRV_CHANNELS_INSUFFICIENT"},
			not:            []string{"DRV_CHANNELS_OK", "DRV_CHANNELS_INVALID"},
			wantSeverities: map[string]Severity{"DRV_CHANNELS_INSUFFICIENT": SevError},
			wantNotes:      0,
		},
		{
			name:      "exact_match_has_no_success_note",
			mutate:    func(s *model.RobotSpec) {},
			not:       []string{"DRV_CHANNELS_OK", "DRV_CHANNELS_INVALID", "DRV_CHANNELS_INSUFFICIENT"},
			wantNotes: 0,
		},
		{
			name: "extra_capacity_has_no_success_note",
			mutate: func(s *model.RobotSpec) {
				s.Driver.Channels = 4
			},
			not:       []string{"DRV_CHANNELS_OK", "DRV_CHANNELS_INVALID", "DRV_CHANNELS_INSUFFICIENT"},
			wantNotes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			findings := ruleDriverChannels(spec, nil)
			codes := findingCodes(findings)
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
			for code, severity := range tt.wantSeverities {
				requireCodeSeverity(t, findings, code, severity)
			}
			if got := countSeverity(findings, SevInfo); got != tt.wantNotes {
				t.Fatalf("expected %d info finding(s), got %d (%#v)", tt.wantNotes, got, findings)
			}
		})
	}
}

func TestRuleMotorSupplyVoltage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "battery_invalid",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.VoltageV = -1
			},
			want: []string{"BAT_V_INVALID"},
			not:  []string{"DRV_SUPPLY_RANGE"},
		},
		{
			name: "battery_out_of_range",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.VoltageV = 20
			},
			want: []string{"DRV_SUPPLY_RANGE"},
			not:  []string{"BAT_V_INVALID"},
		},
		{
			name:   "battery_in_range",
			mutate: func(s *model.RobotSpec) {},
			not:    []string{"BAT_V_INVALID", "DRV_SUPPLY_RANGE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestRuleDriverCurrentHeadroom(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "motor_count_missing",
			mutate: func(s *model.RobotSpec) {
				s.Motors[0].Count = 0
			},
			not: []string{"DRV_PEAK_LT_STALL", "DRV_CONT_LOW_MARGIN"},
		},
		{
			name: "peak_less_than_stall",
			mutate: func(s *model.RobotSpec) {
				s.Driver.PeakPerChA = 3
			},
			want: []string{"DRV_PEAK_LT_STALL"},
			not:  []string{"DRV_CONT_LOW_MARGIN"},
		},
		{
			name: "continuous_low_margin",
			mutate: func(s *model.RobotSpec) {
				s.Driver.ContinuousPerChA = 1.0
			},
			want: []string{"DRV_CONT_LOW_MARGIN"},
			not:  []string{"DRV_PEAK_LT_STALL"},
		},
		{
			name: "peak_and_continuous_issues",
			mutate: func(s *model.RobotSpec) {
				s.Driver.PeakPerChA = 3
				s.Driver.ContinuousPerChA = 1.0
			},
			want: []string{"DRV_PEAK_LT_STALL", "DRV_CONT_LOW_MARGIN"},
			not:  []string{},
		},
		{
			name:   "headroom_ok",
			mutate: func(s *model.RobotSpec) {},
			not:    []string{"DRV_PEAK_LT_STALL", "DRV_CONT_LOW_MARGIN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestRuleLogicVoltageCompat(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "rail_invalid",
			mutate: func(s *model.RobotSpec) {
				s.Power.Rail.VoltageV = -1
			},
			want: []string{"RAIL_V_INVALID"},
			not:  []string{"LOGIC_V_DRIVER_MISMATCH", "LOGIC_V_MCU_MISMATCH"},
		},
		{
			name: "rail_outside_driver_range",
			mutate: func(s *model.RobotSpec) {
				s.Power.Rail.VoltageV = 7
			},
			want: []string{"LOGIC_V_DRIVER_MISMATCH"},
			not:  []string{"RAIL_V_INVALID", "LOGIC_V_MCU_MISMATCH"},
		},
		{
			name: "mcu_mismatch",
			mutate: func(s *model.RobotSpec) {
				s.MCU.LogicVoltageV = 3.3
			},
			want: []string{"LOGIC_V_MCU_MISMATCH"},
			not:  []string{"RAIL_V_INVALID", "LOGIC_V_DRIVER_MISMATCH"},
		},
		{
			name:   "logic_ok",
			mutate: func(s *model.RobotSpec) {},
			not:    []string{"RAIL_V_INVALID", "LOGIC_V_DRIVER_MISMATCH", "LOGIC_V_MCU_MISMATCH"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestRuleLogicLevelMismatch(t *testing.T) {
	// Covers invalid inputs and the mismatch boundary so failures are easy to spot.
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "mcu_voltage_invalid",
			mutate: func(s *model.RobotSpec) {
				s.MCU.LogicVoltageV = -1
			},
			want: []string{"MCU_LOGIC_V_INVALID"},
			not:  []string{"LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "mcu_voltage_missing",
			mutate: func(s *model.RobotSpec) {
				s.MCU.LogicVoltageV = 0
			},
			not: []string{"MCU_LOGIC_V_INVALID", "LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "driver_min_invalid",
			mutate: func(s *model.RobotSpec) {
				s.Driver.LogicVoltageMinV = 0
			},
			not: []string{"DRV_LOGIC_MIN_V_INVALID", "LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "driver_min_negative",
			mutate: func(s *model.RobotSpec) {
				s.Driver.LogicVoltageMinV = -1
			},
			want: []string{"DRV_LOGIC_MIN_V_INVALID"},
			not:  []string{"LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "driver_max_invalid",
			mutate: func(s *model.RobotSpec) {
				s.Driver.LogicVoltageMaxV = 0
			},
			not: []string{"DRV_LOGIC_MAX_V_INVALID", "LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "driver_max_negative",
			mutate: func(s *model.RobotSpec) {
				s.Driver.LogicVoltageMaxV = -1
			},
			want: []string{"DRV_LOGIC_MAX_V_INVALID"},
			not:  []string{"LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "driver_min_greater_than_max",
			mutate: func(s *model.RobotSpec) {
				s.Driver.LogicVoltageMinV = 6.0
				s.Driver.LogicVoltageMaxV = 5.0
			},
			want: []string{"DRV_LOGIC_RANGE_INVALID"},
			not:  []string{"LOGIC_LEVEL_MISMATCH"},
		},
		{
			name: "mcu_outside_driver_window",
			mutate: func(s *model.RobotSpec) {
				s.MCU.LogicVoltageV = 3.3
				s.Driver.LogicVoltageMinV = 5.0
				s.Driver.LogicVoltageMaxV = 6.0
			},
			want: []string{"LOGIC_LEVEL_MISMATCH"},
		},
		{
			name:   "mcu_inside_driver_window",
			mutate: func(s *model.RobotSpec) {},
			not:    []string{"LOGIC_LEVEL_MISMATCH"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestRuleRailCurrentBudget(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(*model.RobotSpec)
		want           []string
		not            []string
		wantSeverities map[string]Severity
		wantNotes      int
	}{
		{
			name: "rail_current_unknown",
			mutate: func(s *model.RobotSpec) {
				s.Power.Rail.MaxCurrentA = 0
			},
			want:           []string{"RAIL_I_UNKNOWN"},
			not:            []string{"RAIL_BUDGET_NOTE"},
			wantSeverities: map[string]Severity{"RAIL_I_UNKNOWN": SevWarn},
			wantNotes:      0,
		},
		{
			name:      "rail_current_set_has_no_success_note",
			mutate:    func(s *model.RobotSpec) {},
			not:       []string{"RAIL_BUDGET_NOTE", "RAIL_I_UNKNOWN"},
			wantNotes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			findings := ruleRailCurrentBudget(spec, nil)
			codes := findingCodes(findings)
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
			for code, severity := range tt.wantSeverities {
				requireCodeSeverity(t, findings, code, severity)
			}
			if got := countSeverity(findings, SevInfo); got != tt.wantNotes {
				t.Fatalf("expected %d info finding(s), got %d (%#v)", tt.wantNotes, got, findings)
			}
		})
	}
}

func TestRuleBatteryCRate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "peak_exceeds_battery_c_rate",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.CapacityAh = 2.0
				s.Power.Battery.CRating = 4.0
				s.Power.Battery.MaxCurrentA = 0
				s.Power.Battery.MaxDischargeA = 0
			},
			want: []string{"BATT_PEAK_OVER_C"},
			not:  []string{"BATT_PEAK_MARGIN_LOW"},
		},
		{
			name: "peak_near_battery_c_rate",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.CapacityAh = 2.0
				s.Power.Battery.CRating = 6.0
				s.Power.Battery.MaxCurrentA = 0
				s.Power.Battery.MaxDischargeA = 0
			},
			want: []string{"BATT_PEAK_MARGIN_LOW"},
			not:  []string{"BATT_PEAK_OVER_C"},
		},
		{
			name: "battery_max_discharge_override",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.MaxDischargeA = 8.0
				s.Power.Battery.CapacityAh = 0
				s.Power.Battery.CRating = 0
				s.Power.Battery.MaxCurrentA = 0
			},
			want: []string{"BATT_PEAK_OVER_C"},
			not:  []string{"BATT_PEAK_MARGIN_LOW"},
		},
		{
			name: "battery_max_current_only_ok",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.MaxCurrentA = 20
				s.Power.Battery.CapacityAh = 0
				s.Power.Battery.CRating = 0
				s.Power.Battery.MaxDischargeA = 0
			},
			not: []string{"BATT_PEAK_MARGIN_LOW", "BATT_PEAK_OVER_C"},
		},
		{
			name: "battery_properties_unknown",
			mutate: func(s *model.RobotSpec) {
				s.Power.Battery.MaxCurrentA = 0
				s.Power.Battery.CapacityAh = 0
				s.Power.Battery.CRating = 0
				s.Power.Battery.MaxDischargeA = 0
			},
			not: []string{"BATT_PEAK_OVER_C", "BATT_PEAK_MARGIN_LOW"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestRuleDriverStallOverload(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "stall_exceeds_driver_peak",
			mutate: func(s *model.RobotSpec) {
				s.Motors[0].StallCurrentA = 8
			},
			want: []string{"DRV_PEAK_OVERLOAD"},
			not:  []string{"DRV_PEAK_MARGIN_LOW"},
		},
		{
			name: "stall_below_warning_threshold",
			mutate: func(s *model.RobotSpec) {
				s.Motors[0].StallCurrentA = 4
			},
			not: []string{"DRV_PEAK_OVERLOAD", "DRV_PEAK_MARGIN_LOW"},
		},
		{
			name: "missing_driver_peak",
			mutate: func(s *model.RobotSpec) {
				s.Driver.PeakPerChA = 0
			},
			not: []string{"DRV_PEAK_OVERLOAD", "DRV_PEAK_MARGIN_LOW"},
		},
		{
			name: "missing_motor_stall",
			mutate: func(s *model.RobotSpec) {
				s.Motors[0].StallCurrentA = 0
			},
			not: []string{"DRV_PEAK_OVERLOAD", "DRV_PEAK_MARGIN_LOW"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestRuleI2CAddressConflict(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RobotSpec)
		want   []string
		not    []string
	}{
		{
			name: "unique_addresses",
			mutate: func(s *model.RobotSpec) {
				s.I2CBuses = []model.I2CBus{
					{
						Name: "bus0",
						Devices: []model.I2CDevice{
							{Name: "imu_left", AddressHex: model.I2CAddress(0x68)},
							{Name: "imu_right", AddressHex: model.I2CAddress(0x69)},
						},
					},
				}
			},
			not: []string{"I2C_ADDR_CONFLICT"},
		},
		{
			name: "conflict_same_bus",
			mutate: func(s *model.RobotSpec) {
				s.I2CBuses = []model.I2CBus{
					{
						Name: "bus0",
						Devices: []model.I2CDevice{
							{Name: "imu_left", AddressHex: model.I2CAddress(0x68)},
							{Name: "imu_right", AddressHex: model.I2CAddress(0x68)},
						},
					},
				}
			},
			want: []string{"I2C_ADDR_CONFLICT"},
		},
		{
			name: "same_address_different_buses",
			mutate: func(s *model.RobotSpec) {
				s.I2CBuses = []model.I2CBus{
					{
						Name: "bus0",
						Devices: []model.I2CDevice{
							{Name: "imu_left", AddressHex: model.I2CAddress(0x68)},
						},
					},
					{
						Name: "bus1",
						Devices: []model.I2CDevice{
							{Name: "imu_right", AddressHex: model.I2CAddress(0x68)},
						},
					},
				}
			},
			not: []string{"I2C_ADDR_CONFLICT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := baseSpec()
			tt.mutate(&spec)
			codes := reportCodes(RunAll(spec, nil))
			for _, c := range tt.want {
				requireHasCode(t, codes, c)
			}
			for _, c := range tt.not {
				requireNoCode(t, codes, c)
			}
		})
	}
}

func TestReadmeMinimalVoltageMismatch(t *testing.T) {
	spec := model.RobotSpec{
		Power: model.PowerSpec{
			Battery: model.Battery{
				VoltageV:    12,
				MaxCurrentA: 10,
			},
			Rail: model.Rail{
				VoltageV:    3.3,
				MaxCurrentA: 1,
			},
		},
		MCU: model.MCU{
			Name:             "Generic MCU",
			LogicVoltageV:    3.3,
			MaxGPIOCurrentmA: 12,
		},
		Driver: model.MotorDriver{
			Name:             "TB6612FNG-like",
			MotorSupplyMinV:  18,
			MotorSupplyMaxV:  24,
			ContinuousPerChA: 0.6,
			PeakPerChA:       6,
			Channels:         1,
			LogicVoltageMinV: 3.0,
			LogicVoltageMaxV: 5.5,
		},
		Motors: []model.Motor{
			{
				Name:            "DC motor",
				Count:           1,
				VoltageMinV:     6,
				VoltageMaxV:     12,
				StallCurrentA:   5,
				NominalCurrentA: 1,
			},
		},
	}

	report := RunAll(spec, nil)
	codes := reportCodes(report)
	requireHasCode(t, codes, "DRV_SUPPLY_RANGE")
	requireHasCode(t, codes, "DRV_CONT_LOW_MARGIN")
}

func TestExitCodeSemantics(t *testing.T) {
	clean := baseSpec()
	if RunAll(clean, nil).HasErrors() {
		t.Fatal("expected clean spec to return exit code 0")
	}

	broken := baseSpec()
	broken.Power.Battery.VoltageV = -1
	if !RunAll(broken, nil).HasErrors() {
		t.Fatal("expected broken spec to return exit code 2")
	}
}
