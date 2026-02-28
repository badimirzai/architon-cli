package resolve

import (
	"fmt"

	"github.com/badimirzai/architon-cli/internal/model"
	"github.com/badimirzai/architon-cli/internal/parts"
)

// ResolveAll takes a raw RobotSpec (possibly with part references and missing values)
// and returns a fully-populated RobotSpec with fields filled from the parts library.
func ResolveAll(spec model.RobotSpec, store *parts.Store) (model.RobotSpec, error) {
	resolved := spec // copy

	// MCU
	mcu, err := resolveMCU(spec.MCU, store)
	if err != nil {
		return model.RobotSpec{}, err
	}
	resolved.MCU = mcu

	// Motor driver
	drv, err := resolveDriver(spec.Driver, store)
	if err != nil {
		return model.RobotSpec{}, err
	}
	resolved.Driver = drv

	// Motors slice
	motors := make([]model.Motor, len(spec.Motors))
	for i, m := range spec.Motors {
		rm, err := resolveMotor(m, store)
		if err != nil {
			return model.RobotSpec{}, fmt.Errorf("motors[%d]: %w", i, err)
		}
		motors[i] = rm
	}
	resolved.Motors = motors

	// I2C buses
	buses := make([]model.I2CBus, len(spec.I2CBuses))
	for i, bus := range spec.I2CBuses {
		rb, err := resolveI2CBus(bus, store)
		if err != nil {
			return model.RobotSpec{}, fmt.Errorf("i2c_buses[%d]: %w", i, err)
		}
		buses[i] = rb
	}
	resolved.I2CBuses = buses

	return resolved, nil
}

func resolveMCU(in model.MCU, store *parts.Store) (model.MCU, error) {
	out := in

	if in.Part != "" {
		p, err := store.LoadMCU(in.Part)
		if err != nil {
			return model.MCU{}, fmt.Errorf("load mcu part %q: %w", in.Part, err)
		}

		if out.LogicVoltageV == 0 {
			out.LogicVoltageV = p.MCU.LogicVoltageV
		}
		if out.Name == "" {
			out.Name = p.Name
		}
	}

	if out.LogicVoltageV == 0 {
		return model.MCU{}, fmt.Errorf("mcu.logic_voltage_v is missing (no part defaults and no explicit value)")
	}

	return out, nil
}

func resolveDriver(in model.MotorDriver, store *parts.Store) (model.MotorDriver, error) {
	out := in

	if in.Part != "" {
		p, err := store.LoadDriver(in.Part)
		if err != nil {
			return model.MotorDriver{}, fmt.Errorf("load driver part %q: %w", in.Part, err)
		}

		if out.Channels == 0 {
			out.Channels = p.MotorDriver.Channels
		}
		if out.MotorSupplyMinV == 0 {
			out.MotorSupplyMinV = p.MotorDriver.MotorSupplyMinV
		}
		if out.MotorSupplyMaxV == 0 {
			out.MotorSupplyMaxV = p.MotorDriver.MotorSupplyMaxV
		}
		if out.LogicVoltageMinV == 0 {
			out.LogicVoltageMinV = p.MotorDriver.LogicVoltageMinV
		}
		if out.LogicVoltageMaxV == 0 {
			out.LogicVoltageMaxV = p.MotorDriver.LogicVoltageMaxV
		}
		if out.ContinuousPerChA == 0 {
			out.ContinuousPerChA = p.MotorDriver.ContinuousPerChA
		}
		if out.PeakPerChA == 0 {
			out.PeakPerChA = p.MotorDriver.PeakPerChA
		}
		if out.Name == "" {
			out.Name = p.Name
		}
	}

	// Sanity checks after merging
	if out.Channels <= 0 {
		return model.MotorDriver{}, fmt.Errorf("motor_driver.channels must be > 0 after resolving")
	}
	if out.MotorSupplyMinV == 0 || out.MotorSupplyMaxV == 0 {
		return model.MotorDriver{}, fmt.Errorf("motor_driver.motor_supply_min_v and motor_driver.motor_supply_max_v missing after resolving")
	}
	if out.LogicVoltageMinV == 0 || out.LogicVoltageMaxV == 0 {
		return model.MotorDriver{}, fmt.Errorf("motor_driver.logic_voltage_min_v and motor_driver.logic_voltage_max_v missing after resolving")
	}
	if out.PeakPerChA == 0 {
		return model.MotorDriver{}, fmt.Errorf("motor_driver.peak_per_channel_a missing after resolving")
	}

	return out, nil
}

func resolveMotor(in model.Motor, store *parts.Store) (model.Motor, error) {
	out := in

	if in.Part != "" {
		p, err := store.LoadMotor(in.Part)
		if err != nil {
			return model.Motor{}, fmt.Errorf("load motor part %q: %w", in.Part, err)
		}

		if out.VoltageMinV == 0 {
			out.VoltageMinV = p.Motor.VoltageMinV
		}
		if out.VoltageMaxV == 0 {
			out.VoltageMaxV = p.Motor.VoltageMaxV
		}
		if out.NominalCurrentA == 0 {
			out.NominalCurrentA = p.Motor.NominalCurrentA
		}
		if out.StallCurrentA == 0 {
			out.StallCurrentA = p.Motor.StallCurrentA
		}
		if out.Name == "" {
			out.Name = p.Name
		}
	}

	if out.Count <= 0 {
		return model.Motor{}, fmt.Errorf("motors[].count must be > 0")
	}
	if out.StallCurrentA == 0 {
		return model.Motor{}, fmt.Errorf("motors[].stall_current_a missing after resolving")
	}

	return out, nil
}

func resolveI2CBus(in model.I2CBus, store *parts.Store) (model.I2CBus, error) {
	out := in
	devices := make([]model.I2CDevice, len(in.Devices))
	for i, d := range in.Devices {
		rd, err := resolveI2CDevice(d, store)
		if err != nil {
			return model.I2CBus{}, fmt.Errorf("devices[%d]: %w", i, err)
		}
		devices[i] = rd
	}
	out.Devices = devices
	return out, nil
}

func resolveI2CDevice(in model.I2CDevice, store *parts.Store) (model.I2CDevice, error) {
	out := in

	if in.Part != "" {
		p, err := store.LoadI2CSensor(in.Part)
		if err != nil {
			return model.I2CDevice{}, fmt.Errorf("load i2c sensor part %q: %w", in.Part, err)
		}
		if out.Name == "" {
			out.Name = p.Name
		}
		if out.AddressHex == 0 {
			out.AddressHex = p.I2CDevice.AddressHex
		}
	}

	return out, nil
}
