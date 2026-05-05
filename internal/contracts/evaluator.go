package contracts

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// Finding is a contract-evaluator finding before it is adapted into report JSON.
type Finding struct {
	RuleID       string     `json:"rule_id"`
	Severity     string     `json:"severity"`
	Message      string     `json:"message"`
	ComponentRef string     `json:"component_ref,omitempty"`
	Net          string     `json:"net,omitempty"`
	Pin          string     `json:"pin,omitempty"`
	Source       string     `json:"source,omitempty"`
	Provenance   Provenance `json:"provenance,omitempty"`
	Fix          string     `json:"fix,omitempty"`
}

// EnabledRuleIDs returns the deterministic contract evaluator rule set.
func EnabledRuleIDs() []string {
	return []string{
		string(ContractSupplyAbsMax),
		string(ContractSupplyRecommendedRange),
		string(ContractGPIOAbsMax),
		string(ContractMotorDriverVMRange),
		string(ContractRegulatorOutputCurrent),
	}
}

// Evaluate runs deterministic contract requirements that are not represented by
// the older generic ContractIR rules.
func Evaluate(design *ir.DesignIR, contractIR *ContractIR) []Finding {
	if design == nil || contractIR == nil || len(contractIR.AppliedRequirements) == 0 {
		return nil
	}

	reqs := append([]AppliedRequirement(nil), contractIR.AppliedRequirements...)
	sortAppliedRequirements(reqs)
	partsByRef := partIndex(design)
	findings := make([]Finding, 0)
	absVoltageViolations := map[string]struct{}{}

	for _, req := range reqs {
		connections := connectedPinsForRequirement(design, req)
		for _, conn := range connections {
			switch req.Type {
			case ContractSupplyAbsMax:
				// Absolute maximum checks are errors. Recommended-range checks
				// are suppressed for the same pin/net when this check already
				// failed, so users see the stronger finding first.
				voltage, ok := netVoltage(contractIR, conn.Net)
				if !ok || req.MaxVoltage == nil || !greaterThanVoltage(voltage, *req.MaxVoltage) {
					continue
				}
				absVoltageViolations[voltageViolationKey(req.ComponentRef, conn.Pin, conn.Net)] = struct{}{}
				findings = append(findings, Finding{
					RuleID:       string(req.Type),
					Severity:     severityOrDefault(req.Severity, "error"),
					ComponentRef: req.ComponentRef,
					Net:          conn.Net,
					Pin:          conn.Pin,
					Source:       req.Source,
					Provenance:   req.Provenance,
					Fix:          req.Fix,
					Message: messageOrDefault(req.Message, fmt.Sprintf(
						"%s pin %s on net %s sees %.2fV above absolute maximum %.2fV",
						req.ComponentRef,
						conn.Pin,
						conn.Net,
						voltage,
						*req.MaxVoltage,
					)),
				})
			case ContractSupplyRecommendedRange:
				voltage, ok := netVoltage(contractIR, conn.Net)
				if !ok {
					continue
				}
				if _, absolute := absVoltageViolations[voltageViolationKey(req.ComponentRef, conn.Pin, conn.Net)]; absolute {
					continue
				}
				if req.MinVoltage != nil && lessThanVoltage(voltage, *req.MinVoltage) {
					findings = append(findings, recommendedVoltageFinding(req, conn, voltage, "below", *req.MinVoltage))
					continue
				}
				if req.MaxVoltage != nil && greaterThanVoltage(voltage, *req.MaxVoltage) {
					findings = append(findings, recommendedVoltageFinding(req, conn, voltage, "above", *req.MaxVoltage))
				}
			case ContractGPIOAbsMax:
				voltage, ok := netVoltage(contractIR, conn.Net)
				if !ok || req.MaxVoltage == nil || !greaterThanVoltage(voltage, *req.MaxVoltage) {
					continue
				}
				findings = append(findings, Finding{
					RuleID:       string(req.Type),
					Severity:     severityOrDefault(req.Severity, "error"),
					ComponentRef: req.ComponentRef,
					Net:          conn.Net,
					Pin:          conn.Pin,
					Source:       req.Source,
					Provenance:   req.Provenance,
					Fix:          req.Fix,
					Message: messageOrDefault(req.Message, fmt.Sprintf(
						"%s GPIO pin %s on net %s sees %.2fV above absolute maximum %.2fV",
						req.ComponentRef,
						conn.Pin,
						conn.Net,
						voltage,
						*req.MaxVoltage,
					)),
				})
			case ContractMotorDriverVMRange:
				voltage, ok := netVoltage(contractIR, conn.Net)
				if !ok {
					continue
				}
				if req.MinVoltage != nil && lessThanVoltage(voltage, *req.MinVoltage) {
					findings = append(findings, motorVMFinding(req, conn, voltage, "below", *req.MinVoltage))
					continue
				}
				if req.MaxVoltage != nil && greaterThanVoltage(voltage, *req.MaxVoltage) {
					findings = append(findings, motorVMFinding(req, conn, voltage, "above", *req.MaxVoltage))
				}
			case ContractRegulatorOutputCurrent:
				// Current is optional input data. If no downstream load current
				// can be read from contracts or BOM fields, this rule stays quiet.
				if req.MaxCurrent == nil {
					continue
				}
				load, ok := outputLoadCurrent(design, contractIR, partsByRef, req.ComponentRef, conn.Net)
				if !ok || !greaterThanCurrent(load, *req.MaxCurrent) {
					continue
				}
				findings = append(findings, Finding{
					RuleID:       string(req.Type),
					Severity:     severityOrDefault(req.Severity, "error"),
					ComponentRef: req.ComponentRef,
					Net:          conn.Net,
					Pin:          conn.Pin,
					Source:       req.Source,
					Provenance:   req.Provenance,
					Fix:          req.Fix,
					Message: messageOrDefault(req.Message, fmt.Sprintf(
						"%s regulator output pin %s on net %s has %.2fA load above %.2fA rating",
						req.ComponentRef,
						conn.Pin,
						conn.Net,
						load,
						*req.MaxCurrent,
					)),
				})
			}
		}
	}

	sortContractFindings(findings)
	return findings
}

type connectedPin struct {
	Ref  string
	Pin  string
	Name string
	Net  string
}

func connectedPinsForRequirement(design *ir.DesignIR, req AppliedRequirement) []connectedPin {
	if design == nil || strings.TrimSpace(req.ComponentRef) == "" {
		return nil
	}
	out := make([]connectedPin, 0, 2)
	seen := map[string]struct{}{}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if strings.TrimSpace(pin.Ref) != req.ComponentRef {
				continue
			}
			// KiCad netlists provide pin numbers and often pin functions. Match
			// either one so contracts work with symbols that use names such as
			// VDD as well as packages that expose only numeric pins.
			if !pinMatchesAny(req.Scope.Pins, pin.Pin, pin.Name) {
				continue
			}
			key := net.Name + "\x00" + pin.Pin
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, connectedPin{Ref: pin.Ref, Pin: pin.Pin, Name: pin.Name, Net: net.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Net != out[j].Net {
			return out[i].Net < out[j].Net
		}
		return out[i].Pin < out[j].Pin
	})
	return out
}

func concretePinsForRequirement(design *ir.DesignIR, ref string, req Requirement) []ir.Pin {
	if design == nil {
		return patternPins(ref, req.Scope.Pins)
	}
	applied := AppliedRequirement{Requirement: req, ComponentRef: ref}
	connections := connectedPinsForRequirement(design, applied)
	if len(connections) == 0 {
		return patternPins(ref, req.Scope.Pins)
	}
	out := make([]ir.Pin, 0, len(connections))
	for _, conn := range connections {
		out = append(out, ir.Pin{Ref: ref, Pin: conn.Pin, Name: conn.Name})
	}
	return out
}

func patternPins(ref string, patterns []string) []ir.Pin {
	out := make([]ir.Pin, 0, len(patterns))
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" || strings.Contains(pattern, "*") {
			continue
		}
		out = append(out, ir.Pin{Ref: ref, Pin: pattern})
	}
	return out
}

func pinMatchesAny(patterns []string, pin string, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if pinMatches(pattern, pin) || pinMatches(pattern, name) {
			return true
		}
	}
	return false
}

func pinMatches(pattern string, pin string) bool {
	pattern = strings.TrimSpace(pattern)
	pin = strings.TrimSpace(pin)
	if pattern == "" || pin == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	normalizedPattern := normalizePin(pattern)
	normalizedPin := normalizePin(pin)
	if strings.HasSuffix(normalizedPattern, "*") {
		return strings.HasPrefix(normalizedPin, strings.TrimSuffix(normalizedPattern, "*"))
	}
	return normalizedPattern == normalizedPin
}

func normalizePin(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '*' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func netVoltage(contractIR *ContractIR, net string) (float64, bool) {
	contract, ok := contractIR.Net(net)
	if !ok || contract.VoltageNominal == nil {
		return 0, false
	}
	return *contract.VoltageNominal, true
}

func recommendedVoltageFinding(req AppliedRequirement, conn connectedPin, voltage float64, direction string, limit float64) Finding {
	return Finding{
		RuleID:       string(req.Type),
		Severity:     severityOrDefault(req.Severity, "warning"),
		ComponentRef: req.ComponentRef,
		Net:          conn.Net,
		Pin:          conn.Pin,
		Source:       req.Source,
		Provenance:   req.Provenance,
		Fix:          req.Fix,
		Message: messageOrDefault(req.Message, fmt.Sprintf(
			"%s pin %s on net %s sees %.2fV %s recommended limit %.2fV",
			req.ComponentRef,
			conn.Pin,
			conn.Net,
			voltage,
			direction,
			limit,
		)),
	}
}

func motorVMFinding(req AppliedRequirement, conn connectedPin, voltage float64, direction string, limit float64) Finding {
	return Finding{
		RuleID:       string(req.Type),
		Severity:     severityOrDefault(req.Severity, "error"),
		ComponentRef: req.ComponentRef,
		Net:          conn.Net,
		Pin:          conn.Pin,
		Source:       req.Source,
		Provenance:   req.Provenance,
		Fix:          req.Fix,
		Message: messageOrDefault(req.Message, fmt.Sprintf(
			"%s motor supply pin %s on net %s sees %.2fV %s VM limit %.2fV",
			req.ComponentRef,
			conn.Pin,
			conn.Net,
			voltage,
			direction,
			limit,
		)),
	}
}

func outputLoadCurrent(design *ir.DesignIR, contractIR *ContractIR, parts map[string]ir.Part, providerRef string, netName string) (float64, bool) {
	total := 0.0
	found := false
	seenRefs := map[string]struct{}{}
	for _, net := range design.Nets {
		if net.Name != netName {
			continue
		}
		for _, pin := range net.Pins {
			ref := strings.TrimSpace(pin.Ref)
			if ref == "" || ref == providerRef {
				continue
			}
			if _, ok := seenRefs[ref]; ok {
				continue
			}
			seenRefs[ref] = struct{}{}

			if current, ok := loadCurrentFromPinContract(contractIR, ref, pin.Pin); ok {
				total += current
				found = true
				continue
			}
			if current, ok := loadCurrentFromPart(parts[ref]); ok {
				total += current
				found = true
			}
		}
	}
	return total, found
}

func loadCurrentFromPinContract(contractIR *ContractIR, ref string, pin string) (float64, bool) {
	contract, ok := contractIR.Pin(ref, pin)
	if !ok || contract.CurrentMax == nil {
		return 0, false
	}
	return *contract.CurrentMax, true
}

func loadCurrentFromPart(part ir.Part) (float64, bool) {
	fields := normalizedFields(part.Fields)
	current, ok, err := fieldFloat(fields, []string{
		"architon_current_a",
		"architon_load_current_a",
		"load_current_a",
		"supply_current_a",
		"current_a",
		"max_current_a",
		"nominal_current_a",
		"current",
	})
	if err != nil || !ok {
		return 0, false
	}
	return current, true
}

func partIndex(design *ir.DesignIR) map[string]ir.Part {
	out := make(map[string]ir.Part, len(design.Parts))
	for _, part := range design.Parts {
		out[part.Ref] = part
	}
	return out
}

func parseEngineeringFloat(value string) (float64, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, ",", "")
	if value == "" {
		return 0, fmt.Errorf("empty number")
	}
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "ma"):
		multiplier = 0.001
		value = strings.TrimSpace(strings.TrimSuffix(value, "ma"))
	case strings.HasSuffix(value, "a"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "a"))
	case strings.HasSuffix(value, "mv"):
		multiplier = 0.001
		value = strings.TrimSpace(strings.TrimSuffix(value, "mv"))
	case strings.HasSuffix(value, "v"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "v"))
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return parsed * multiplier, nil
}

func directionForRequirement(contractType ContractType) Direction {
	switch contractType {
	case ContractRegulatorOutputCurrent:
		return DirectionOutput
	case ContractGPIOAbsMax:
		return DirectionBidirectional
	default:
		return DirectionInput
	}
}

func severityOrDefault(severity string, fallback string) string {
	severity = strings.ToLower(strings.TrimSpace(severity))
	if severity == "" {
		return fallback
	}
	return severity
}

func messageOrDefault(message string, fallback string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return fallback
	}
	return message
}

func voltageViolationKey(ref string, pin string, net string) string {
	return ref + "\x00" + pin + "\x00" + net
}

func sortAppliedRequirements(reqs []AppliedRequirement) {
	sort.SliceStable(reqs, func(i, j int) bool {
		if reqs[i].ComponentRef != reqs[j].ComponentRef {
			return reqs[i].ComponentRef < reqs[j].ComponentRef
		}
		if reqs[i].Type != reqs[j].Type {
			return reqs[i].Type < reqs[j].Type
		}
		return strings.Join(reqs[i].Scope.Pins, ",") < strings.Join(reqs[j].Scope.Pins, ",")
	})
}

func sortContractFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		if findings[i].ComponentRef != findings[j].ComponentRef {
			return findings[i].ComponentRef < findings[j].ComponentRef
		}
		if findings[i].Net != findings[j].Net {
			return findings[i].Net < findings[j].Net
		}
		if findings[i].Pin != findings[j].Pin {
			return findings[i].Pin < findings[j].Pin
		}
		return findings[i].Message < findings[j].Message
	})
}

func greaterThanVoltage(left float64, right float64) bool {
	return left-right > 1e-9 && math.Abs(left-right) >= 1e-9
}

func lessThanVoltage(left float64, right float64) bool {
	return right-left > 1e-9 && math.Abs(left-right) >= 1e-9
}

func greaterThanCurrent(left float64, right float64) bool {
	return left-right > 1e-9 && math.Abs(left-right) >= 1e-9
}
