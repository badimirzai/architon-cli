package contracts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// ContractSource is one independent source of deterministic contract data.
// Source order expresses precedence; earlier sources win during ContractIR.Merge.
type ContractSource interface {
	Name() string
	Enrich(design *ir.DesignIR) (*ContractIR, error)
}

// FieldContractSource adapts explicit schematic/BOM fields into contracts.
// Supported fields are intentionally namespaced or plain deterministic keys,
// never scraped free text.
type FieldContractSource struct{}

func (FieldContractSource) Name() string { return "schematic-bom-fields" }

func (s FieldContractSource) Enrich(design *ir.DesignIR) (*ContractIR, error) {
	out := NewContractIR()
	if design == nil {
		return out, nil
	}

	parts := append([]ir.Part(nil), design.Parts...)
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Ref < parts[j].Ref
	})
	for _, part := range parts {
		reqs, err := fieldRequirements(part, s.Name())
		if err != nil {
			return nil, err
		}
		if len(reqs) == 0 {
			continue
		}
		component := out.EnsureComponent(part.Ref)
		component.MPN = contractPartMPN(part)
		component.Source = s.Name()
		out.PutComponent(component)
		for _, req := range reqs {
			out.PutAppliedRequirement(AppliedRequirement{
				Requirement:  req,
				ComponentRef: part.Ref,
				ComponentMPN: component.MPN,
				Source:       s.Name(),
				Provenance: Provenance{
					Source: "schematic_bom_field",
					Detail: "explicit contract field on component " + part.Ref,
				},
			})
			for _, pin := range req.Scope.Pins {
				out.PutPin(part.Ref, pin, PinContract{
					Role:      req.Scope.Role,
					Direction: directionForRequirement(req.Type),
					Source:    s.Name(),
				})
			}
		}
	}
	return out, nil
}

func fieldRequirements(part ir.Part, source string) ([]Requirement, error) {
	fields := normalizedFields(part.Fields)
	reqs := make([]Requirement, 0, 4)

	supplyPins := fieldPins(fields, []string{"architon_supply_pins", "supply_pins", "power_pins"}, []string{"VCC", "VDD", "VIN"})
	if maxV, ok, err := fieldFloat(fields, []string{"architon_supply_abs_max_v", "supply_abs_max_v", "max_voltage", "max_voltage_v"}); err != nil {
		return nil, fmt.Errorf("%s %s supply max: %w", source, part.Ref, err)
	} else if ok {
		reqs = append(reqs, Requirement{
			Type:       ContractSupplyAbsMax,
			Scope:      ContractScope{Pins: supplyPins, Role: RolePowerIn},
			MaxVoltage: Float64(maxV),
			Fix:        "Move the component to a rail within its absolute maximum voltage.",
		})
	}

	recMin, hasRecMin, err := fieldFloat(fields, []string{"architon_supply_recommended_min_v", "supply_recommended_min_v", "recommended_min_voltage_v"})
	if err != nil {
		return nil, fmt.Errorf("%s %s recommended min: %w", source, part.Ref, err)
	}
	recMax, hasRecMax, err := fieldFloat(fields, []string{"architon_supply_recommended_max_v", "supply_recommended_max_v", "recommended_max_voltage_v"})
	if err != nil {
		return nil, fmt.Errorf("%s %s recommended max: %w", source, part.Ref, err)
	}
	if hasRecMin || hasRecMax {
		reqs = append(reqs, Requirement{
			Type:       ContractSupplyRecommendedRange,
			Scope:      ContractScope{Pins: supplyPins, Role: RolePowerIn},
			MinVoltage: optionalFloat(recMin, hasRecMin),
			MaxVoltage: optionalFloat(recMax, hasRecMax),
			Severity:   "warning",
			Fix:        "Use a supply rail inside the recommended operating range.",
		})
	}

	gpioPins := fieldPins(fields, []string{"architon_gpio_pins", "gpio_pins", "io_pins"}, []string{"GPIO*", "IO*", "SDA", "SCL"})
	if maxV, ok, err := fieldFloat(fields, []string{"architon_gpio_abs_max_v", "gpio_abs_max_v", "io_abs_max_v"}); err != nil {
		return nil, fmt.Errorf("%s %s gpio max: %w", source, part.Ref, err)
	} else if ok {
		reqs = append(reqs, Requirement{
			Type:       ContractGPIOAbsMax,
			Scope:      ContractScope{Pins: gpioPins, Role: RoleGPIO},
			MaxVoltage: Float64(maxV),
			Fix:        "Add level shifting or drive the signal at a compatible voltage.",
		})
	}

	vmPins := fieldPins(fields, []string{"architon_motor_vm_pins", "motor_vm_pins", "vm_pins"}, []string{"VM", "VMOT", "VS"})
	vmMin, hasVMMin, err := fieldFloat(fields, []string{"architon_motor_vm_min_v", "motor_vm_min_v", "vm_min_v"})
	if err != nil {
		return nil, fmt.Errorf("%s %s motor VM min: %w", source, part.Ref, err)
	}
	vmMax, hasVMMax, err := fieldFloat(fields, []string{"architon_motor_vm_max_v", "motor_vm_max_v", "vm_max_v"})
	if err != nil {
		return nil, fmt.Errorf("%s %s motor VM max: %w", source, part.Ref, err)
	}
	if hasVMMin || hasVMMax {
		reqs = append(reqs, Requirement{
			Type:       ContractMotorDriverVMRange,
			Scope:      ContractScope{Pins: vmPins, Role: RolePowerIn},
			MinVoltage: optionalFloat(vmMin, hasVMMin),
			MaxVoltage: optionalFloat(vmMax, hasVMMax),
			Fix:        "Use a motor supply rail inside the motor driver's VM range.",
		})
	}

	regPins := fieldPins(fields, []string{"architon_regulator_output_pins", "regulator_output_pins", "output_pins"}, []string{"VOUT", "OUT", "3"})
	if maxA, ok, err := fieldFloat(fields, []string{"architon_regulator_output_current_a", "regulator_output_current_a", "output_current_a"}); err != nil {
		return nil, fmt.Errorf("%s %s regulator current: %w", source, part.Ref, err)
	} else if ok {
		reqs = append(reqs, Requirement{
			Type:       ContractRegulatorOutputCurrent,
			Scope:      ContractScope{Pins: regPins, Role: RoleRegulatorOut},
			MaxCurrent: Float64(maxA),
			Fix:        "Reduce downstream load or choose a regulator with more output current.",
		})
	}

	return reqs, nil
}

func normalizedFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		out[normalizeFieldKey(key)] = strings.TrimSpace(value)
	}
	return out
}

func normalizeFieldKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	var b strings.Builder
	b.Grow(len(key))
	prevUnderscore := false
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func fieldPins(fields map[string]string, keys []string, fallback []string) []string {
	for _, key := range keys {
		value := strings.TrimSpace(fields[key])
		if value == "" {
			continue
		}
		return splitPinList(value)
	}
	out := append([]string(nil), fallback...)
	sort.Strings(out)
	return out
}

func splitPinList(value string) []string {
	raw := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	})
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, pin := range raw {
		pin = strings.TrimSpace(pin)
		if pin == "" {
			continue
		}
		if _, ok := seen[pin]; ok {
			continue
		}
		seen[pin] = struct{}{}
		out = append(out, pin)
	}
	sort.Strings(out)
	return out
}

func fieldFloat(fields map[string]string, keys []string) (float64, bool, error) {
	for _, key := range keys {
		value := strings.TrimSpace(fields[key])
		if value == "" {
			continue
		}
		parsed, err := parseEngineeringFloat(value)
		if err != nil {
			return 0, false, fmt.Errorf("%s=%q: %w", key, value, err)
		}
		return parsed, true, nil
	}
	return 0, false, nil
}

func optionalFloat(value float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return Float64(value)
}
