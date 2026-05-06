package contracts

import (
	"sort"

	"github.com/badimirzai/architon-cli/internal/ir"
)

const builtInContractSourceName = "built-in"

// BuiltinPartsSource exposes the curated v0.3.1 built-in contracts as one
// ContractSource. Built-in contracts are intentionally minimal. This is NOT a
// general-purpose parts database.
type BuiltinPartsSource struct {
	contracts []SystemContract
}

func NewBuiltinPartsSource() BuiltinPartsSource {
	return BuiltinPartsSource{contracts: BuiltinContracts()}
}

func (s BuiltinPartsSource) Name() string {
	return builtInContractSourceName
}

func (s BuiltinPartsSource) Enrich(design *ir.DesignIR) (*ContractIR, error) {
	out := NewContractIR()
	if design == nil {
		return out, nil
	}
	contracts := s.contracts
	if len(contracts) == 0 {
		contracts = BuiltinContracts()
	}

	parts := append([]ir.Part(nil), design.Parts...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].Ref < parts[j].Ref })
	for _, part := range parts {
		// Built-ins are applied only after a deterministic exact or alias match.
		// Ambiguous aliases are reported as missing data instead of guessed.
		match := MatchPart(part, contracts)
		if !match.Matched {
			if match.Ambiguous {
				out.MissingContractData = append(out.MissingContractData, MissingContractData{
					Kind:    "ambiguous_part_contract",
					Ref:     part.Ref,
					Message: "Ambiguous built-in contract match for " + part.Ref + "; no built-in contract applied",
				})
			}
			continue
		}

		component := out.EnsureComponent(part.Ref)
		component.MPN = match.Contract.MPN
		component.Source = s.Name()
		out.PutComponent(component)
		out.PartMatches = append(out.PartMatches, PartMatch{
			Ref:         part.Ref,
			MPN:         match.Query,
			ContractMPN: match.Contract.MPN,
			Kind:        match.Kind,
			Source:      s.Name(),
			Provenance:  match.Contract.Provenance,
		})

		for _, req := range match.Contract.Requirements {
			// Requirements are copied and bound to the concrete schematic ref
			// before they enter ContractIR. The catalog remains immutable.
			contractID := match.Contract.ID
			if contractID == "" {
				contractID = match.Contract.MPN
			}
			req.Scope.ComponentRef = part.Ref
			req.Scope.MPN = match.Contract.MPN
			req.ContractID = contractID
			req.ContractSource = ContractSourceBuiltIn
			if req.Provenance.Source == "" {
				req.Provenance = match.Contract.Provenance
			}
			applied := AppliedRequirement{
				Requirement:  req,
				ComponentRef: part.Ref,
				ComponentMPN: match.Contract.MPN,
				Source:       s.Name(),
				Provenance:   req.Provenance,
			}
			out.PutAppliedRequirement(applied)
			for _, pin := range concretePinsForRequirement(design, part.Ref, req) {
				out.PutPin(part.Ref, pin.Pin, PinContract{
					Role:      req.Scope.Role,
					Direction: directionForRequirement(req.Type),
					Source:    s.Name(),
				})
			}
		}
		for _, pin := range concretePinsForAliases(design, part.Ref, match.Contract.GroundPins) {
			out.PutPin(part.Ref, pin.Pin, PinContract{
				Role:      RoleGround,
				Direction: DirectionPassive,
				Source:    s.Name(),
			})
		}
	}
	return out, nil
}

// BuiltinContracts returns the small curated v0.3.1 catalog. Built-in contracts
// are intentionally minimal. This is NOT a general-purpose parts database.
func BuiltinContracts() []SystemContract {
	contracts := []SystemContract{
		{
			MPN:          "ESP32-WROOM-32",
			Manufacturer: "Espressif",
			Aliases:      []string{"ESP32 WROOM 32", "ESP32-WROOM", "ESP32WROOM32"},
			Description:  "ESP32 Wi-Fi/Bluetooth module",
			Requirements: []Requirement{
				supplyAbsMax([]string{"+3V3", "3V3", "VCC", "VDD", "VDDA"}, 3.6),
				supplyRecommended([]string{"+3V3", "3V3", "VCC", "VDD", "VDDA"}, 3.0, 3.6),
				gpioAbsMax([]string{"GPIO*", "IO*", "SCL", "SCL/SPC", "SDA", "SDA/SDI", "SDA/SDI/SDO", "TXD*", "RXD*"}, 3.6),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("ESP32-WROOM-32"),
		},
		{
			MPN:          "STM32F103C8T6",
			Manufacturer: "STMicroelectronics",
			Aliases:      []string{"STM32F103", "STM32F103C8", "BLUEPILL"},
			Description:  "STM32F103 Cortex-M3 MCU",
			Requirements: []Requirement{
				supplyAbsMax([]string{"VBAT", "VCC", "VDD", "VDDA"}, 4.0),
				supplyRecommended([]string{"VCC", "VDD", "VDDA"}, 2.0, 3.6),
				gpioAbsMax([]string{"GPIO*", "PA*", "PB*", "PC*", "PD*", "SCL", "SCL/SPC", "SDA", "SDA/SDI", "SDA/SDI/SDO"}, 3.6),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("STM32F103C8T6"),
		},
		{
			MPN:          "RP2040",
			Manufacturer: "Raspberry Pi",
			Aliases:      []string{"RP2040 MCU"},
			Description:  "RP2040 microcontroller",
			Requirements: []Requirement{
				supplyAbsMax([]string{"+3V3", "3V3", "ADC_AVDD", "DVDD", "IOVDD", "USB_VDD", "VCC", "VDD", "VDDIO"}, 3.63),
				supplyRecommended([]string{"+3V3", "3V3", "ADC_AVDD", "IOVDD", "USB_VDD", "VCC", "VDD", "VDDIO"}, 1.8, 3.3),
				gpioAbsMax([]string{"GPIO*", "IO*", "SCL", "SCL/SPC", "SDA", "SDA/SDI", "SDA/SDI/SDO"}, 3.63),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("RP2040"),
		},
		{
			MPN:          "MPU-6050",
			Manufacturer: "TDK InvenSense",
			Aliases:      []string{"MPU6050", "GY-521"},
			Description:  "6-axis IMU",
			Requirements: []Requirement{
				supplyAbsMax([]string{"VCC", "VDD", "VDDIO", "VLOGIC", "VL"}, 3.6),
				supplyRecommended([]string{"VCC", "VDD", "VDDIO", "VLOGIC", "VL"}, 2.375, 3.46),
				gpioAbsMax([]string{"AD0", "FSYNC", "INT", "SCL", "SCL/SPC", "SDA", "SDA/SDI", "SDA/SDI/SDO"}, 3.6),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("MPU-6050"),
		},
		{
			MPN:          "BNO055",
			Manufacturer: "Bosch Sensortec",
			Aliases:      []string{"BNO-055"},
			Description:  "9-axis absolute orientation sensor",
			Requirements: []Requirement{
				supplyAbsMax([]string{"VCC", "VDD", "VDDIO", "VLOGIC", "VL"}, 3.6),
				supplyRecommended([]string{"VCC", "VDD", "VDDIO", "VLOGIC", "VL"}, 2.4, 3.6),
				gpioAbsMax([]string{"INT", "PS0", "PS1", "RST", "SCL", "SCL/SPC", "SDA", "SDA/SDI", "SDA/SDI/SDO"}, 3.6),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("BNO055"),
		},
		{
			MPN:          "AMS1117-3.3",
			Manufacturer: "Advanced Monolithic Systems",
			Aliases:      []string{"AMS1117 3.3", "AMS1117-3V3", "AMS1117"},
			Description:  "3.3 V linear regulator",
			Requirements: []Requirement{
				supplyAbsMax([]string{"IN", "VI", "VIN"}, 15.0),
				regulatorOutputCurrent([]string{"3", "OUT", "VO", "VOUT"}, 1.0),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("AMS1117-3.3"),
		},
		{
			MPN:          "AP2114H-3.3",
			Manufacturer: "Diodes Inc.",
			Aliases:      []string{"AP2114-3.3", "AP2114H-3V3", "AP2114"},
			Description:  "3.3 V low-dropout regulator",
			Requirements: []Requirement{
				supplyAbsMax([]string{"IN", "VI", "VIN"}, 6.5),
				regulatorOutputCurrent([]string{"5", "OUT", "VO", "VOUT"}, 1.0),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("AP2114H-3.3"),
		},
		{
			MPN:          "DRV8833",
			Manufacturer: "Texas Instruments",
			Aliases:      []string{"DRV8833PWPR", "DRV8833PWP"},
			Description:  "Dual H-bridge motor driver",
			Requirements: []Requirement{
				supplyAbsMax(logicSupplyAliases(), 7.0),
				motorVMRange(motorSupplyAliases("VMM"), 2.7, 10.8),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("DRV8833"),
		},
		{
			MPN:          "TB6612FNG",
			Manufacturer: "Toshiba",
			Aliases:      []string{"TB6612", "TB6612FNGC"},
			Description:  "Dual DC motor driver",
			Requirements: []Requirement{
				supplyAbsMax(logicSupplyAliases(), 6.0),
				supplyRecommended(logicSupplyAliases(), 2.7, 5.5),
				motorVMRange(motorSupplyAliases(), 4.5, 13.5),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("TB6612FNG"),
		},
		{
			MPN:          "L298N",
			Manufacturer: "STMicroelectronics",
			Aliases:      []string{"L298", "L298HN"},
			Description:  "Dual full-bridge motor driver",
			Requirements: []Requirement{
				supplyAbsMax([]string{"VCC", "VDD", "VLOGIC", "VL", "VSS"}, 7.0),
				motorVMRange(motorSupplyAliases("VMS"), 5.0, 46.0),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("L298N"),
		},
		{
			MPN:          "PCA9306",
			Manufacturer: "Texas Instruments",
			Aliases:      []string{"PCA9306D", "PCA9306DC"},
			Description:  "I2C level translator",
			Requirements: []Requirement{
				supplyAbsMax([]string{"EN", "VCC", "VDD", "VLOGIC", "VL", "VREF", "VREF1", "VREF2"}, 6.0),
				supplyRecommended([]string{"VREF1"}, 1.2, 3.3),
				supplyRecommended([]string{"VREF2"}, 1.8, 5.5),
				gpioAbsMax([]string{"SCL", "SCL1", "SCL2", "SCL/SPC", "SDA", "SDA1", "SDA2", "SDA/SDI", "SDA/SDI/SDO"}, 6.0),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("PCA9306"),
		},
		{
			MPN:          "TXS0108E",
			Manufacturer: "Texas Instruments",
			Aliases:      []string{"TXS0108", "TXS0108EPWR"},
			Description:  "8-bit bidirectional voltage-level translator",
			Requirements: []Requirement{
				supplyAbsMax([]string{"VCCA"}, 4.6),
				supplyAbsMax([]string{"VCCB"}, 6.5),
				supplyRecommended([]string{"VCCA"}, 1.65, 3.6),
				supplyRecommended([]string{"VCCB"}, 2.3, 5.5),
				gpioAbsMax([]string{"A*", "B*"}, 6.5),
			},
			GroundPins: groundAliases(),
			SourceKind: ContractSourceBuiltIn,
			Provenance: builtInProvenance("TXS0108E"),
		},
	}
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].MPN < contracts[j].MPN })
	return cloneSystemContracts(contracts)
}

func groundAliases() []string {
	return []string{"AGND", "DGND", "GND", "GND1", "GND2", "PGND", "PGND1", "PGND2"}
}

func logicSupplyAliases(extra ...string) []string {
	return append([]string{"VCC", "VDD", "VDDIO", "VLOGIC", "VL", "VREF"}, extra...)
}

func motorSupplyAliases(extra ...string) []string {
	return append([]string{"MOTOR_VM", "VM", "VM1", "VM2", "VM3", "VMOT", "VS", "VS1", "VS2"}, extra...)
}

func supplyAbsMax(pins []string, maxV float64) Requirement {
	return Requirement{
		Type:       ContractSupplyAbsMax,
		Scope:      ContractScope{Pins: cloneStrings(pins), Role: RolePowerIn},
		MaxVoltage: Float64(maxV),
		Fix:        "Move the component to a rail within its absolute maximum voltage.",
	}
}

func supplyRecommended(pins []string, minV float64, maxV float64) Requirement {
	return Requirement{
		Type:       ContractSupplyRecommendedRange,
		Scope:      ContractScope{Pins: cloneStrings(pins), Role: RolePowerIn},
		MinVoltage: Float64(minV),
		MaxVoltage: Float64(maxV),
		Severity:   "WARN",
		Fix:        "Use a supply rail inside the recommended operating range.",
	}
}

func gpioAbsMax(pins []string, maxV float64) Requirement {
	return Requirement{
		Type:       ContractGPIOAbsMax,
		Scope:      ContractScope{Pins: cloneStrings(pins), Role: RoleGPIO},
		MaxVoltage: Float64(maxV),
		Fix:        "Add level shifting or drive the signal at a compatible voltage.",
	}
}

func motorVMRange(pins []string, minV float64, maxV float64) Requirement {
	return Requirement{
		Type:       ContractMotorDriverVMRange,
		Scope:      ContractScope{Pins: cloneStrings(pins), Role: RolePowerIn},
		MinVoltage: Float64(minV),
		MaxVoltage: Float64(maxV),
		Fix:        "Use a motor supply rail inside the motor driver's VM range.",
	}
}

func regulatorOutputCurrent(pins []string, maxA float64) Requirement {
	return Requirement{
		Type:       ContractRegulatorOutputCurrent,
		Scope:      ContractScope{Pins: cloneStrings(pins), Role: RoleRegulatorOut},
		MaxCurrent: Float64(maxA),
		Fix:        "Reduce downstream load or choose a regulator with more output current.",
	}
}

func builtInProvenance(mpn string) Provenance {
	return Provenance{
		Source:   builtInContractSourceName,
		SourceID: mpn,
		Detail:   "curated Architon v0.3.1 built-in contract",
	}
}

func cloneSystemContracts(in []SystemContract) []SystemContract {
	out := make([]SystemContract, len(in))
	for i, contract := range in {
		out[i] = contract
		out[i].Aliases = cloneStrings(contract.Aliases)
		out[i].GroundPins = cloneStrings(contract.GroundPins)
		out[i].Scope.Pins = cloneStrings(contract.Scope.Pins)
		out[i].Requirements = make([]Requirement, len(contract.Requirements))
		copy(out[i].Requirements, contract.Requirements)
		for j := range out[i].Requirements {
			out[i].Requirements[j].Scope.Pins = cloneStrings(contract.Requirements[j].Scope.Pins)
			out[i].Requirements[j].MinVoltage = cloneFloat(contract.Requirements[j].MinVoltage)
			out[i].Requirements[j].MaxVoltage = cloneFloat(contract.Requirements[j].MaxVoltage)
			out[i].Requirements[j].MaxCurrent = cloneFloat(contract.Requirements[j].MaxCurrent)
			out[i].Requirements[j].MinOhms = cloneFloat(contract.Requirements[j].MinOhms)
			out[i].Requirements[j].MaxOhms = cloneFloat(contract.Requirements[j].MaxOhms)
			out[i].Requirements[j].MaxUtilizationPct = cloneFloat(contract.Requirements[j].MaxUtilizationPct)
		}
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func cloneFloat(in *float64) *float64 {
	if in == nil {
		return nil
	}
	return Float64(*in)
}
