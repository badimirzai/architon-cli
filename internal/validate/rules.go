package validate

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/badimirzai/architon-cli/internal/model"
)

type Severity string

const (
	SevError Severity = "ERROR"
	SevWarn  Severity = "WARN"
	SevInfo  Severity = "INFO"
)

type Finding struct {
	Severity Severity
	Code     string
	Message  string
	Path     string
	Location *Location
}

type Report struct {
	Findings []Finding
}

type Location struct {
	File   string
	Line   int
	Column int
}

func (r Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}

func RunAll(spec model.RobotSpec, locs map[string]Location) Report {
	var r Report
	r.Findings = append(r.Findings, ruleDriverChannels(spec, locs)...)
	r.Findings = append(r.Findings, ruleMotorSupplyVoltage(spec, locs)...)
	r.Findings = append(r.Findings, ruleDriverCurrentHeadroom(spec, locs)...)
	r.Findings = append(r.Findings, ruleLogicVoltageCompat(spec, locs)...)
	r.Findings = append(r.Findings, ruleRailCurrentBudget(spec, locs)...)
	r.Findings = append(r.Findings, ruleLogicLevelMisMatch(spec, locs)...)
	r.Findings = append(r.Findings, ruleBatteryCRate(spec, locs)...)
	r.Findings = append(r.Findings, ruleDriverStallOverload(spec, locs)...)
	r.Findings = append(r.Findings, ruleI2CAddressConflict(spec, locs)...)
	return r
}

func withLocation(locs map[string]Location, path string, f Finding) Finding {
	f.Path = path
	if locs == nil {
		return f
	}
	if loc, ok := findLocation(locs, path); ok {
		f.Location = &loc
	}
	return f
}

func findLocation(locs map[string]Location, path string) (Location, bool) {
	if loc, ok := locs[path]; ok {
		return loc, true
	}
	for path != "" {
		if idx := strings.LastIndex(path, "."); idx >= 0 {
			path = path[:idx]
		} else if idx := strings.LastIndex(path, "["); idx >= 0 {
			path = path[:idx]
		} else {
			path = ""
		}
		if loc, ok := locs[path]; ok {
			return loc, true
		}
	}
	return Location{}, false
}

func ruleDriverChannels(spec model.RobotSpec, locs map[string]Location) []Finding {
	totalMotors := 0
	for _, m := range spec.Motors {
		totalMotors += m.Count
	}
	if spec.Driver.Channels <= 0 {
		return []Finding{withLocation(locs, "motor_driver.channels", Finding{
			Severity: SevError,
			Code:     "DRV_CHANNELS_INVALID",
			Message:  "motor_driver.channels must be > 0",
		})}
	}
	if totalMotors > spec.Driver.Channels {
		return []Finding{withLocation(locs, "motor_driver.channels", Finding{
			Severity: SevError,
			Code:     "DRV_CHANNELS_INSUFFICIENT",
			Message:  fmt.Sprintf("motors require %d channels but motor_driver.channels is %d", totalMotors, spec.Driver.Channels),
		})}
	}
	return []Finding{withLocation(locs, "motor_driver.channels", Finding{
		Severity: SevInfo,
		Code:     "DRV_CHANNELS_OK",
		Message:  fmt.Sprintf("driver channels OK: %d motor(s) mapped to %d available channel(s)", totalMotors, spec.Driver.Channels),
	})}
}

func ruleMotorSupplyVoltage(spec model.RobotSpec, locs map[string]Location) []Finding {
	batV := spec.Power.Battery.VoltageV
	if batV < 0 {
		return []Finding{withLocation(locs, "power.battery.voltage_v", Finding{
			Severity: SevError,
			Code:     "BAT_V_INVALID",
			Message:  "power.battery.voltage_v must be > 0",
		})}
	}
	if batV == 0 || spec.Driver.MotorSupplyMinV == 0 || spec.Driver.MotorSupplyMaxV == 0 {
		return nil
	}
	if batV < spec.Driver.MotorSupplyMinV || batV > spec.Driver.MotorSupplyMaxV {
		return []Finding{withLocation(locs, "power.battery.voltage_v", Finding{
			Severity: SevError,
			Code:     "DRV_SUPPLY_RANGE",
			Message: fmt.Sprintf(
				"battery %.2fV outside motor_driver motor supply range [%.2f, %.2f]V",
				batV,
				spec.Driver.MotorSupplyMinV,
				spec.Driver.MotorSupplyMaxV,
			),
		})}
	}
	return nil
}

func ruleDriverCurrentHeadroom(spec model.RobotSpec, locs map[string]Location) []Finding {
	var out []Finding
	for _, m := range spec.Motors {
		if m.Count <= 0 {
			continue
		}
		// Worst case per channel: stall current. If you want to be conservative, require peak >= stall.
		if spec.Driver.PeakPerChA > 0 && m.StallCurrentA > 0 && spec.Driver.PeakPerChA < m.StallCurrentA {
			out = append(out, withLocation(locs, "motor_driver.peak_per_channel_a", Finding{
				Severity: SevError,
				Code:     "DRV_PEAK_LT_STALL",
				Message: fmt.Sprintf(
					"motor_driver.peak_per_channel_a %.2fA < motor %s stall %.2fA (per channel)",
					spec.Driver.PeakPerChA,
					m.Name,
					m.StallCurrentA,
				),
			}))
		}
		// Continuous should exceed nominal with margin
		margin := 1.25
		if spec.Driver.ContinuousPerChA > 0 && m.NominalCurrentA > 0 &&
			spec.Driver.ContinuousPerChA < margin*m.NominalCurrentA {
			out = append(out, withLocation(locs, "motor_driver.continuous_per_channel_a", Finding{
				Severity: SevWarn,
				Code:     "DRV_CONT_LOW_MARGIN",
				Message: fmt.Sprintf(
					"driver continuous rating %.2fA is below recommended %.2fA for motor %s (nominal %.2fA). Risk of overheating or current limiting under sustained load.",
					spec.Driver.ContinuousPerChA,
					margin*m.NominalCurrentA,
					m.Name,
					m.NominalCurrentA,
				),
			}))
		}
	}
	return out
}

func ruleLogicVoltageCompat(spec model.RobotSpec, locs map[string]Location) []Finding {
	lv := spec.Power.Rail.VoltageV
	if lv < 0 {
		return []Finding{withLocation(locs, "power.logic_rail.voltage_v", Finding{
			Severity: SevError,
			Code:     "RAIL_V_INVALID",
			Message:  "power.logic_rail.voltage_v must be > 0",
		})}
	} else if lv == 0 {
		return nil
	}
	if spec.Driver.LogicVoltageMinV == 0 || spec.Driver.LogicVoltageMaxV == 0 {
		return nil
	}
	if lv < spec.Driver.LogicVoltageMinV || lv > spec.Driver.LogicVoltageMaxV {
		return []Finding{withLocation(locs, "power.logic_rail.voltage_v", Finding{
			Severity: SevError,
			Code:     "LOGIC_V_DRIVER_MISMATCH",
			Message: fmt.Sprintf(
				"logic rail %.2fV outside motor_driver logic range [%.2f, %.2f]V",
				lv,
				spec.Driver.LogicVoltageMinV,
				spec.Driver.LogicVoltageMaxV,
			),
		})}
	}
	if spec.MCU.LogicVoltageV > 0 && math.Abs(spec.MCU.LogicVoltageV-lv) > 0.25 {
		return []Finding{withLocation(locs, "mcu.logic_voltage_v", Finding{
			Severity: SevWarn,
			Code:     "LOGIC_V_MCU_MISMATCH",
			Message: fmt.Sprintf(
				"MCU logic %.2fV differs from rail %.2fV, check level shifting",
				spec.MCU.LogicVoltageV,
				lv,
			),
		})}
	}
	return nil
}

func ruleRailCurrentBudget(spec model.RobotSpec, locs map[string]Location) []Finding {
	railMax := spec.Power.Rail.MaxCurrentA
	if railMax <= 0 {
		return []Finding{withLocation(locs, "power.logic_rail.max_current_a", Finding{
			Severity: SevWarn,
			Code:     "RAIL_I_UNKNOWN",
			Message:  "power.logic_rail.max_current_a not set, cannot budget logic rail current",
		})}
	}
	// For v1 we do not model currents precisely. We just nudge the user.
	return []Finding{withLocation(locs, "power.logic_rail.max_current_a", Finding{
		Severity: SevInfo,
		Code:     "RAIL_BUDGET_NOTE",
		Message:  fmt.Sprintf("logic rail budget set to %.2fA. v1 does not estimate MCU and driver logic current yet.", railMax),
	})}
}

func ruleLogicLevelMisMatch(spec model.RobotSpec, locs map[string]Location) []Finding {
	mcuLogicV := spec.MCU.LogicVoltageV
	driverMinV := spec.Driver.LogicVoltageMinV
	driverMaxV := spec.Driver.LogicVoltageMaxV

	var out []Finding
	// Validate voltages before comparing logic levels.
	if mcuLogicV < 0 {
		out = append(out, withLocation(locs, "mcu.logic_voltage_v", Finding{
			Severity: SevError,
			Code:     "MCU_LOGIC_V_INVALID",
			Message:  "mcu.logic_voltage_v must be > 0",
		}))
	}
	if driverMinV < 0 {
		out = append(out, withLocation(locs, "motor_driver.logic_voltage_min_v", Finding{
			Severity: SevError,
			Code:     "DRV_LOGIC_MIN_V_INVALID",
			Message:  "motor_driver.logic_voltage_min_v must be > 0",
		}))
	}
	if driverMaxV < 0 {
		out = append(out, withLocation(locs, "motor_driver.logic_voltage_max_v", Finding{
			Severity: SevError,
			Code:     "DRV_LOGIC_MAX_V_INVALID",
			Message:  "motor_driver.logic_voltage_max_v must be > 0",
		}))
	}
	if driverMinV > 0 && driverMaxV > 0 && driverMinV > driverMaxV {
		out = append(out, withLocation(locs, "motor_driver.logic_voltage_min_v", Finding{
			Severity: SevError,
			Code:     "DRV_LOGIC_RANGE_INVALID",
			Message:  "motor_driver.logic_voltage_min_v must be <= motor_driver.logic_voltage_max_v",
		}))
	}
	if len(out) > 0 {
		return out
	}
	if mcuLogicV == 0 || driverMinV == 0 || driverMaxV == 0 {
		return nil
	}

	if mcuLogicV < driverMinV || mcuLogicV > driverMaxV {
		return []Finding{withLocation(locs, "mcu.logic_voltage_v", Finding{
			Severity: SevError,
			Code:     "LOGIC_LEVEL_MISMATCH",
			Message: fmt.Sprintf(
				"MCU logic %.2fV outside driver logic window [%.2f, %.2f]V",
				mcuLogicV,
				driverMinV,
				driverMaxV,
			),
		})}
	}
	return nil
}

func ruleBatteryCRate(spec model.RobotSpec, locs map[string]Location) []Finding {
	cRate := spec.Power.Battery.CRating
	maxDischargeA := spec.Power.Battery.MaxDischargeA
	capacityAh := spec.Power.Battery.CapacityAh
	maxCurrentA := spec.Power.Battery.MaxCurrentA

	batteryMaxA := 0.0
	sourcePath := ""
	sourceDetail := ""
	switch {
	case maxDischargeA > 0:
		batteryMaxA = maxDischargeA
		sourcePath = yamlPathForRobotSpec("Power", "Battery", "MaxDischargeA")
		sourceDetail = "MaxDischargeA override"
	case capacityAh > 0 && cRate > 0:
		batteryMaxA = capacityAh * cRate
		sourcePath = yamlPathForRobotSpec("Power", "Battery", "CRating")
		sourceDetail = fmt.Sprintf("%.2fAh * %.2fC", capacityAh, cRate)
	case maxCurrentA > 0:
		batteryMaxA = maxCurrentA
		sourcePath = yamlPathForRobotSpec("Power", "Battery", "MaxCurrentA")
		sourceDetail = "max_current_a"
	default:
		return nil
	}

	peakCurrentA := 0.0
	for _, motor := range spec.Motors {
		if motor.StallCurrentA <= 0 || motor.Count <= 0 {
			continue
		}
		peakCurrentA += motor.StallCurrentA * float64(motor.Count)
	}
	if batteryMaxA <= 0 || peakCurrentA <= 0 {
		return nil
	}

	if peakCurrentA > batteryMaxA {
		return []Finding{withLocation(locs, sourcePath, Finding{
			Severity: SevError,
			Code:     "BATT_PEAK_OVER_C",
			Message:  fmt.Sprintf("Peak current %.2fA exceeds battery max %.2fA (%s)", peakCurrentA, batteryMaxA, sourceDetail),
		})}
	}
	if peakCurrentA >= batteryMaxA*0.8 {
		return []Finding{withLocation(locs, sourcePath, Finding{
			Severity: SevWarn,
			Code:     "BATT_PEAK_MARGIN_LOW",
			Message:  fmt.Sprintf("Peak current %.2fA is close to battery max %.2fA (%s)", peakCurrentA, batteryMaxA, sourceDetail),
		})}
	}
	return nil
}

func yamlPathForRobotSpec(fields ...string) string {
	t := reflect.TypeOf(model.RobotSpec{})
	parts := make([]string, 0, len(fields))
	for _, name := range fields {
		field, ok := t.FieldByName(name)
		if !ok {
			return ""
		}
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			return ""
		}
		tag = strings.Split(tag, ",")[0]
		if tag == "" {
			return ""
		}
		parts = append(parts, tag)

		t = field.Type
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			break
		}
	}
	return strings.Join(parts, ".")
}

func ruleDriverStallOverload(spec model.RobotSpec, locs map[string]Location) []Finding {
	driverPeakPerChannelA := spec.Driver.PeakPerChA
	driverChannels := spec.Driver.Channels
	driverPeakTotalA := driverPeakPerChannelA * float64(driverChannels)
	stallTotalA := 0.0
	hasStallData := false

	// Ignore undefined (zero or negative) driver peak or channels.
	if driverPeakPerChannelA <= 0 || driverChannels <= 0 {
		return nil
	}

	for _, motor := range spec.Motors {
		if motor.StallCurrentA <= 0 || motor.Count <= 0 {
			continue
		}
		hasStallData = true
		stallTotalA += motor.StallCurrentA * float64(motor.Count)
	}
	if !hasStallData {
		return nil
	}

	if stallTotalA > driverPeakTotalA {
		return []Finding{withLocation(locs, "motor_driver.peak_per_channel_a", Finding{
			Severity: SevError,
			Code:     "DRV_PEAK_OVERLOAD",
			Message: fmt.Sprintf(
				"Total motor stall %.2fA exceeds driver peak %.2fA (%.2fA per channel x %d)",
				stallTotalA,
				driverPeakTotalA,
				driverPeakPerChannelA,
				driverChannels,
			),
		})}
	} else if stallTotalA >= 0.8*driverPeakTotalA {
		return []Finding{withLocation(locs, "motor_driver.peak_per_channel_a", Finding{
			Severity: SevWarn,
			Code:     "DRV_PEAK_MARGIN_LOW",
			Message:  fmt.Sprintf("Total motor stall %.2fA is close to driver peak %.2fA", stallTotalA, driverPeakTotalA),
		})}
	}

	return nil
}

func ruleI2CAddressConflict(spec model.RobotSpec, locs map[string]Location) []Finding {
	if len(spec.I2CBuses) == 0 {
		return nil
	}

	var out []Finding
	for busIndex, bus := range spec.I2CBuses {
		if len(bus.Devices) == 0 {
			continue
		}
		addresses := make(map[uint16][]string)
		for _, device := range bus.Devices {
			addr := uint16(device.AddressHex)
			if addr == 0 {
				continue
			}
			addresses[addr] = append(addresses[addr], device.Name)
		}
		for addr, names := range addresses {
			if len(names) < 2 {
				continue
			}
			message := fmt.Sprintf(
				"Devices %s share I2C address 0x%X on bus %s",
				strings.Join(names, ", "),
				addr,
				bus.Name,
			)
			path := fmt.Sprintf("i2c_buses[%d].devices", busIndex)
			out = append(out, withLocation(locs, path, Finding{
				Severity: SevError,
				Code:     "I2C_ADDR_CONFLICT",
				Message:  message,
			}))
		}
	}
	return out
}
