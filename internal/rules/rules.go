package rules

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

const (
	RuleSupplyContract     = "RULE_SUPPLY_CONTRACT"
	RuleLogicLevelContract = "RULE_LOGIC_LEVEL_CONTRACT"
	RuleBusRoleContract    = "RULE_BUS_ROLE_CONTRACT"
)

// Finding is the rule-engine result type before it is adapted into report JSON.
type Finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Net      string `json:"net,omitempty"`
	Message  string `json:"message"`
	Provider string `json:"provider,omitempty"`
	Consumer string `json:"consumer,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Pin      string `json:"pin,omitempty"`
}

// Rule is the only interface rule implementations need. It deliberately exposes
// DesignIR + ContractIR and nothing from any importer package.
type Rule interface {
	ID() string
	Check(design *ir.DesignIR, contracts *contracts.ContractIR) []Finding
}

// DefaultRules returns the scan-time contract rule set.
func DefaultRules() []Rule {
	return []Rule{
		SupplyContractRule{},
		LogicLevelContractRule{},
		BusRoleContractRule{},
	}
}

// CheckAll runs rules and normalizes their rule IDs and ordering.
func CheckAll(design *ir.DesignIR, contractIR *contracts.ContractIR, ruleSet []Rule) []Finding {
	if len(ruleSet) == 0 {
		ruleSet = DefaultRules()
	}
	findings := make([]Finding, 0)
	for _, rule := range ruleSet {
		if rule == nil {
			continue
		}
		for _, finding := range rule.Check(design, contractIR) {
			if finding.RuleID == "" {
				finding.RuleID = rule.ID()
			}
			findings = append(findings, finding)
		}
	}
	sortFindings(findings)
	return findings
}

// SupplyContractRule compares power providers on a net with power consumers on
// the same net. It reports only when contracts contain enough voltage data.
type SupplyContractRule struct{}

func (SupplyContractRule) ID() string { return RuleSupplyContract }

func (r SupplyContractRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}

	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		providers := providerPins(net, contractIR)
		consumers := consumerPins(net, contractIR)
		if len(consumers) == 0 {
			continue
		}
		if len(providers) == 0 {
			if netContract, ok := contractIR.Net(net.Name); ok && netContract.VoltageNominal != nil {
				providers = append(providers, pinContractOnNet{
					net:     net.Name,
					label:   "net " + net.Name,
					voltage: netContract.VoltageNominal,
				})
			}
		}
		if len(providers) == 0 {
			continue
		}

		for _, provider := range providers {
			if provider.voltage == nil {
				continue
			}
			for _, consumer := range consumers {
				if consumer.contract.VoltageMax == nil && consumer.contract.VoltageMin == nil {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "warning",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s has provider %.2fV but %s has no voltage limits",
							net.Name,
							*provider.voltage,
							consumer.label,
						),
						Provider: provider.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
					})
					continue
				}
				if consumer.contract.VoltageMax != nil && greaterThan(*provider.voltage, *consumer.contract.VoltageMax) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "error",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s provides %.2fV but %s allows max %.2fV",
							net.Name,
							*provider.voltage,
							consumer.label,
							*consumer.contract.VoltageMax,
						),
						Provider: provider.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
					})
				}
				if consumer.contract.VoltageMin != nil && lessThan(*provider.voltage, *consumer.contract.VoltageMin) {
					findings = append(findings, Finding{
						RuleID:   r.ID(),
						Severity: "error",
						Net:      net.Name,
						Message: fmt.Sprintf(
							"Net %s provides %.2fV but %s requires min %.2fV",
							net.Name,
							*provider.voltage,
							consumer.label,
							*consumer.contract.VoltageMin,
						),
						Provider: provider.label,
						Consumer: consumer.label,
						Ref:      consumer.ref,
						Pin:      consumer.pin,
					})
				}
			}
		}
	}
	return findings
}

// LogicLevelContractRule catches signal outputs that exceed contracted input
// logic tolerance, for example 5 V GPIO into a 3.3 V-only input.
type LogicLevelContractRule struct{}

func (LogicLevelContractRule) ID() string { return RuleLogicLevelContract }

func (r LogicLevelContractRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}

	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		pins := signalPins(net, contractIR)
		if len(pins) < 2 {
			continue
		}
		outputs := make([]pinContractOnNet, 0)
		inputs := make([]pinContractOnNet, 0)
		for _, pin := range pins {
			if isOutputDirection(pin.contract.Direction) {
				outputs = append(outputs, pin)
			}
			if isInputDirection(pin.contract.Direction) {
				inputs = append(inputs, pin)
			}
		}
		for _, output := range outputs {
			outputVoltage := logicOutputVoltage(output.contract)
			if outputVoltage == nil {
				continue
			}
			for _, input := range inputs {
				if output.ref == input.ref && output.pin == input.pin {
					continue
				}
				if input.contract.VoltageMax == nil {
					continue
				}
				if !greaterThan(*outputVoltage, *input.contract.VoltageMax) {
					continue
				}
				findings = append(findings, Finding{
					RuleID:   r.ID(),
					Severity: "error",
					Net:      net.Name,
					Message: fmt.Sprintf(
						"Net %s has %s driving %.2fV logic into %s max %.2fV",
						net.Name,
						output.label,
						*outputVoltage,
						input.label,
						*input.contract.VoltageMax,
					),
					Provider: output.label,
					Consumer: input.label,
					Ref:      input.ref,
					Pin:      input.pin,
				})
			}
		}
	}
	return findings
}

// BusRoleContractRule checks obvious bus-role conflicts without guessing when
// contracts are incomplete.
type BusRoleContractRule struct{}

func (BusRoleContractRule) ID() string { return RuleBusRoleContract }

func (r BusRoleContractRule) Check(design *ir.DesignIR, contractIR *contracts.ContractIR) []Finding {
	if design == nil || contractIR == nil {
		return nil
	}

	findings := make([]Finding, 0)
	for _, net := range sortedNets(design.Nets) {
		pins := contractedPins(net, contractIR)
		if len(pins) == 0 {
			continue
		}

		hasSDA := false
		hasSCL := false
		for _, pin := range pins {
			switch pin.contract.Role {
			case contracts.RoleI2CSDA:
				hasSDA = true
			case contracts.RoleI2CSCL:
				hasSCL = true
			}
		}
		if !hasSDA && !hasSCL {
			continue
		}
		if hasSDA && hasSCL {
			findings = append(findings, Finding{
				RuleID:   r.ID(),
				Severity: "error",
				Net:      net.Name,
				Message:  fmt.Sprintf("I2C net %s mixes SDA and SCL pin roles", net.Name),
			})
		}

		for _, pin := range pins {
			if pin.contract.Role != contracts.RoleI2CSDA && pin.contract.Role != contracts.RoleI2CSCL {
				if pin.contract.Role == contracts.RoleUnknown || pin.contract.Role == contracts.RoleGround {
					continue
				}
				findings = append(findings, Finding{
					RuleID:   r.ID(),
					Severity: "warning",
					Net:      net.Name,
					Message:  fmt.Sprintf("I2C net %s includes %s with non-I2C role %s", net.Name, pin.label, pin.contract.Role),
					Ref:      pin.ref,
					Pin:      pin.pin,
				})
				continue
			}
			if pin.contract.Direction != contracts.DirectionOutput {
				continue
			}
			if pin.contract.OpenDrain != nil && *pin.contract.OpenDrain {
				continue
			}
			findings = append(findings, Finding{
				RuleID:   r.ID(),
				Severity: "warning",
				Net:      net.Name,
				Message:  fmt.Sprintf("I2C %s should be bidirectional or open-drain compatible on net %s", pin.label, net.Name),
				Ref:      pin.ref,
				Pin:      pin.pin,
			})
		}
	}
	return findings
}

type pinContractOnNet struct {
	net      string
	ref      string
	pin      string
	label    string
	contract contracts.PinContract
	voltage  *float64
}

// providerPins selects pins that are contracted as voltage providers.
func providerPins(net ir.Net, contractIR *contracts.ContractIR) []pinContractOnNet {
	out := make([]pinContractOnNet, 0)
	for _, pinRef := range sortedPinRefs(net.Pins) {
		contract, ok := contractIR.Pin(pinRef.Ref, pinRef.Pin)
		if !ok || !isProviderRole(contract.Role) {
			continue
		}
		out = append(out, pinContractOnNet{
			net:      net.Name,
			ref:      pinRef.Ref,
			pin:      pinRef.Pin,
			label:    pinLabel(pinRef.Ref, pinRef.Pin),
			contract: contract,
			voltage:  providerVoltage(contract),
		})
	}
	return out
}

// consumerPins selects pins that are contracted as power inputs.
func consumerPins(net ir.Net, contractIR *contracts.ContractIR) []pinContractOnNet {
	out := make([]pinContractOnNet, 0)
	for _, pinRef := range sortedPinRefs(net.Pins) {
		contract, ok := contractIR.Pin(pinRef.Ref, pinRef.Pin)
		if !ok || contract.Role != contracts.RolePowerIn {
			continue
		}
		if isSystemContractEvaluatorSource(contract.Source) {
			continue
		}
		out = append(out, pinContractOnNet{
			net:      net.Name,
			ref:      pinRef.Ref,
			pin:      pinRef.Pin,
			label:    pinLabel(pinRef.Ref, pinRef.Pin),
			contract: contract,
		})
	}
	return out
}

func isSystemContractEvaluatorSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "built-in", "schematic-bom-fields":
		return true
	default:
		return false
	}
}

// signalPins filters contracted pins down to signal-bearing roles.
func signalPins(net ir.Net, contractIR *contracts.ContractIR) []pinContractOnNet {
	pins := contractedPins(net, contractIR)
	out := make([]pinContractOnNet, 0, len(pins))
	for _, pin := range pins {
		if isSignalRole(pin.contract.Role) {
			out = append(out, pin)
		}
	}
	return out
}

// contractedPins joins DesignIR connectivity with available pin contracts.
func contractedPins(net ir.Net, contractIR *contracts.ContractIR) []pinContractOnNet {
	out := make([]pinContractOnNet, 0)
	for _, pinRef := range sortedPinRefs(net.Pins) {
		contract, ok := contractIR.Pin(pinRef.Ref, pinRef.Pin)
		if !ok {
			continue
		}
		out = append(out, pinContractOnNet{
			net:      net.Name,
			ref:      pinRef.Ref,
			pin:      pinRef.Pin,
			label:    pinLabel(pinRef.Ref, pinRef.Pin),
			contract: contract,
		})
	}
	return out
}

// providerVoltage prefers nominal voltage, falling back to max when a source
// only described its output limit.
func providerVoltage(contract contracts.PinContract) *float64 {
	if contract.VoltageNominal != nil {
		return contract.VoltageNominal
	}
	return contract.VoltageMax
}

// logicOutputVoltage mirrors providerVoltage for signal-level comparisons.
func logicOutputVoltage(contract contracts.PinContract) *float64 {
	if contract.VoltageNominal != nil {
		return contract.VoltageNominal
	}
	return contract.VoltageMax
}

// isProviderRole is the supply-rule definition of a voltage provider.
func isProviderRole(role contracts.PinRole) bool {
	switch role {
	case contracts.RolePowerOut, contracts.RoleRegulatorOut, contracts.RoleSource:
		return true
	default:
		return false
	}
}

// isSignalRole is the logic-rule definition of a signal pin.
func isSignalRole(role contracts.PinRole) bool {
	switch role {
	case contracts.RoleGPIO, contracts.RoleI2CSDA, contracts.RoleI2CSCL, contracts.RoleSPI, contracts.RoleUART:
		return true
	default:
		return false
	}
}

// isOutputDirection treats bidirectional pins as possible drivers.
func isOutputDirection(direction contracts.Direction) bool {
	return direction == contracts.DirectionOutput || direction == contracts.DirectionBidirectional
}

// isInputDirection treats bidirectional pins as possible receivers.
func isInputDirection(direction contracts.Direction) bool {
	return direction == contracts.DirectionInput || direction == contracts.DirectionBidirectional
}

// greaterThan and lessThan avoid floating-point noise around exact voltages.
func greaterThan(left float64, right float64) bool {
	return left-right > 1e-9 && !nearlyEqual(left, right)
}

func lessThan(left float64, right float64) bool {
	return right-left > 1e-9 && !nearlyEqual(left, right)
}

func nearlyEqual(left float64, right float64) bool {
	return math.Abs(left-right) < 1e-9
}

// sortedNets copies before sorting so rules never mutate DesignIR.
func sortedNets(nets []ir.Net) []ir.Net {
	out := append([]ir.Net(nil), nets...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// sortedPinRefs gives stable per-net rule output.
func sortedPinRefs(pins []ir.PinRef) []ir.PinRef {
	out := append([]ir.PinRef(nil), pins...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref != out[j].Ref {
			return out[i].Ref < out[j].Ref
		}
		return out[i].Pin < out[j].Pin
	})
	return out
}

// sortFindings gives deterministic JSON and CLI output.
func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		if findings[i].Net != findings[j].Net {
			return findings[i].Net < findings[j].Net
		}
		return findings[i].Message < findings[j].Message
	})
}

// pinLabel centralizes human-readable pin labels used in findings.
func pinLabel(ref string, pin string) string {
	ref = strings.TrimSpace(ref)
	pin = strings.TrimSpace(pin)
	if ref == "" {
		return "pin " + pin
	}
	if pin == "" {
		return ref
	}
	return ref + " pin " + pin
}
