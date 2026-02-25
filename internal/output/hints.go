package output

// FixHint returns a deterministic remediation hint for a rule code.
func FixHint(ruleID string) (string, bool) {
	switch ruleID {
	case "DRV_CHANNELS_INVALID":
		return "Set motor_driver.channels to a positive integer.", true
	case "DRV_CHANNELS_INSUFFICIENT":
		return "Increase driver channels or reduce total motor count.", true
	case "BAT_V_INVALID":
		return "Set power.battery.voltage_v to a positive value.", true
	case "DRV_SUPPLY_RANGE":
		return "Align battery voltage with driver supply range, or use compatible driver.", true
	case "DRV_PEAK_LT_STALL":
		return "Use a driver with higher peak current, lower stall-current motors, or add gearing.", true
	case "DRV_CONT_LOW_MARGIN":
		return "Increase driver current headroom or use lower current motor.", true
	case "RAIL_V_INVALID":
		return "Set power.logic_rail.voltage_v to a positive value.", true
	case "LOGIC_V_DRIVER_MISMATCH":
		return "Set logic rail voltage within the driver logic range.", true
	case "LOGIC_V_MCU_MISMATCH":
		return "Match MCU and logic rail voltage, or add level shifting.", true
	case "RAIL_I_UNKNOWN":
		return "Set power.logic_rail.max_current_a to validate logic rail current budget.", true
	case "MCU_LOGIC_V_INVALID":
		return "Set mcu.logic_voltage_v to a positive value.", true
	case "DRV_LOGIC_MIN_V_INVALID":
		return "Set motor_driver.logic_voltage_min_v to a positive value.", true
	case "DRV_LOGIC_MAX_V_INVALID":
		return "Set motor_driver.logic_voltage_max_v to a positive value.", true
	case "DRV_LOGIC_RANGE_INVALID":
		return "Ensure driver logic min voltage is less than or equal to max voltage.", true
	case "LOGIC_LEVEL_MISMATCH":
		return "Use level shifting or choose MCU and driver with compatible logic levels.", true
	case "BATT_PEAK_OVER_C":
		return "Increase battery max current capability, or reduce total motor peak demand.", true
	case "BATT_PEAK_MARGIN_LOW":
		return "Add battery current headroom or reduce peak motor demand.", true
	case "DRV_PEAK_OVERLOAD":
		return "Increase driver peak current capacity or reduce simultaneous motor stall load.", true
	case "DRV_PEAK_MARGIN_LOW":
		return "Increase peak current margin or reduce simultaneous peak motor load.", true
	case "I2C_ADDR_CONFLICT":
		return "Assign unique I2C addresses or isolate devices with a bus multiplexer.", true
	default:
		return "", false
	}
}
