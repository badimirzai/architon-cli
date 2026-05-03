package rules

import (
	"fmt"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

type SupplyAbsMaxRule struct{}

func (SupplyAbsMaxRule) ID() string { return RuleSupplyAbsMax }

func (r SupplyAbsMaxRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		voltages := voltageObservations(net, contractIR)
		if len(voltages) == 0 {
			continue
		}
		for _, consumer := range consumerPins(net, contractIR) {
			for _, voltage := range voltages {
				source := contractSource(consumer.contract.Source, voltage.source)
				if consumer.contract.AbsVoltageMax != nil && greaterThan(voltage.value, *consumer.contract.AbsVoltageMax) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "error",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s applies %.2fV to %s supply %s, above absolute max %.2fV (source=%s)",
							net.Name,
							voltage.value,
							consumer.label,
							supplyName(consumer.contract),
							*consumer.contract.AbsVoltageMax,
							source,
						),
						Provider: voltage.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
						Source:   source,
					})
				}
				if consumer.contract.AbsVoltageMin != nil && lessThan(voltage.value, *consumer.contract.AbsVoltageMin) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "error",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s applies %.2fV to %s supply %s, below absolute min %.2fV (source=%s)",
							net.Name,
							voltage.value,
							consumer.label,
							supplyName(consumer.contract),
							*consumer.contract.AbsVoltageMin,
							source,
						),
						Provider: voltage.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
						Source:   source,
					})
				}
			}
		}
	}
	return findings
}

type SupplyRecommendedRangeRule struct{}

func (SupplyRecommendedRangeRule) ID() string { return RuleSupplyRange }

func (r SupplyRecommendedRangeRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		voltages := voltageObservations(net, contractIR)
		if len(voltages) == 0 {
			continue
		}
		for _, consumer := range consumerPins(net, contractIR) {
			for _, voltage := range voltages {
				if outsideAbsRange(voltage.value, consumer.contract) {
					continue
				}
				source := contractSource(consumer.contract.Source, voltage.source)
				if consumer.contract.RecommendedVoltageMin != nil && lessThan(voltage.value, *consumer.contract.RecommendedVoltageMin) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "warning",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s applies %.2fV to %s supply %s, below recommended min %.2fV (source=%s)",
							net.Name,
							voltage.value,
							consumer.label,
							supplyName(consumer.contract),
							*consumer.contract.RecommendedVoltageMin,
							source,
						),
						Provider: voltage.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
						Source:   source,
					})
				}
				if consumer.contract.RecommendedVoltageMax != nil && greaterThan(voltage.value, *consumer.contract.RecommendedVoltageMax) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "warning",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s applies %.2fV to %s supply %s, above recommended max %.2fV (source=%s)",
							net.Name,
							voltage.value,
							consumer.label,
							supplyName(consumer.contract),
							*consumer.contract.RecommendedVoltageMax,
							source,
						),
						Provider: voltage.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
						Source:   source,
					})
				}
			}
		}
	}
	return findings
}

type GPIOAbsMaxRule struct{}

func (GPIOAbsMaxRule) ID() string { return RuleGPIOAbsMax }

func (r GPIOAbsMaxRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		pins := signalPins(net, contractIR)
		if len(pins) == 0 {
			continue
		}
		observations := voltageObservations(net, contractIR)
		for _, output := range pins {
			if !isOutputDirection(output.contract.Direction) {
				continue
			}
			if v := logicOutputVoltage(output.contract); v != nil {
				observations = append(observations, voltageObservation{
					value:  *v,
					source: output.contract.Source,
					label:  output.label,
				})
			}
		}
		if len(observations) == 0 {
			continue
		}
		for _, input := range pins {
			if !isInputDirection(input.contract.Direction) {
				continue
			}
			limit := pinAbsMax(input.contract)
			if limit == nil {
				continue
			}
			for _, voltage := range observations {
				if voltage.label == input.label {
					continue
				}
				if !greaterThan(voltage.value, *limit) {
					continue
				}
				source := contractSource(input.contract.Source, voltage.source)
				findings = append(findings, Finding{
					RuleID:   r.ID(),
					Severity: "error",
					Net:      net.Name,
					Message: fmt.Sprintf(
						"Net %s drives %.2fV into %s IO absolute max %.2fV (source=%s)",
						net.Name,
						voltage.value,
						input.label,
						*limit,
						source,
					),
					Provider: voltage.label,
					Consumer: input.label,
					Ref:      input.ref,
					Pin:      input.pin,
					Source:   source,
				})
			}
		}
	}
	return findings
}

type LogicLevelMarginRule struct{}

func (LogicLevelMarginRule) ID() string { return RuleLogicLevelMargin }

func (r LogicLevelMarginRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		pins := signalPins(net, contractIR)
		if len(pins) < 2 {
			continue
		}
		for _, output := range pins {
			if !isOutputDirection(output.contract.Direction) {
				continue
			}
			for _, input := range pins {
				if output.ref == input.ref && output.pin == input.pin {
					continue
				}
				if !isInputDirection(input.contract.Direction) {
					continue
				}
				findings = append(findings, logicHighMarginFinding(r.ID(), net.Name, output, input)...)
				findings = append(findings, logicLowMarginFinding(r.ID(), net.Name, output, input)...)
			}
		}
	}
	return findings
}

type RegulatorOutputCurrentRule struct{}

func (RegulatorOutputCurrentRule) ID() string { return RuleRegulatorCurrent }

func (r RegulatorOutputCurrentRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		for _, provider := range providerPins(net, contractIR) {
			if provider.contract.Role != contracts.RoleRegulatorOut || provider.contract.OutputCurrentMax == nil {
				continue
			}
			load, known := estimatedNetLoadCurrent(net, contractIR, provider)
			if !known {
				continue
			}
			maxCurrent := *provider.contract.OutputCurrentMax
			source := contractSource(provider.contract.Source)
			if greaterThan(load, maxCurrent) {
				findings = append(findings, Finding{
					RuleID:   r.ID(),
					Severity: "error",
					Net:      net.Name,
					Message: fmt.Sprintf(
						"Regulator output %s on net %s has estimated load %.3fA above max %.3fA (source=%s)",
						provider.label,
						net.Name,
						load,
						maxCurrent,
						source,
					),
					Provider: provider.label,
					Ref:      provider.ref,
					Pin:      provider.pin,
					Source:   source,
				})
				continue
			}
			if greaterThan(load, maxCurrent*0.8) {
				findings = append(findings, Finding{
					RuleID:   r.ID(),
					Severity: "warning",
					Net:      net.Name,
					Message: fmt.Sprintf(
						"Regulator output %s on net %s has estimated load %.3fA above 80%% of max %.3fA (source=%s)",
						provider.label,
						net.Name,
						load,
						maxCurrent,
						source,
					),
					Provider: provider.label,
					Ref:      provider.ref,
					Pin:      provider.pin,
					Source:   source,
				})
			}
		}
	}
	return findings
}

type MotorDriverVMRangeRule struct{}

func (MotorDriverVMRangeRule) ID() string { return RuleMotorDriverVMRange }

func (r MotorDriverVMRangeRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		voltages := voltageObservations(net, contractIR)
		if len(voltages) == 0 {
			continue
		}
		for _, pin := range consumerPins(net, contractIR) {
			if !pin.contract.MotorSupply {
				continue
			}
			for _, voltage := range voltages {
				source := contractSource(pin.contract.Source, voltage.source)
				if pin.contract.AbsVoltageMax != nil && greaterThan(voltage.value, *pin.contract.AbsVoltageMax) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "error",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Motor driver VM net %s applies %.2fV to %s above absolute max %.2fV (source=%s)",
							net.Name,
							voltage.value,
							pin.label,
							*pin.contract.AbsVoltageMax,
							source,
						),
						Provider: voltage.label,
						Consumer: pin.label,
						Ref:      pin.ref,
						Pin:      pin.pin,
						Source:   source,
					})
					continue
				}
				if outsideAbsRange(voltage.value, pin.contract) {
					continue
				}
				if pin.contract.RecommendedVoltageMin != nil && lessThan(voltage.value, *pin.contract.RecommendedVoltageMin) {
					findings = append(findings, motorVMWarning(r.ID(), net.Name, voltage, pin, "below recommended min", *pin.contract.RecommendedVoltageMin, source))
				}
				if pin.contract.RecommendedVoltageMax != nil && greaterThan(voltage.value, *pin.contract.RecommendedVoltageMax) {
					findings = append(findings, motorVMWarning(r.ID(), net.Name, voltage, pin, "above recommended max", *pin.contract.RecommendedVoltageMax, source))
				}
			}
		}
	}
	return findings
}

type MotorDriverCurrentMarginRule struct{}

func (MotorDriverCurrentMarginRule) ID() string { return RuleMotorDriverCurrent }

func (r MotorDriverCurrentMarginRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		for _, output := range contractedPins(net, contractIR) {
			if !output.contract.MotorOutput {
				continue
			}
			component, ok := contractIR.Components[output.ref]
			if !ok || component.MotorDriver == nil {
				continue
			}
			for _, loadPin := range contractedPins(net, contractIR) {
				if loadPin.ref == output.ref {
					continue
				}
				source := contractSource(output.contract.Source, loadPin.contract.Source)
				if component.MotorDriver.PeakOutputCurrentA != nil {
					if peak := loadPin.contract.CurrentMax; peak != nil && greaterThan(*peak, *component.MotorDriver.PeakOutputCurrentA) {
						findings = append(findings, Finding{
							RuleID:   r.ID(),
							Severity: "error",
							Net:      net.Name,
							Message: fmt.Sprintf(
								"Motor load %s peak %.3fA exceeds driver %s peak %.3fA (source=%s)",
								loadPin.label,
								*peak,
								output.label,
								*component.MotorDriver.PeakOutputCurrentA,
								source,
							),
							Provider: output.label,
							Consumer: loadPin.label,
							Ref:      output.ref,
							Pin:      output.pin,
							Source:   source,
						})
					}
				}
				if component.MotorDriver.ContinuousOutputCurrentA != nil {
					if nominal := loadPin.contract.TypicalCurrent; nominal != nil && greaterThan(*nominal, *component.MotorDriver.ContinuousOutputCurrentA) {
						severity := "warning"
						if greaterThan(*nominal, *component.MotorDriver.ContinuousOutputCurrentA*1.2) {
							severity = "error"
						}
						findings = append(findings, Finding{
							RuleID:   r.ID(),
							Severity: severity,
							Net:      net.Name,
							Message: fmt.Sprintf(
								"Motor load %s nominal %.3fA exceeds driver %s continuous %.3fA (source=%s)",
								loadPin.label,
								*nominal,
								output.label,
								*component.MotorDriver.ContinuousOutputCurrentA,
								source,
							),
							Provider: output.label,
							Consumer: loadPin.label,
							Ref:      output.ref,
							Pin:      output.pin,
							Source:   source,
						})
					}
				}
			}
		}
	}
	return findings
}

type ProtectionClampCurrentRule struct{}

func (ProtectionClampCurrentRule) ID() string { return RuleProtectionClamp }

func (r ProtectionClampCurrentRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}
	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		voltages := voltageObservations(net, contractIR)
		if len(voltages) == 0 {
			continue
		}
		for _, pin := range signalPins(net, contractIR) {
			if pin.contract.ClampCurrentMaxMA == nil || pinAbsMax(pin.contract) == nil {
				continue
			}
			for _, voltage := range voltages {
				absMax := pinAbsMax(pin.contract)
				if !greaterThan(voltage.value, *absMax) {
					continue
				}
				source := contractSource(pin.contract.Source, voltage.source)
				if pin.contract.CurrentMax == nil {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "warning",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s exceeds %s IO max %.2fV and clamp current cannot be estimated against %.1fmA limit (source=%s)",
							net.Name,
							pin.label,
							*absMax,
							*pin.contract.ClampCurrentMaxMA,
							source,
						),
						Consumer: pin.label,
						Ref:      pin.ref,
						Pin:      pin.pin,
						Source:   source,
					})
					continue
				}
				clampMA := *pin.contract.CurrentMax * 1000
				if greaterThan(clampMA, *pin.contract.ClampCurrentMaxMA) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "error",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Estimated clamp current %.1fmA into %s exceeds %.1fmA limit (source=%s)",
							clampMA,
							pin.label,
							*pin.contract.ClampCurrentMaxMA,
							source,
						),
						Consumer: pin.label,
						Ref:      pin.ref,
						Pin:      pin.pin,
						Source:   source,
					})
				}
			}
		}
	}
	return findings
}

type voltageObservation struct {
	value  float64
	source string
	label  string
}

func voltageObservations(net ir.Net, contractIR *contracts.ContractIR) []voltageObservation {
	out := make([]voltageObservation, 0, 2)
	if netContract, ok := contractIR.Net(net.Name); ok && netContract.VoltageNominal != nil {
		out = append(out, voltageObservation{
			value:  *netContract.VoltageNominal,
			source: netContract.Source,
			label:  "net " + net.Name,
		})
	}
	for _, provider := range providerPins(net, contractIR) {
		if provider.voltage == nil {
			continue
		}
		out = append(out, voltageObservation{
			value:  *provider.voltage,
			source: provider.contract.Source,
			label:  provider.label,
		})
	}
	return out
}

func outsideAbsRange(value float64, contract contracts.PinContract) bool {
	if contract.AbsVoltageMax != nil && greaterThan(value, *contract.AbsVoltageMax) {
		return true
	}
	if contract.AbsVoltageMin != nil && lessThan(value, *contract.AbsVoltageMin) {
		return true
	}
	return false
}

func pinAbsMax(contract contracts.PinContract) *float64 {
	if contract.AbsVoltageMax != nil {
		return contract.AbsVoltageMax
	}
	return contract.VoltageMax
}

func supplyName(contract contracts.PinContract) string {
	name := strings.TrimSpace(contract.SupplyName)
	if name == "" {
		return "input"
	}
	return name
}

func logicHighMarginFinding(ruleID string, netName string, output pinContractOnNet, input pinContractOnNet) []Finding {
	if output.contract.VOHMin == nil || input.contract.VIHMin == nil {
		return nil
	}
	if !lessThan(*output.contract.VOHMin, *input.contract.VIHMin) {
		return nil
	}
	deficit := *input.contract.VIHMin - *output.contract.VOHMin
	severity := marginSeverity(deficit)
	source := contractSource(output.contract.Source, input.contract.Source)
	return []Finding{{
		RuleID:   ruleID,
		Severity: severity,
		Net:      netName,
		Message: fmt.Sprintf(
			"Logic high margin on net %s is negative: %s VOH min %.2fV below %s VIH min %.2fV by %.2fV (source=%s)",
			netName,
			output.label,
			*output.contract.VOHMin,
			input.label,
			*input.contract.VIHMin,
			deficit,
			source,
		),
		Provider: output.label,
		Consumer: input.label,
		Ref:      input.ref,
		Pin:      input.pin,
		Source:   source,
	}}
}

func logicLowMarginFinding(ruleID string, netName string, output pinContractOnNet, input pinContractOnNet) []Finding {
	if output.contract.VOLMax == nil || input.contract.VILMax == nil {
		return nil
	}
	if !greaterThan(*output.contract.VOLMax, *input.contract.VILMax) {
		return nil
	}
	deficit := *output.contract.VOLMax - *input.contract.VILMax
	severity := marginSeverity(deficit)
	source := contractSource(output.contract.Source, input.contract.Source)
	return []Finding{{
		RuleID:   ruleID,
		Severity: severity,
		Net:      netName,
		Message: fmt.Sprintf(
			"Logic low margin on net %s is negative: %s VOL max %.2fV above %s VIL max %.2fV by %.2fV (source=%s)",
			netName,
			output.label,
			*output.contract.VOLMax,
			input.label,
			*input.contract.VILMax,
			deficit,
			source,
		),
		Provider: output.label,
		Consumer: input.label,
		Ref:      input.ref,
		Pin:      input.pin,
		Source:   source,
	}}
}

func marginSeverity(deficit float64) string {
	if deficit > 0.2 || nearlyEqual(deficit, 0.2) {
		return "error"
	}
	return "warning"
}

func estimatedNetLoadCurrent(net ir.Net, contractIR *contracts.ContractIR, provider pinContractOnNet) (float64, bool) {
	total := 0.0
	known := false
	for _, pin := range contractedPins(net, contractIR) {
		if pin.ref == provider.ref && pin.pin == provider.pin {
			continue
		}
		if pin.contract.Role != contracts.RolePowerIn {
			continue
		}
		if current := estimatedPinCurrent(pin.contract); current != nil {
			total += *current
			known = true
		}
	}
	return total, known
}

func estimatedPinCurrent(contract contracts.PinContract) *float64 {
	if contract.TypicalCurrent != nil {
		return contract.TypicalCurrent
	}
	if contract.CurrentMax != nil {
		return contract.CurrentMax
	}
	return nil
}

func motorVMWarning(ruleID string, netName string, voltage voltageObservation, pin pinContractOnNet, phrase string, limit float64, source string) Finding {
	return Finding{
		RuleID:   ruleID,
		Severity: "warning",
		Net:      netName,
		Message: fmt.Sprintf(
			"Motor driver VM net %s applies %.2fV to %s, %s %.2fV (source=%s)",
			netName,
			voltage.value,
			pin.label,
			phrase,
			limit,
			source,
		),
		Provider: voltage.label,
		Consumer: pin.label,
		Ref:      pin.ref,
		Pin:      pin.pin,
		Source:   source,
	}
}

func contractSource(sources ...string) string {
	for _, source := range sources {
		source = strings.TrimSpace(strings.ToLower(source))
		switch {
		case source == "":
			continue
		case strings.Contains(source, "meta"):
			return "meta.yaml"
		case strings.Contains(source, "schematic"):
			return "schematic"
		case strings.Contains(source, "parts-library"):
			return "parts-library"
		case strings.Contains(source, "inferred") || strings.Contains(source, "initial") || strings.Contains(source, "net-voltage") || strings.Contains(source, "net_name") || strings.Contains(source, "regulator"):
			return "inferred"
		default:
			return source
		}
	}
	return "inferred"
}
