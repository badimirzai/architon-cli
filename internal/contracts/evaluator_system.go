package contracts

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

type netConnection struct {
	Net  string
	Ref  string
	Pin  string
	Name string
}

type i2cSignalNet struct {
	Net     ir.Net
	Role    string
	Key     string
	BusID   string
	BusNets *I2CBusNets
}

type i2cBus struct {
	ID   string
	Type string
	Nets *I2CBusNets
	Refs []string
}

type pullupCandidate struct {
	Ref       string
	Ohms      float64
	RailNet   string
	SignalNet string
}

// evaluateCommonGround checks that scoped components share at least one ground net.
func evaluateCommonGround(design *ir.DesignIR, _ *ContractIR, req AppliedRequirement) []Finding {
	components := scopedComponentRefs(design, req.Scope)
	if len(components) == 0 {
		return nil
	}
	groundByRef := componentGroundNets(design)
	var shared map[string]struct{}
	for _, ref := range components {
		grounds := groundByRef[ref]
		if len(grounds) == 0 {
			finding := findingForRequirement(req, fmt.Sprintf("%s has no ground connection in scope", ref))
			finding.ComponentRef = ref
			return []Finding{finding}
		}
		if shared == nil {
			// Start with the first component's ground nets, then intersect with
			// every other component in scope.
			shared = stringSet(grounds)
			continue
		}
		for ground := range shared {
			if !stringSliceContains(grounds, ground) {
				delete(shared, ground)
			}
		}
	}
	if len(components) > 1 && len(shared) == 0 {
		finding := findingForRequirement(req, fmt.Sprintf("Scoped components do not share a common ground net: %s", strings.Join(components, ", ")))
		finding.ComponentRef = components[0]
		return []Finding{finding}
	}
	return nil
}

// evaluatePullupOhms checks I2C signal pull-up presence and effective resistance.
func evaluatePullupOhms(design *ir.DesignIR, contractIR *ContractIR, req AppliedRequirement) []Finding {
	signalNets := scopedI2CSignalNets(design, req.Scope)
	if len(signalNets) == 0 && strings.TrimSpace(req.Scope.Net) != "" {
		// A policy may target one named net instead of a bus. In that case the
		// caller has already declared intent, so no I2C-name inference is needed.
		if net, ok := findNet(design, req.Scope.Net); ok {
			signalNets = []i2cSignalNet{{Net: net, Role: "signal", Key: "scope", BusID: req.Scope.BusID, BusNets: cloneI2CBusNets(req.Scope.Nets)}}
		}
	}
	findings := make([]Finding, 0)
	for _, signalNet := range signalNets {
		pullups := pullupsForSignalNet(design, contractIR, signalNet.Net.Name, req.Scope)
		if len(pullups) == 0 {
			invalidPullups := invalidPullupCandidatesForSignalNet(design, contractIR, signalNet.Net.Name, req.Scope)
			finding := findingForRequirement(req, pullupMissingMessage(signalNet.Net.Name, invalidPullups))
			finding.Net = signalNet.Net.Name
			attachI2CBusToFinding(&finding, signalNet.BusID, signalNet.BusNets)
			attachPullupBoundsToFinding(&finding, req)
			if len(invalidPullups) > 0 {
				finding.ComponentRef = invalidPullups[0].Ref
				finding.PullupResistors = pullupRefs(invalidPullups)
			}
			finding.WhyThisMatters = pullupMissingWhyThisMatters(invalidPullups)
			findings = append(findings, finding)
			continue
		}
		effective, ok := effectivePullupOhms(pullups)
		if !ok {
			continue
		}
		if req.MinOhms != nil && effective < *req.MinOhms {
			finding := findingForRequirement(req, fmt.Sprintf("Observed: effective pull-up on %s is %s. Expected: %s.", signalNet.Net.Name, formatPullupOhms(effective), pullupExpectedRange(req)))
			finding.Net = signalNet.Net.Name
			finding.ComponentRef = pullups[0].Ref
			finding.WhyThisMatters = "Too-low pull-up resistance increases sink current when devices pull the line low. This can exceed device limits and distort bus behavior."
			attachI2CBusToFinding(&finding, signalNet.BusID, signalNet.BusNets)
			attachPullupDetailsToFinding(&finding, req, effective, pullups)
			findings = append(findings, finding)
			continue
		}
		if req.MaxOhms != nil && effective > *req.MaxOhms {
			finding := findingForRequirement(req, fmt.Sprintf("Observed: effective pull-up on %s is %s. Expected: %s.", signalNet.Net.Name, formatPullupOhms(effective), pullupExpectedRange(req)))
			finding.Net = signalNet.Net.Name
			finding.ComponentRef = pullups[0].Ref
			finding.WhyThisMatters = "Too-high pull-up resistance slows rising edges. At higher bus speeds or larger bus capacitance, devices may read invalid logic levels."
			attachI2CBusToFinding(&finding, signalNet.BusID, signalNet.BusNets)
			attachPullupDetailsToFinding(&finding, req, effective, pullups)
			findings = append(findings, finding)
		}
	}
	return findings
}

func pullupMissingMessage(netName string, invalidPullups []pullupCandidate) string {
	if len(invalidPullups) > 0 {
		pullup := invalidPullups[0]
		return fmt.Sprintf("Observed: %s = %s connects %s to %s. Expected: pull-up resistor between 2.2k and 10k to a compatible positive rail.", pullup.Ref, formatPullupOhms(pullup.Ohms), netName, pullup.RailNet)
	}
	return fmt.Sprintf("Observed: no pull-up resistor found on net %s. Expected: pull-up resistor between 2.2k and 10k to a compatible positive rail.", netName)
}

func pullupMissingWhyThisMatters(invalidPullups []pullupCandidate) string {
	for _, pullup := range invalidPullups {
		if isGroundNetName(pullup.RailNet) {
			return "I2C lines are open-drain and must idle high. A resistor to GND holds the bus low, which can prevent communication entirely."
		}
	}
	return "I2C lines are open-drain and must idle high. Without pull-ups, SDA/SCL may never reach a valid HIGH level, so devices may not communicate."
}

func pullupExpectedRange(req AppliedRequirement) string {
	if req.MinOhms != nil && req.MaxOhms != nil {
		return fmt.Sprintf("%s to %s", formatPullupOhms(*req.MinOhms), formatPullupOhms(*req.MaxOhms))
	}
	if req.MinOhms != nil {
		return fmt.Sprintf("at least %s", formatPullupOhms(*req.MinOhms))
	}
	if req.MaxOhms != nil {
		return fmt.Sprintf("at most %s", formatPullupOhms(*req.MaxOhms))
	}
	return "a compatible pull-up resistance"
}

func formatPullupOhms(ohms float64) string {
	if math.Abs(ohms) >= 1000 && math.Abs(math.Mod(ohms, 100)) < 1e-9 {
		kOhms := ohms / 1000
		if math.Abs(kOhms-math.Round(kOhms)) < 1e-9 {
			return fmt.Sprintf("%.0fk", kOhms)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1fk", kOhms), "0"), ".")
	}
	if math.Abs(ohms-math.Round(ohms)) < 1e-9 {
		return fmt.Sprintf("%.0f ohms", ohms)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f ohms", ohms), "0"), ".")
}

// evaluateVoltageCompatible checks scoped nets against explicit voltage limits.
func evaluateVoltageCompatible(design *ir.DesignIR, contractIR *ContractIR, req AppliedRequirement) []Finding {
	nets := scopedVoltageNets(design, req.Scope)
	findings := make([]Finding, 0)
	for _, net := range nets {
		// For signal nets, the effective voltage can come from a direct net
		// contract or from a pull-up rail attached to the signal.
		voltage, ok := scopedNetVoltage(design, contractIR, net.Name, req.Scope)
		if !ok {
			continue
		}
		for _, pin := range sortedPinRefs(net.Pins) {
			if passiveRef(pin.Ref) || !componentMatchesScope(partByRef(design, pin.Ref), req.Scope) {
				continue
			}
			minV, hasMin, maxV, hasMax := pinVoltageLimits(design, contractIR, pin.Ref, pin.Pin, pin.Name)
			if hasMax && voltage > maxV+1e-9 {
				finding := findingForRequirement(req, fmt.Sprintf("%s pin %s on net %s sees %.2fV above compatible maximum %.2fV", pin.Ref, pin.Pin, net.Name, voltage, maxV))
				finding.ComponentRef = pin.Ref
				finding.Net = net.Name
				finding.Pin = pin.Pin
				findings = append(findings, finding)
				continue
			}
			if hasMin && voltage+1e-9 < minV {
				finding := findingForRequirement(req, fmt.Sprintf("%s pin %s on net %s sees %.2fV below compatible minimum %.2fV", pin.Ref, pin.Pin, net.Name, voltage, minV))
				finding.ComponentRef = pin.Ref
				finding.Net = net.Name
				finding.Pin = pin.Pin
				findings = append(findings, finding)
			}
		}
	}
	return findings
}

// evaluateCurrentBudget checks rail load utilization against a percentage limit.
func evaluateCurrentBudget(design *ir.DesignIR, contractIR *ContractIR, parts map[string]ir.Part, req AppliedRequirement) []Finding {
	if req.MaxUtilizationPct == nil {
		return nil
	}
	nets := scopedRailNets(design, contractIR, req.Scope)
	findings := make([]Finding, 0)
	for _, net := range nets {
		// Current budget only runs when both sides are explicit: a source
		// capacity and at least one load current.
		capacity, ok := railCurrentCapacity(design, contractIR, parts, net)
		if !ok || capacity <= 0 {
			continue
		}
		load, hasLoad := railLoadCurrent(contractIR, parts, net)
		if !hasLoad {
			continue
		}
		utilization := load / capacity * 100
		if utilization <= *req.MaxUtilizationPct+1e-9 {
			continue
		}
		finding := findingForRequirement(req, fmt.Sprintf("Rail %s current budget is %.1f%% utilized (%.2fA load / %.2fA capacity), above maximum %.1f%%", net.Name, utilization, load, capacity, *req.MaxUtilizationPct))
		finding.Net = net.Name
		findings = append(findings, finding)
	}
	return findings
}

// evaluatePowerBudget compares declared consumer current with a declared source limit.
// Load is the sum of consumer current_a. Remaining margin percent is
// (max_current_a - load) / max_current_a * 100. Currents come only from the contract.
// A missing source or consumer ref is interface_component_missing. Nets are not checked.
func evaluatePowerBudget(design *ir.DesignIR, req AppliedRequirement) []Finding {
	if design == nil || req.PowerSource == nil || req.MinimumMarginPct == nil || req.PowerSource.MaxCurrentA <= 0 {
		return nil
	}
	contractID := strings.TrimSpace(req.ContractID)
	if contractID == "" {
		contractID = strings.TrimSpace(req.Scope.BusID)
	}
	parts := partIndex(design)
	refs := make([]string, 0, 1+len(req.PowerConsumers))
	refs = append(refs, req.PowerSource.Ref)
	for _, consumer := range req.PowerConsumers {
		refs = append(refs, consumer.Ref)
	}
	findings := make([]Finding, 0)
	seen := map[string]struct{}{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		if _, ok := parts[ref]; ok {
			continue
		}
		findings = append(findings, connectedFinding(
			req,
			RuleInterfaceComponentMissing,
			ref,
			"",
			fmt.Sprintf("Contract %s requires component %s. Observed: component %s is missing.", contractID, ref, ref),
			"Add the missing component to the schematic.",
			evidenceText("component "+ref),
			evidenceText("missing"),
		))
	}
	if len(findings) > 0 {
		return findings
	}

	load := 0.0
	for _, consumer := range req.PowerConsumers {
		load += consumer.CurrentA
	}
	maxA := req.PowerSource.MaxCurrentA
	lead := powerBudgetLead(contractID, req.Scope.Rail)
	sourceRef := strings.TrimSpace(req.PowerSource.Ref)
	if greaterThanCurrent(load, maxA) {
		return []Finding{connectedFinding(
			req,
			RulePowerBudgetExceeded,
			sourceRef,
			"",
			fmt.Sprintf("%s requires source %s at or below %sA. Observed: load is %sA.", lead, sourceRef, formatContractNumber(maxA), formatContractNumber(load)),
			"Reduce the declared consumer currents or raise the source max current.",
			evidenceMax(maxA, "A"),
			evidenceMax(load, "A"),
		)}
	}
	margin := (maxA - load) / maxA * 100
	// Exactly on the margin passes. A difference within 1e-9 is the same value.
	if *req.MinimumMarginPct-margin > 1e-9 {
		return []Finding{connectedFinding(
			req,
			RulePowerMarginLow,
			sourceRef,
			"",
			fmt.Sprintf("%s requires at least %s percent remaining margin. Observed: remaining margin is %s percent.", lead, formatContractNumber(*req.MinimumMarginPct), formatContractNumber(margin)),
			"Reduce the declared consumer currents or lower the minimum remaining margin.",
			evidenceMin(*req.MinimumMarginPct, "percent"),
			evidenceMin(margin, "percent"),
		)}
	}
	return nil
}

// powerBudgetLead names the contract, and the declared rail when the contract set one.
// The rail is a label. This check does not look up that net.
func powerBudgetLead(contractID string, rail string) string {
	rail = strings.TrimSpace(rail)
	if rail == "" {
		return "Contract " + contractID
	}
	return "Contract " + contractID + " for " + rail
}

// formatContractNumber prints a contract current or percent for a finding message.
func formatContractNumber(value float64) string {
	return strconv.FormatFloat(value, 'g', 6, 64)
}

// evaluateNoI2CAddressConflict checks duplicate device addresses per I2C bus.
func evaluateNoI2CAddressConflict(design *ir.DesignIR, req AppliedRequirement) []Finding {
	buses := scopedI2CBuses(design, req.Scope)
	parts := partIndex(design)
	findings := make([]Finding, 0)
	for _, bus := range buses {
		// Address conflicts are scoped per explicit or inferred bus, not
		// globally across the whole design.
		byAddress := map[uint64][]string{}
		for _, ref := range bus.Refs {
			if passiveRef(ref) || !componentMatchesScope(parts[ref], req.Scope) {
				continue
			}
			address, ok := i2cAddress(parts[ref])
			if !ok || address == 0 {
				continue
			}
			byAddress[address] = append(byAddress[address], ref)
		}
		addresses := make([]uint64, 0, len(byAddress))
		for address := range byAddress {
			addresses = append(addresses, address)
		}
		sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
		for _, address := range addresses {
			refs := byAddress[address]
			sort.Strings(refs)
			if len(refs) < 2 {
				continue
			}
			message := fmt.Sprintf("I2C devices %s share address %s", strings.Join(refs, ", "), formatI2CAddress(address))
			if busName := i2cBusDisplayName(bus); busName != "" {
				message += fmt.Sprintf(" on bus %s", busName)
			}
			finding := findingForRequirement(req, message)
			finding.ComponentRef = refs[0]
			attachI2CBusToFinding(&finding, bus.ID, bus.Nets)
			findings = append(findings, finding)
		}
	}
	return findings
}

// scopedComponentRefs returns component refs selected by a contract scope.
func scopedComponentRefs(design *ir.DesignIR, scope ContractScope) []string {
	if design == nil {
		return nil
	}
	refs := map[string]struct{}{}
	if strings.TrimSpace(scope.ComponentRef) != "" {
		if partExists(design, scope.ComponentRef) {
			refs[scope.ComponentRef] = struct{}{}
		}
	} else if strings.EqualFold(strings.TrimSpace(scope.BusType), "i2c") {
		for _, bus := range scopedI2CBuses(design, scope) {
			for _, ref := range bus.Refs {
				if !passiveRef(ref) {
					refs[ref] = struct{}{}
				}
			}
		}
	} else if strings.TrimSpace(scope.Net) != "" || strings.TrimSpace(scope.Rail) != "" {
		netName := firstNonEmpty(scope.Net, scope.Rail)
		if net, ok := findNet(design, netName); ok {
			for _, pin := range net.Pins {
				if !passiveRef(pin.Ref) {
					refs[pin.Ref] = struct{}{}
				}
			}
		}
	} else {
		for _, part := range design.Parts {
			if !passiveRef(part.Ref) && componentMatchesScope(part, scope) {
				refs[part.Ref] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(refs))
	parts := partIndex(design)
	for ref := range refs {
		if componentMatchesScope(parts[ref], scope) {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

// scopedI2CSignalNets returns SDA/SCL-like nets selected by scope.
func scopedI2CSignalNets(design *ir.DesignIR, scope ContractScope) []i2cSignalNet {
	if design == nil {
		return nil
	}
	if hasExplicitI2CNets(scope) {
		out := make([]i2cSignalNet, 0, 2)
		key := explicitI2CBusKey(scope)
		if net, ok := findExplicitScopedNet(design, scope.Nets.SDA); ok {
			if scope.Net == "" || net.Name == scope.Net {
				out = append(out, i2cSignalNet{Net: net, Role: "sda", Key: key, BusID: scope.BusID, BusNets: cloneI2CBusNets(scope.Nets)})
			}
		}
		if net, ok := findExplicitScopedNet(design, scope.Nets.SCL); ok {
			if scope.Net == "" || net.Name == scope.Net {
				out = append(out, i2cSignalNet{Net: net, Role: "scl", Key: key, BusID: scope.BusID, BusNets: cloneI2CBusNets(scope.Nets)})
			}
		}
		return out
	}
	out := make([]i2cSignalNet, 0)
	for _, net := range sortedIRNets(design.Nets) {
		if scope.Net != "" && net.Name != scope.Net {
			continue
		}
		role := i2cNetRole(net)
		if role == "" {
			continue
		}
		out = append(out, i2cSignalNet{Net: net, Role: role, Key: i2cBusKey(net.Name, role), BusID: scope.BusID})
	}
	return out
}

// scopedI2CBuses groups I2C components by explicit or inferred bus key.
func scopedI2CBuses(design *ir.DesignIR, scope ContractScope) []i2cBus {
	signals := scopedI2CSignalNets(design, scope)
	type busBuilder struct {
		id   string
		nets *I2CBusNets
		refs map[string]struct{}
	}
	byKey := map[string]*busBuilder{}
	for _, signal := range signals {
		if byKey[signal.Key] == nil {
			byKey[signal.Key] = &busBuilder{
				id:   signal.BusID,
				nets: cloneI2CBusNets(signal.BusNets),
				refs: map[string]struct{}{},
			}
		}
		for _, pin := range signal.Net.Pins {
			if passiveRef(pin.Ref) {
				continue
			}
			byKey[signal.Key].refs[pin.Ref] = struct{}{}
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]i2cBus, 0, len(keys))
	for _, key := range keys {
		refs := make([]string, 0, len(byKey[key].refs))
		for ref := range byKey[key].refs {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		out = append(out, i2cBus{
			ID:   byKey[key].id,
			Type: "i2c",
			Nets: cloneI2CBusNets(byKey[key].nets),
			Refs: refs,
		})
	}
	return out
}

func hasExplicitI2CNets(scope ContractScope) bool {
	return scope.Nets != nil && strings.TrimSpace(scope.Nets.SDA) != "" && strings.TrimSpace(scope.Nets.SCL) != ""
}

func missingExplicitI2CNetFindings(design *ir.DesignIR, req AppliedRequirement) []Finding {
	if design == nil || !hasExplicitI2CNets(req.Scope) {
		return nil
	}
	out := make([]Finding, 0, 2)
	for _, netName := range []string{req.Scope.Nets.SDA, req.Scope.Nets.SCL} {
		if _, ok := findExplicitScopedNet(design, netName); ok {
			continue
		}
		contractID := strings.TrimSpace(req.ContractID)
		if contractID == "" {
			contractID = strings.TrimSpace(req.Provenance.SourceID)
		}
		if contractID == "" {
			contractID = string(req.Type)
		}
		message := fmt.Sprintf("Contract %s references missing net %s", contractID, netName)
		finding := findingForRequirement(req, message)
		finding.Message = message
		finding.Severity = "ERROR"
		if strings.TrimSpace(finding.ContractID) == "" {
			finding.ContractID = contractID
		}
		finding.Net = netName
		attachI2CBusToFinding(&finding, req.Scope.BusID, req.Scope.Nets)
		out = append(out, finding)
	}
	return out
}

func normalizeNetName(name string) string {
	return strings.TrimPrefix(name, "/")
}

func findExplicitScopedNet(design *ir.DesignIR, name string) (ir.Net, bool) {
	if net, ok := findNet(design, name); ok {
		return net, true
	}
	normalized := normalizeNetName(name)
	for _, net := range sortedIRNets(design.Nets) {
		if normalizeNetName(net.Name) == normalized {
			return net, true
		}
	}
	return ir.Net{}, false
}

func explicitI2CBusKey(scope ContractScope) string {
	if strings.TrimSpace(scope.BusID) != "" {
		return "explicit:id:" + strings.TrimSpace(scope.BusID)
	}
	if !hasExplicitI2CNets(scope) {
		return ""
	}
	return "explicit:nets:" + scope.Nets.SDA + "\x00" + scope.Nets.SCL
}

func i2cBusDisplayName(bus i2cBus) string {
	if strings.TrimSpace(bus.ID) != "" {
		return strings.TrimSpace(bus.ID)
	}
	return ""
}

func attachI2CBusToFinding(finding *Finding, busID string, nets *I2CBusNets) {
	if finding == nil {
		return
	}
	if strings.TrimSpace(busID) != "" {
		finding.BusID = strings.TrimSpace(busID)
	}
	finding.BusType = "i2c"
	if nets != nil {
		finding.BusNets = cloneI2CBusNets(nets)
	}
}

func attachPullupBoundsToFinding(finding *Finding, req AppliedRequirement) {
	if finding == nil {
		return
	}
	finding.MinPullupOhms = cloneFloat(req.MinOhms)
	finding.MaxPullupOhms = cloneFloat(req.MaxOhms)
}

func attachPullupDetailsToFinding(finding *Finding, req AppliedRequirement, effective float64, pullups []pullupCandidate) {
	attachPullupBoundsToFinding(finding, req)
	finding.EffectivePullupOhms = cloneFloat(&effective)
	finding.PullupResistors = pullupRefs(pullups)
}

func pullupRefs(pullups []pullupCandidate) []string {
	refs := make([]string, 0, len(pullups))
	for _, pullup := range pullups {
		refs = append(refs, pullup.Ref)
	}
	sort.Strings(refs)
	return refs
}

// scopedVoltageNets returns nets that voltage compatibility should inspect.
func scopedVoltageNets(design *ir.DesignIR, scope ContractScope) []ir.Net {
	if design == nil {
		return nil
	}
	if strings.TrimSpace(scope.Net) != "" {
		if net, ok := findNet(design, scope.Net); ok {
			return []ir.Net{net}
		}
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(scope.BusType), "i2c") {
		signals := scopedI2CSignalNets(design, scope)
		out := make([]ir.Net, 0, len(signals))
		for _, signal := range signals {
			out = append(out, signal.Net)
		}
		return out
	}
	if strings.TrimSpace(scope.Rail) != "" {
		if net, ok := findNet(design, scope.Rail); ok {
			return []ir.Net{net}
		}
		return nil
	}
	return sortedIRNets(design.Nets)
}

// scopedRailNets returns rail-like nets selected by a current-budget scope.
func scopedRailNets(design *ir.DesignIR, contractIR *ContractIR, scope ContractScope) []ir.Net {
	if design == nil {
		return nil
	}
	if strings.TrimSpace(scope.Rail) != "" || strings.TrimSpace(scope.Net) != "" {
		if net, ok := findNet(design, firstNonEmpty(scope.Rail, scope.Net)); ok {
			return []ir.Net{net}
		}
		return nil
	}
	out := make([]ir.Net, 0)
	for _, net := range sortedIRNets(design.Nets) {
		if isGroundNetName(net.Name) {
			continue
		}
		if contractIR != nil {
			if netContract, ok := contractIR.Net(net.Name); ok && netContract.VoltageNominal != nil {
				out = append(out, net)
				continue
			}
		}
		if looksLikeRailNet(net.Name) {
			out = append(out, net)
		}
	}
	return out
}

// componentGroundNets maps each component to ground nets it touches.
func componentGroundNets(design *ir.DesignIR) map[string][]string {
	out := map[string][]string{}
	if design == nil {
		return out
	}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if isGroundNetName(net.Name) || isGroundPinName(pin.Pin) || isGroundPinName(pin.Name) {
				out[pin.Ref] = appendUniqueString(out[pin.Ref], net.Name)
			}
		}
	}
	for ref := range out {
		sort.Strings(out[ref])
	}
	return out
}

// pullupsForSignalNet finds resistors tying one signal net to a rail.
func pullupsForSignalNet(design *ir.DesignIR, contractIR *ContractIR, signalNet string, scope ContractScope) []pullupCandidate {
	connected := componentConnections(design)
	parts := partIndex(design)
	out := make([]pullupCandidate, 0)
	for ref, nets := range connected {
		part := parts[ref]
		if !looksLikeResistor(part) {
			continue
		}
		if len(nets) < 2 || !componentConnectedToNet(nets, signalNet) {
			continue
		}
		if componentConnectedToGround(nets) {
			continue
		}
		ohms := pullupOhms(part)
		if ohms <= 0 {
			continue
		}
		for _, conn := range nets {
			// A pull-up resistor must bridge the signal net to a rail, never to
			// ground or another signal.
			if conn.Net == signalNet || isGroundNetName(conn.Net) {
				continue
			}
			if scope.Rail != "" && conn.Net != scope.Rail {
				continue
			}
			if scope.Net != "" && signalNet != scope.Net {
				continue
			}
			if isPositiveSupplyNet(contractIR, conn.Net) {
				out = append(out, pullupCandidate{Ref: ref, Ohms: ohms, RailNet: conn.Net, SignalNet: signalNet})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

// invalidPullupCandidatesForSignalNet finds resistor-like parts that touch an
// I2C signal but do not qualify as pull-ups. This keeps scan provenance on
// pulldowns and SDA-to-SCL resistors without changing the finding itself.
func invalidPullupCandidatesForSignalNet(design *ir.DesignIR, contractIR *ContractIR, signalNet string, scope ContractScope) []pullupCandidate {
	connected := componentConnections(design)
	parts := partIndex(design)
	out := make([]pullupCandidate, 0)
	for ref, nets := range connected {
		part := parts[ref]
		if !looksLikeResistor(part) {
			continue
		}
		if len(nets) < 2 || !componentConnectedToNet(nets, signalNet) {
			continue
		}
		ohms := pullupOhms(part)
		if ohms <= 0 {
			continue
		}
		for _, conn := range nets {
			if conn.Net == signalNet {
				continue
			}
			if isValidPullupTarget(contractIR, conn.Net, scope) {
				continue
			}
			out = append(out, pullupCandidate{Ref: ref, Ohms: ohms, RailNet: conn.Net, SignalNet: signalNet})
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

func isValidPullupTarget(contractIR *ContractIR, netName string, scope ContractScope) bool {
	if isGroundNetName(netName) {
		return false
	}
	if scope.Rail != "" && netName != scope.Rail {
		return false
	}
	return isPositiveSupplyNet(contractIR, netName)
}

// effectivePullupOhms combines parallel pull-up resistors.
func effectivePullupOhms(pullups []pullupCandidate) (float64, bool) {
	if len(pullups) == 0 {
		return 0, false
	}
	inverse := 0.0
	for _, pullup := range pullups {
		if pullup.Ohms <= 0 {
			continue
		}
		inverse += 1 / pullup.Ohms
	}
	if inverse <= 0 {
		return 0, false
	}
	return 1 / inverse, true
}

// scopedNetVoltage finds direct or pull-up-derived voltage for a scoped net.
func scopedNetVoltage(design *ir.DesignIR, contractIR *ContractIR, netName string, scope ContractScope) (float64, bool) {
	if contractIR != nil {
		if netContract, ok := contractIR.Net(netName); ok && netContract.VoltageNominal != nil {
			return *netContract.VoltageNominal, true
		}
	}
	pullups := pullupsForSignalNet(design, contractIR, netName, scope)
	for _, pullup := range pullups {
		if contractIR == nil {
			continue
		}
		if netContract, ok := contractIR.Net(pullup.RailNet); ok && netContract.VoltageNominal != nil {
			return *netContract.VoltageNominal, true
		}
	}
	return 0, false
}

// pinVoltageLimits reads voltage limits from contracts or explicit part fields.
func pinVoltageLimits(design *ir.DesignIR, contractIR *ContractIR, ref string, pin string, pinName string) (float64, bool, float64, bool) {
	minV := 0.0
	maxV := 0.0
	hasMin := false
	hasMax := false
	if contractIR != nil {
		// Prefer already-enriched pin contracts, then fall back to explicit BOM
		// fields on the component.
		if pinContract, ok := contractIR.Pin(ref, pin); ok {
			if pinContract.VoltageMin != nil {
				minV = *pinContract.VoltageMin
				hasMin = true
			}
			if pinContract.VoltageMax != nil {
				maxV = *pinContract.VoltageMax
				hasMax = true
			}
		}
		for _, applied := range contractIR.AppliedRequirements {
			if applied.ComponentRef != ref {
				continue
			}
			if !pinMatchesAny(applied.Scope.Pins, pin, pinName) {
				continue
			}
			if applied.MinVoltage != nil && !hasMin {
				minV = *applied.MinVoltage
				hasMin = true
			}
			if applied.MaxVoltage != nil && !hasMax {
				maxV = *applied.MaxVoltage
				hasMax = true
			}
		}
	}
	part := partByRef(design, ref)
	fields := normalizedFields(part.Fields)
	if !hasMin {
		if parsed, ok := firstFieldVoltage(fields, []string{
			"architon_voltage_min_v",
			"voltage_min_v",
			"logic_voltage_min_v",
			"architon_logic_voltage_min_v",
		}); ok {
			minV = parsed
			hasMin = true
		}
	}
	if !hasMax {
		if parsed, ok := firstFieldVoltage(fields, []string{
			"architon_voltage_max_v",
			"voltage_max_v",
			"max_voltage_v",
			"max_voltage",
			"architon_logic_voltage_max_v",
			"logic_voltage_max_v",
			"architon_gpio_abs_max_v",
			"gpio_abs_max_v",
			"io_abs_max_v",
		}); ok {
			maxV = parsed
			hasMax = true
		}
	}
	return minV, hasMin, maxV, hasMax
}

// railCurrentCapacity totals explicit source capacity on a rail.
func railCurrentCapacity(design *ir.DesignIR, contractIR *ContractIR, parts map[string]ir.Part, net ir.Net) (float64, bool) {
	total := 0.0
	found := false
	seenRefs := map[string]struct{}{}
	for _, pin := range net.Pins {
		ref := pin.Ref
		if _, ok := seenRefs[ref]; ok {
			continue
		}
		seenRefs[ref] = struct{}{}
		if contractIR != nil {
			if pinContract, ok := contractIR.Pin(ref, pin.Pin); ok && isProviderPinRole(pinContract.Role) && pinContract.CurrentMax != nil {
				total += *pinContract.CurrentMax
				found = true
				continue
			}
			for _, req := range contractIR.AppliedRequirements {
				if req.ComponentRef != ref || req.Type != ContractRegulatorOutputCurrent || req.MaxCurrent == nil {
					continue
				}
				if !pinMatchesAny(req.Scope.Pins, pin.Pin, pin.Name) {
					continue
				}
				total += *req.MaxCurrent
				found = true
				break
			}
		}
		if capacity, ok := currentCapacityFromPart(parts[ref]); ok {
			total += capacity
			found = true
		}
	}
	_ = design
	return total, found
}

// railLoadCurrent totals explicit load current on a rail.
func railLoadCurrent(contractIR *ContractIR, parts map[string]ir.Part, net ir.Net) (float64, bool) {
	total := 0.0
	found := false
	seenRefs := map[string]struct{}{}
	for _, pin := range net.Pins {
		ref := pin.Ref
		if _, ok := seenRefs[ref]; ok {
			continue
		}
		seenRefs[ref] = struct{}{}
		if contractIR != nil {
			if pinContract, ok := contractIR.Pin(ref, pin.Pin); ok && isProviderPinRole(pinContract.Role) {
				continue
			}
			if current, ok := loadCurrentFromPinContract(contractIR, ref, pin.Pin); ok {
				total += current
				found = true
				continue
			}
		}
		if current, ok := loadCurrentFromPart(parts[ref]); ok {
			total += current
			found = true
		}
	}
	return total, found
}

// currentCapacityFromPart reads output-current capacity fields from a part.
func currentCapacityFromPart(part ir.Part) (float64, bool) {
	fields := normalizedFields(part.Fields)
	value, ok, err := fieldFloat(fields, []string{
		"architon_current_budget_a",
		"architon_output_current_a",
		"architon_regulator_output_current_a",
		"regulator_output_current_a",
		"output_current_a",
		"current_budget_a",
	})
	if err != nil || !ok {
		return 0, false
	}
	return value, true
}

// componentConnections indexes all net connections by component ref.
func componentConnections(design *ir.DesignIR) map[string][]netConnection {
	out := map[string][]netConnection{}
	if design == nil {
		return out
	}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			out[pin.Ref] = append(out[pin.Ref], netConnection{
				Net:  net.Name,
				Ref:  pin.Ref,
				Pin:  pin.Pin,
				Name: pin.Name,
			})
		}
	}
	for ref := range out {
		sort.Slice(out[ref], func(i, j int) bool {
			if out[ref][i].Net != out[ref][j].Net {
				return out[ref][i].Net < out[ref][j].Net
			}
			return out[ref][i].Pin < out[ref][j].Pin
		})
	}
	return out
}

// componentConnectedToNet checks whether a component touches a named net.
func componentConnectedToNet(connections []netConnection, netName string) bool {
	for _, conn := range connections {
		if conn.Net == netName {
			return true
		}
	}
	return false
}

func componentConnectedToGround(connections []netConnection) bool {
	for _, conn := range connections {
		if isGroundNetName(conn.Net) {
			return true
		}
	}
	return false
}

// partByRef finds a DesignIR part by reference.
func partByRef(design *ir.DesignIR, ref string) ir.Part {
	if design == nil {
		return ir.Part{}
	}
	for _, part := range design.Parts {
		if part.Ref == ref {
			return part
		}
	}
	return ir.Part{Ref: ref}
}

// partExists reports whether a component ref appears in parts or nets.
func partExists(design *ir.DesignIR, ref string) bool {
	if design == nil {
		return false
	}
	for _, part := range design.Parts {
		if part.Ref == ref {
			return true
		}
	}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if pin.Ref == ref {
				return true
			}
		}
	}
	return false
}

// findNet finds a DesignIR net by exact name.
func findNet(design *ir.DesignIR, name string) (ir.Net, bool) {
	if design == nil {
		return ir.Net{}, false
	}
	for _, net := range design.Nets {
		if net.Name == name {
			return net, true
		}
	}
	return ir.Net{}, false
}

// componentMatchesScope checks component_ref and component_type scope filters.
func componentMatchesScope(part ir.Part, scope ContractScope) bool {
	if strings.TrimSpace(scope.ComponentRef) != "" && part.Ref != scope.ComponentRef {
		return false
	}
	componentType := strings.TrimSpace(scope.ComponentType)
	if componentType == "" {
		return true
	}
	return strings.EqualFold(componentTypeFromPart(part), componentType)
}

// componentTypeFromPart reads an explicit component type field.
func componentTypeFromPart(part ir.Part) string {
	fields := normalizedFields(part.Fields)
	for _, key := range []string{"architon_component_type", "component_type", "type", "category"} {
		if strings.TrimSpace(fields[key]) != "" {
			return strings.TrimSpace(fields[key])
		}
	}
	return ""
}

// i2cAddress reads an explicit I2C address field from a part.
func i2cAddress(part ir.Part) (uint64, bool) {
	fields := normalizedFields(part.Fields)
	for _, key := range []string{"architon_i2c_address", "i2c_address", "i2c_address_hex", "address_hex", "address"} {
		value := strings.TrimSpace(fields[key])
		if value == "" {
			continue
		}
		parsed, err := parseInteger(value)
		if err != nil {
			continue
		}
		return parsed, true
	}
	return 0, false
}

// parseInteger parses decimal, 0x-prefixed hex, or 68h-style hex strings.
func parseInteger(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" {
		return 0, fmt.Errorf("empty integer")
	}
	base := 10
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "0x") {
		base = 0
	} else if strings.HasSuffix(lower, "h") {
		base = 16
		value = value[:len(value)-1]
	}
	return strconv.ParseUint(value, base, 16)
}

func formatI2CAddress(address uint64) string {
	return fmt.Sprintf("0x%02x", address)
}

// i2cNetRole classifies a net as SDA, SCL, or neither.
func i2cNetRole(net ir.Net) string {
	role := roleFromI2CString(net.Name)
	if role != "" {
		return role
	}
	for _, pin := range net.Pins {
		role = roleFromI2CString(pin.Name)
		if role != "" {
			return role
		}
		role = roleFromI2CString(pin.Pin)
		if role != "" {
			return role
		}
	}
	return ""
}

// roleFromI2CString detects SDA/SCL in one name-like string.
func roleFromI2CString(value string) string {
	normalized := normalizeAlphaNum(value)
	switch {
	case strings.Contains(normalized, "SDA"):
		return "sda"
	case strings.Contains(normalized, "SCL"):
		return "scl"
	default:
		return ""
	}
}

// i2cBusKey removes the role suffix to group SDA/SCL nets into one bus.
func i2cBusKey(netName string, role string) string {
	key := normalizeAlphaNum(netName)
	key = strings.ReplaceAll(key, strings.ToUpper(role), "")
	key = strings.TrimSpace(key)
	if key == "" {
		return "i2c"
	}
	return key
}

// pullupOhms reads resistor value fields or value text as ohms.
func pullupOhms(part ir.Part) float64 {
	fields := normalizedFields(part.Fields)
	for _, key := range []string{
		"architon_pullup_ohms",
		"pullup_ohms",
		"resistance_ohms",
		"resistor_ohms",
		"ohms",
	} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			if parsed, err := parseResistanceOhms(value); err == nil {
				return parsed
			}
		}
	}
	if parsed, err := parseResistanceOhms(part.Value); err == nil {
		return parsed
	}
	return 0
}

// parseResistanceOhms parses resistor values like 4700, 4.7k, or 4k7.
func parseResistanceOhms(value string) (float64, error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "Ω", "ohm")
	value = strings.ReplaceAll(value, "ω", "ohm")
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "ohms", "")
	value = strings.ReplaceAll(value, "ohm", "")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, ",", "")
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty resistance")
	}
	if strings.Contains(value, "k") && !strings.HasSuffix(value, "k") {
		parts := strings.SplitN(value, "k", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			value = parts[0] + "." + parts[1]
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return 0, err
			}
			return parsed * 1000, nil
		}
	}
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "k"):
		multiplier = 1000
		value = strings.TrimSuffix(value, "k")
	case strings.HasSuffix(value, "m"):
		multiplier = 1000000
		value = strings.TrimSuffix(value, "m")
	case strings.HasSuffix(value, "r"):
		value = strings.TrimSuffix(value, "r")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return 0, fmt.Errorf("invalid resistance")
	}
	return parsed * multiplier, nil
}

// looksLikeResistor checks explicit type fields or R* refs.
func looksLikeResistor(part ir.Part) bool {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(part.Ref)), "R") {
		return true
	}
	fields := normalizedFields(part.Fields)
	for _, key := range []string{"architon_component_type", "component_type", "type", "category"} {
		value := strings.ToLower(strings.TrimSpace(fields[key]))
		if value == "resistor" || value == "res" {
			return true
		}
	}
	for _, key := range []string{
		"architon_pullup_ohms",
		"pullup_ohms",
		"resistance_ohms",
		"resistor_ohms",
		"ohms",
	} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			if _, err := parseResistanceOhms(value); err == nil {
				return true
			}
		}
	}
	if _, err := parseResistanceOhms(part.Value); err == nil {
		return true
	}
	footprint := strings.ToLower(strings.TrimSpace(part.Footprint))
	if strings.Contains(footprint, "resistor") || strings.Contains(footprint, ":r_") || strings.HasPrefix(footprint, "r_") {
		return true
	}
	return false
}

// firstFieldVoltage reads the first parseable voltage field from a key list.
func firstFieldVoltage(fields map[string]string, keys []string) (float64, bool) {
	for _, key := range keys {
		value := strings.TrimSpace(fields[key])
		if value == "" {
			continue
		}
		parsed, err := parseEngineeringFloat(value)
		if err != nil {
			continue
		}
		return parsed, true
	}
	return 0, false
}

// passiveRef filters common passive component refs out of device-level checks.
func passiveRef(ref string) bool {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	for _, prefix := range []string{"R", "C", "L", "FB", "F", "TP"} {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	return false
}

// isProviderPinRole reports whether a pin role can source rail current.
func isProviderPinRole(role PinRole) bool {
	switch role {
	case RolePowerOut, RoleRegulatorOut, RoleSource:
		return true
	default:
		return false
	}
}

// isGroundNetName reports whether a net name is ground-like.
func isGroundNetName(value string) bool {
	normalized := normalizeAlphaNum(value)
	if normalized == "0V" || normalized == "GND" || normalized == "GROUND" {
		return true
	}
	return strings.Contains(normalized, "GND")
}

// isGroundPinName reports whether a pin name is ground-like.
func isGroundPinName(value string) bool {
	normalized := normalizeAlphaNum(value)
	return normalized == "GND" || normalized == "GROUND" || strings.HasSuffix(normalized, "GND")
}

func isPositiveSupplyNet(contractIR *ContractIR, netName string) bool {
	if isGroundNetName(netName) {
		return false
	}
	if contractIR != nil {
		if netContract, ok := contractIR.Net(netName); ok && netContract.VoltageNominal != nil && *netContract.VoltageNominal > 0 {
			return true
		}
	}
	return looksLikeRailNet(netName)
}

// looksLikeRailNet reports whether a net name is rail-like.
func looksLikeRailNet(value string) bool {
	normalized := normalizeAlphaNum(value)
	if isGroundNetName(value) {
		return false
	}
	if strings.Contains(normalized, "V") {
		return true
	}
	for _, prefix := range []string{"VBAT", "VIN", "VCC", "VDD"} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// normalizeAlphaNum uppercases and strips separators for name matching.
func normalizeAlphaNum(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sortedIRNets returns a sorted copy of nets.
func sortedIRNets(nets []ir.Net) []ir.Net {
	out := append([]ir.Net(nil), nets...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sortedPinRefs returns a sorted copy of pin refs.
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

// stringSet converts a slice to a membership set.
func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

// appendUniqueString appends a string only if it is not already present.
func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// stringSliceContains checks membership in a string slice.
func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// firstNonEmpty returns the first non-blank string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// evaluateConnected checks that every named participant has a pin on every named net.
// Roles are contract labels. The check does not look up MCU peripherals.
// Order is fixed: missing component, missing net, then no pin on the net.
// An optional pins map is checked after connectivity, and only for participants already on the net.
// chip_selects is checked last. It does not repeat the shared-net test above.
func evaluateConnected(design *ir.DesignIR, req AppliedRequirement) []Finding {
	if design == nil {
		return nil
	}
	contractID := strings.TrimSpace(req.ContractID)
	if contractID == "" {
		contractID = strings.TrimSpace(req.Scope.BusID)
	}
	parts := partIndex(design)
	participants := append([]InterfaceParticipant(nil), req.Participants...)
	sort.Slice(participants, func(i, j int) bool { return participants[i].Ref < participants[j].Ref })
	nets := append([]string(nil), req.Nets...)
	sort.Strings(nets)

	findings := make([]Finding, 0)
	// Missing components are reported first and skipped in the pin check below.
	missingRefs := map[string]struct{}{}
	for _, participant := range participants {
		if _, ok := parts[participant.Ref]; ok {
			continue
		}
		missingRefs[participant.Ref] = struct{}{}
		findings = append(findings, connectedFinding(
			req,
			RuleInterfaceComponentMissing,
			participant.Ref,
			"",
			fmt.Sprintf("Contract %s requires component %s (%s). Observed: component %s is missing.", contractID, participant.Ref, participant.Role, participant.Ref),
			"Add the missing component to the schematic.",
			evidenceText("component "+participant.Ref),
			evidenceText("missing"),
		))
	}

	// A leading slash is ignored, so SPI_SCK matches /SPI_SCK.
	missingNets := map[string]struct{}{}
	resolved := map[string]ir.Net{}
	for _, netName := range nets {
		net, ok := findExplicitScopedNet(design, netName)
		if !ok {
			missingNets[netName] = struct{}{}
			findings = append(findings, connectedFinding(
				req,
				RuleInterfaceNetMissing,
				"",
				netName,
				fmt.Sprintf("Contract %s requires net %s. Observed: net %s is missing.", contractID, netName, netName),
				"Add the missing net or correct the interface contract net name.",
				evidenceText("net "+netName),
				evidenceText("missing"),
			))
			continue
		}
		resolved[netName] = net
	}

	// Only check pins when both the component and the net are present.
	// Saved so the chip-select check does not report the same missing pin again.
	alreadyUnconnected := map[string]struct{}{}
	for _, participant := range participants {
		if _, missing := missingRefs[participant.Ref]; missing {
			continue
		}
		for _, netName := range nets {
			if _, missing := missingNets[netName]; missing {
				continue
			}
			net := resolved[netName]
			if partTouchesNet(net, participant.Ref) {
				continue
			}
			observedNet := net.Name
			if strings.TrimSpace(observedNet) == "" {
				observedNet = netName
			}
			alreadyUnconnected[participant.Ref+"\x00"+normalizeNetName(netName)] = struct{}{}
			findings = append(findings, connectedFinding(
				req,
				RuleInterfaceNotConnected,
				participant.Ref,
				observedNet,
				fmt.Sprintf("Contract %s requires %s (%s) on net %s. Observed: %s has no pin on %s.", contractID, participant.Ref, participant.Role, observedNet, participant.Ref, observedNet),
				fmt.Sprintf("Connect %s to %s.", participant.Ref, observedNet),
				evidenceText(participant.Ref+" connected to "+observedNet),
				evidenceText(participant.Ref+" has no pin on "+observedNet),
			))
		}
	}
	// Pin name and pin number checks run after connectivity so a missing part or
	// net is not also reported as the wrong pin.
	findings = append(findings, evaluateConnectedPins(req, contractID, participants, nets, missingRefs, missingNets, resolved)...)
	findings = append(findings, evaluateChipSelects(design, req, contractID, participants, missingRefs, missingNets, alreadyUnconnected)...)
	return findings
}

// evaluateConnectedPins applies each participant's optional pins map.
// A participant with no pins map is connectivity-only. A missing component is
// already reported and is skipped here. The contract token is compared with the
// netlist pin name and the pin number. Alternate pin functions are not inferred.
func evaluateConnectedPins(req AppliedRequirement, contractID string, participants []InterfaceParticipant, nets []string, missingRefs map[string]struct{}, missingNets map[string]struct{}, resolved map[string]ir.Net) []Finding {
	findings := make([]Finding, 0)
	for _, participant := range participants {
		if len(participant.Pins) == 0 {
			continue
		}
		if _, missing := missingRefs[participant.Ref]; missing {
			continue
		}
		findings = append(findings, evaluateParticipantPins(req, contractID, participant, nets, missingNets, resolved)...)
	}
	return findings
}

// evaluateParticipantPins reports pin mismatch and pin conflict for one participant.
// tokenNets groups the signals that require the same pin token. Nets are already
// sorted, so that grouping stays stable. A token required by two signals is one
// physical pin bound twice in this contract. A later pass reports a pin that
// landed on a net but is the token for a different signal.
func evaluateParticipantPins(req AppliedRequirement, contractID string, participant InterfaceParticipant, nets []string, missingNets map[string]struct{}, resolved map[string]ir.Net) []Finding {
	tokenNets := map[string][]string{}
	for _, netName := range nets {
		token, ok := participant.Pins[netName]
		if !ok || strings.TrimSpace(token) == "" {
			continue
		}
		tokenNets[token] = append(tokenNets[token], netName)
	}

	findings := make([]Finding, 0)
	// reportedConflict records a pin token that already produced interface_pin_conflict,
	// so the same token is not reported again when it is also seen on one of those nets.
	reportedConflict := map[string]struct{}{}
	tokens := make([]string, 0, len(tokenNets))
	for token := range tokenNets {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	// The same token on two nets is interface_pin_conflict. expected.text is the
	// first signal. observed.text is the pin token. The finding net is the last signal.
	for _, token := range tokens {
		bound := tokenNets[token]
		if len(bound) < 2 {
			continue
		}
		reportedConflict[token] = struct{}{}
		findings = append(findings, connectedPinFinding(
			req,
			RuleInterfacePinConflict,
			participant.Ref,
			connectedDisplayNet(resolved, bound[len(bound)-1]),
			matchingPinNumber(resolved, participant.Ref, token, bound),
			fmt.Sprintf("Contract %s binds %s pin %s to %s. Observed: %s is bound to more than one signal.", contractID, participant.Ref, token, joinSignals(bound), token),
			fmt.Sprintf("Assign %s to only one signal in contract %s.", token, contractID),
			evidenceText(bound[0]),
			evidenceText(token),
		))
	}

	for _, netName := range nets {
		token, ok := participant.Pins[netName]
		if !ok || strings.TrimSpace(token) == "" {
			continue
		}
		if _, missing := missingNets[netName]; missing {
			continue
		}
		net := resolved[netName]
		pins := componentPinsOnNet(net, participant.Ref)
		if len(pins) == 0 {
			continue
		}
		displayNet := connectedDisplayNet(resolved, netName)
		// One matching pin is enough. If none match, the first pin in stable
		// order supplies observed.text: its name, or its number when the name is empty.
		if !pinsMatchContractToken(pins, token) {
			pin := pins[0]
			observed := pinObservedText(pin)
			findings = append(findings, connectedPinFinding(
				req,
				RuleInterfacePinMismatch,
				participant.Ref,
				displayNet,
				strings.TrimSpace(pin.Pin),
				fmt.Sprintf("Contract %s requires %s (%s) pin %s on net %s. Observed: %s pin %s.", contractID, participant.Ref, participant.Role, token, displayNet, participant.Ref, observed),
				fmt.Sprintf("Connect %s pin %s to %s.", participant.Ref, token, displayNet),
				evidenceText(token),
				evidenceText(observed),
			))
		}
		// A pin can conflict even when its name matches this net. Its number may
		// still be the token this contract bound to a different signal.
		for _, pin := range pins {
			for _, id := range pinIdentities(pin) {
				if _, already := reportedConflict[id]; already {
					continue
				}
				others := otherPinSignals(tokenNets, id, netName)
				if len(others) == 0 {
					continue
				}
				reportedConflict[id] = struct{}{}
				findings = append(findings, connectedPinFinding(
					req,
					RuleInterfacePinConflict,
					participant.Ref,
					displayNet,
					strings.TrimSpace(pin.Pin),
					fmt.Sprintf("Contract %s binds %s pin %s to %s. Observed: %s on %s.", contractID, participant.Ref, id, joinSignals(others), id, displayNet),
					fmt.Sprintf("Assign %s to only one signal in contract %s.", id, contractID),
					evidenceText(others[0]),
					evidenceText(id),
				))
			}
		}
	}
	return findings
}

// connectedPinFinding copies one interface finding and records the netlist pin number.
// expected and observed are set by the caller. The message is what human output prints.
func connectedPinFinding(req AppliedRequirement, ruleID string, ref string, net string, pin string, message string, fix string, expected *Evidence, observed *Evidence) Finding {
	finding := connectedFinding(req, ruleID, ref, net, message, fix, expected, observed)
	finding.Pin = strings.TrimSpace(pin)
	return finding
}

// connectedDisplayNet returns the net name from the design, including a leading slash.
// The contract key stays "SPI_SCK" when the schematic net is "/SPI_SCK".
func connectedDisplayNet(resolved map[string]ir.Net, netName string) string {
	net, ok := resolved[netName]
	if !ok {
		return netName
	}
	if name := strings.TrimSpace(net.Name); name != "" {
		return name
	}
	return netName
}

// componentPinsOnNet returns this component's pins on the net, sorted by pin number.
// An empty result means the component is not on the net, which is interface_not_connected.
func componentPinsOnNet(net ir.Net, ref string) []ir.PinRef {
	matched := make([]ir.PinRef, 0, 1)
	for _, pin := range net.Pins {
		if pin.Ref == ref {
			matched = append(matched, pin)
		}
	}
	return sortedPinRefs(matched)
}

// pinsMatchContractToken reports whether any of the component's pins on this net
// equals the contract token by name or by number.
func pinsMatchContractToken(pins []ir.PinRef, token string) bool {
	for _, pin := range pins {
		if pinMatchesContractToken(pin, token) {
			return true
		}
	}
	return false
}

// pinMatchesContractToken is an exact match against the netlist pin name or pin number.
// "PB13" matches Name. "42" matches Pin. Neither field is rewritten or aliased.
func pinMatchesContractToken(pin ir.PinRef, token string) bool {
	if token == "" {
		return false
	}
	if strings.TrimSpace(pin.Name) == token {
		return true
	}
	return strings.TrimSpace(pin.Pin) == token
}

// pinObservedText is the value stored in observed.text for a pin mismatch.
// The pin name is used when the netlist has one. Otherwise the pin number is used.
func pinObservedText(pin ir.PinRef) string {
	if name := strings.TrimSpace(pin.Name); name != "" {
		return name
	}
	return strings.TrimSpace(pin.Pin)
}

// pinIdentities lists the name and number that can collide with another signal.
// The same string is returned once when the name and number are identical.
func pinIdentities(pin ir.PinRef) []string {
	name := strings.TrimSpace(pin.Name)
	number := strings.TrimSpace(pin.Pin)
	out := make([]string, 0, 2)
	if name != "" {
		out = append(out, name)
	}
	if number != "" && number != name {
		out = append(out, number)
	}
	return out
}

// otherPinSignals returns the other nets in this contract that require token.
// An empty result means this pin is not bound to a different signal.
func otherPinSignals(tokenNets map[string][]string, token string, current string) []string {
	bound := tokenNets[token]
	if len(bound) == 0 {
		return nil
	}
	others := make([]string, 0, len(bound))
	for _, netName := range bound {
		if netName != current {
			others = append(others, netName)
		}
	}
	return others
}

// matchingPinNumber finds the netlist pin number for a token that matched by name or number.
// The finding's pin field keeps that number. observed.text still carries the contract token.
func matchingPinNumber(resolved map[string]ir.Net, ref string, token string, netNames []string) string {
	for _, netName := range netNames {
		net, ok := resolved[netName]
		if !ok {
			continue
		}
		for _, pin := range componentPinsOnNet(net, ref) {
			if !pinMatchesContractToken(pin, token) {
				continue
			}
			if number := strings.TrimSpace(pin.Pin); number != "" {
				return number
			}
		}
	}
	return ""
}

// joinSignals formats the signal names a single pin is bound to, in sorted order.
func joinSignals(nets []string) string {
	switch len(nets) {
	case 0:
		return ""
	case 1:
		return nets[0]
	case 2:
		return nets[0] + " and " + nets[1]
	default:
		return strings.Join(nets[:len(nets)-1], ", ") + ", and " + nets[len(nets)-1]
	}
}

// connectedFinding copies contract provenance onto one interface finding.
// ruleID is the specific failure, not the YAML key "connected".
func connectedFinding(req AppliedRequirement, ruleID string, ref string, net string, message string, fix string, expected *Evidence, observed *Evidence) Finding {
	finding := findingForRequirement(req, message)
	finding.RuleID = ruleID
	finding.Requirement = ruleID
	finding.ComponentRef = ref
	finding.Net = net
	finding.BusID = strings.TrimSpace(req.Scope.BusID)
	finding.BusType = strings.TrimSpace(req.Scope.BusType)
	finding.Fix = fix
	finding.Expected = expected
	finding.Observed = observed
	return finding
}

// partTouchesNet reports whether ref has any pin on net.
// Which pin it is does not matter here. Pin name and number are checked later.
func partTouchesNet(net ir.Net, ref string) bool {
	for _, pin := range net.Pins {
		if pin.Ref == ref {
			return true
		}
	}
	return false
}

// evidenceText builds a text-only expected or observed value. Empty text stays unset.
func evidenceText(text string) *Evidence {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return &Evidence{Text: text}
}

// evidenceMax stores one numeric comparison in max, with a unit. Text stays empty.
func evidenceMax(value float64, unit string) *Evidence {
	copied := value
	return &Evidence{Max: &copied, Unit: unit}
}

// evidenceMin stores one numeric comparison in min, with a unit. Text stays empty.
func evidenceMin(value float64, unit string) *Evidence {
	copied := value
	return &Evidence{Min: &copied, Unit: unit}
}

// evaluateTerminated counts two-pin parts of the required resistance between two nets.
// It does not check that other components share those nets, and it does not look up a part database.
// A missing net is interface_net_missing. The count is reported only when both nets exist.
func evaluateTerminated(design *ir.DesignIR, req AppliedRequirement) []Finding {
	if design == nil || req.TerminatorCount == nil || req.ResistanceOhms == nil || len(req.Nets) != 2 {
		return nil
	}
	contractID := strings.TrimSpace(req.ContractID)
	if contractID == "" {
		contractID = strings.TrimSpace(req.Scope.BusID)
	}
	findings := make([]Finding, 0)
	resolved := make(map[string]ir.Net, 2)
	for _, netName := range req.Nets {
		net, ok := findExplicitScopedNet(design, netName)
		if !ok {
			findings = append(findings, connectedFinding(
				req,
				RuleInterfaceNetMissing,
				"",
				netName,
				fmt.Sprintf("Contract %s requires net %s. Observed: net %s is missing.", contractID, netName, netName),
				"Add the missing net or correct the interface contract net name.",
				evidenceText("net "+netName),
				evidenceText("missing"),
			))
			continue
		}
		resolved[netName] = net
	}
	// Both nets have to exist before a count means anything.
	if len(findings) > 0 {
		return findings
	}

	target := *req.ResistanceOhms
	spanned := []ir.Net{resolved[req.Nets[0]], resolved[req.Nets[1]]}
	parts := partIndex(design)
	refs := make([]string, 0, len(parts))
	for ref := range parts {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	// A match is the right ohms and exactly one pin on each of the two nets.
	matched := 0
	for _, ref := range refs {
		if !partMatchesResistance(parts[ref], target) {
			continue
		}
		if !isTerminator(design, ref, spanned) {
			continue
		}
		matched++
	}

	expected := *req.TerminatorCount
	if matched == expected {
		return nil
	}
	// Too few and too many are separate findings. The texts are just the two counts.
	ruleID := RuleTerminationCountLow
	fix := "Add terminators of the required resistance between the named nets until the count matches the contract."
	if matched > expected {
		ruleID = RuleTerminationCountHigh
		fix = "Remove extra terminators between the named nets until the count matches the contract."
	}
	display := []string{
		connectedDisplayNet(resolved, req.Nets[0]),
		connectedDisplayNet(resolved, req.Nets[1]),
	}
	return []Finding{connectedFinding(
		req,
		ruleID,
		"",
		"",
		fmt.Sprintf("Contract %s requires %d terminators of %s ohms between %s. Observed: %d.", contractID, expected, formatOhmsNumber(target), joinSignals(display), matched),
		fix,
		evidenceText(strconv.Itoa(expected)),
		evidenceText(strconv.Itoa(matched)),
	)}
}

// isTerminator reports a two-pin part with one pin on each named net.
// A third pin, or both pins on one net, is not a terminator.
func isTerminator(design *ir.DesignIR, ref string, nets []ir.Net) bool {
	if partConnectionCount(design, ref) != 2 {
		return false
	}
	for _, net := range nets {
		if len(componentPinsOnNet(net, ref)) != 1 {
			return false
		}
	}
	return true
}

// partConnectionCount is the number of pin attachments for ref across every net.
func partConnectionCount(design *ir.DesignIR, ref string) int {
	count := 0
	if design == nil {
		return 0
	}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if pin.Ref == ref {
				count++
			}
		}
	}
	return count
}

// partMatchesResistance reports whether the part value or a resistance field equals ohms.
// Either match is enough. Pull-up aliases and component databases are not consulted.
func partMatchesResistance(part ir.Part, ohms float64) bool {
	if value, err := parseResistanceOhms(part.Value); err == nil && sameResistance(value, ohms) {
		return true
	}
	fields := normalizedFields(part.Fields)
	for _, key := range []string{"resistance", "resistance_ohms"} {
		raw := strings.TrimSpace(fields[key])
		if raw == "" {
			continue
		}
		value, err := parseResistanceOhms(raw)
		if err == nil && sameResistance(value, ohms) {
			return true
		}
	}
	return false
}

// sameResistance treats 120 and 120.0 as the same resistance.
func sameResistance(left, right float64) bool {
	return math.Abs(left-right) <= 1e-6
}

// formatOhmsNumber prints a whole number as "120", not "120.000".
func formatOhmsNumber(ohms float64) string {
	if math.Abs(ohms-math.Round(ohms)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(ohms)), 10)
	}
	return strconv.FormatFloat(ohms, 'f', -1, 64)
}

// evaluateChipSelects checks per-slave chip-select nets on an SPI connected contract.
// Shared bus nets are already checked by evaluateConnected. This does not repeat that test.
// A missing chip-select net is interface_net_missing. A slave with no pin on its net is
// interface_not_connected. Two entries naming one net, or two slaves on one chip-select net,
// is spi_cs_shared.
func evaluateChipSelects(design *ir.DesignIR, req AppliedRequirement, contractID string, participants []InterfaceParticipant, missingRefs map[string]struct{}, missingNets map[string]struct{}, alreadyUnconnected map[string]struct{}) []Finding {
	if design == nil || len(req.ChipSelects) == 0 {
		return nil
	}
	selects := append([]ChipSelect(nil), req.ChipSelects...)
	sort.Slice(selects, func(i, j int) bool {
		left := normalizeNetName(selects[i].Net)
		right := normalizeNetName(selects[j].Net)
		if left != right {
			return left < right
		}
		return selects[i].Ref < selects[j].Ref
	})

	findings := make([]Finding, 0)
	resolved := map[string]ir.Net{}
	reportedMissing := map[string]struct{}{}
	// Look up each chip-select net once. A missing net is reported once.
	for _, sel := range selects {
		norm := normalizeNetName(sel.Net)
		if _, ok := resolved[norm]; ok {
			continue
		}
		if _, ok := reportedMissing[norm]; ok {
			continue
		}
		if netAlreadyMissing(missingNets, sel.Net) {
			reportedMissing[norm] = struct{}{}
			continue
		}
		net, ok := findExplicitScopedNet(design, sel.Net)
		if !ok {
			reportedMissing[norm] = struct{}{}
			findings = append(findings, connectedFinding(
				req,
				RuleInterfaceNetMissing,
				"",
				sel.Net,
				fmt.Sprintf("Contract %s requires net %s. Observed: net %s is missing.", contractID, sel.Net, sel.Net),
				"Add the missing net or correct the interface contract net name.",
				evidenceText("net "+sel.Net),
				evidenceText("missing"),
			))
			continue
		}
		resolved[norm] = net
	}

	// Each slave needs a pin on its own chip-select net.
	for _, sel := range selects {
		norm := normalizeNetName(sel.Net)
		if _, missing := reportedMissing[norm]; missing {
			continue
		}
		if _, missing := missingRefs[sel.Ref]; missing {
			continue
		}
		if _, already := alreadyUnconnected[sel.Ref+"\x00"+norm]; already {
			continue
		}
		net := resolved[norm]
		if partTouchesNet(net, sel.Ref) {
			continue
		}
		display := chipSelectDisplayNet(resolved, sel.Net)
		role := participantRole(participants, sel.Ref)
		findings = append(findings, connectedFinding(
			req,
			RuleInterfaceNotConnected,
			sel.Ref,
			display,
			fmt.Sprintf("Contract %s requires %s (%s) on net %s. Observed: %s has no pin on %s.", contractID, sel.Ref, role, display, sel.Ref, display),
			fmt.Sprintf("Connect %s to %s.", sel.Ref, display),
			evidenceText(sel.Ref+" connected to "+display),
			evidenceText(sel.Ref+" has no pin on "+display),
		))
	}

	slaves := map[string]struct{}{}
	for _, participant := range participants {
		if participant.Role != "slave" {
			continue
		}
		if _, missing := missingRefs[participant.Ref]; missing {
			continue
		}
		slaves[participant.Ref] = struct{}{}
	}

	// Group entries that name the same net, ignoring a leading slash.
	groups := map[string][]ChipSelect{}
	order := make([]string, 0)
	for _, sel := range selects {
		norm := normalizeNetName(sel.Net)
		if _, ok := groups[norm]; !ok {
			order = append(order, norm)
		}
		groups[norm] = append(groups[norm], sel)
	}
	for _, norm := range order {
		entries := groups[norm]
		shared := map[string]struct{}{}
		// The contract itself can name one net for two slaves.
		if chipSelectRefCount(entries) >= 2 {
			for _, entry := range entries {
				shared[entry.Ref] = struct{}{}
			}
		}
		if net, ok := resolved[norm]; ok {
			// Or two slaves can simply both land on that net.
			onNet := slaveRefsOnNet(net, slaves)
			if len(onNet) >= 2 {
				for _, ref := range onNet {
					shared[ref] = struct{}{}
				}
			}
		}
		if len(shared) < 2 {
			continue
		}
		refs := make([]string, 0, len(shared))
		for ref := range shared {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		display := chipSelectDisplayNet(resolved, entries[0].Net)
		findings = append(findings, connectedFinding(
			req,
			RuleSPICSShared,
			"",
			display,
			fmt.Sprintf("Contract %s requires one slave on %s. Observed: %s share %s.", contractID, display, joinSignals(refs), display),
			"Give each slave its own chip-select net.",
			evidenceText(display),
			evidenceText(strings.Join(refs, ", ")),
		))
	}
	return findings
}

// netAlreadyMissing reports whether the shared-net check already flagged this net.
func netAlreadyMissing(missing map[string]struct{}, name string) bool {
	norm := normalizeNetName(name)
	for key := range missing {
		if normalizeNetName(key) == norm {
			return true
		}
	}
	return false
}

// chipSelectDisplayNet uses the schematic name, so a KiCad /IMU_CS stays /IMU_CS.
func chipSelectDisplayNet(resolved map[string]ir.Net, contractNet string) string {
	net, ok := resolved[normalizeNetName(contractNet)]
	if !ok {
		return contractNet
	}
	if name := strings.TrimSpace(net.Name); name != "" {
		return name
	}
	return contractNet
}

// participantRole is the master or slave label from the contract.
func participantRole(participants []InterfaceParticipant, ref string) string {
	for _, participant := range participants {
		if participant.Ref == ref {
			return participant.Role
		}
	}
	return "slave"
}

// chipSelectRefCount is how many different slaves the contract puts on this net.
func chipSelectRefCount(entries []ChipSelect) int {
	seen := map[string]struct{}{}
	for _, entry := range entries {
		seen[entry.Ref] = struct{}{}
	}
	return len(seen)
}

// slaveRefsOnNet lists the slaves that have a pin on this net. The master is left out.
func slaveRefsOnNet(net ir.Net, slaves map[string]struct{}) []string {
	found := map[string]struct{}{}
	for _, pin := range net.Pins {
		if _, ok := slaves[pin.Ref]; ok {
			found[pin.Ref] = struct{}{}
		}
	}
	refs := make([]string, 0, len(found))
	for ref := range found {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}
