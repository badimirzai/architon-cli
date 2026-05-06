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
	Net  ir.Net
	Role string
	Key  string
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
			signalNets = []i2cSignalNet{{Net: net, Role: "signal", Key: "scope"}}
		}
	}
	findings := make([]Finding, 0)
	for _, signalNet := range signalNets {
		pullups := pullupsForSignalNet(design, contractIR, signalNet.Net.Name, req.Scope)
		if len(pullups) == 0 {
			finding := findingForRequirement(req, fmt.Sprintf("Net %s has no pull-up resistor in scope", signalNet.Net.Name))
			finding.Net = signalNet.Net.Name
			findings = append(findings, finding)
			continue
		}
		effective, ok := effectivePullupOhms(pullups)
		if !ok {
			continue
		}
		if req.MinOhms != nil && effective < *req.MinOhms {
			finding := findingForRequirement(req, fmt.Sprintf("Net %s effective pull-up %.0f ohms is below minimum %.0f ohms", signalNet.Net.Name, effective, *req.MinOhms))
			finding.Net = signalNet.Net.Name
			finding.ComponentRef = pullups[0].Ref
			findings = append(findings, finding)
			continue
		}
		if req.MaxOhms != nil && effective > *req.MaxOhms {
			finding := findingForRequirement(req, fmt.Sprintf("Net %s effective pull-up %.0f ohms is above maximum %.0f ohms", signalNet.Net.Name, effective, *req.MaxOhms))
			finding.Net = signalNet.Net.Name
			finding.ComponentRef = pullups[0].Ref
			findings = append(findings, finding)
		}
	}
	return findings
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

// evaluateNoI2CAddressConflict checks duplicate device addresses per I2C bus.
func evaluateNoI2CAddressConflict(design *ir.DesignIR, req AppliedRequirement) []Finding {
	buses := scopedI2CBuses(design, req.Scope)
	parts := partIndex(design)
	findings := make([]Finding, 0)
	for _, bus := range buses {
		// Address conflicts are scoped per inferred bus, not globally across the
		// whole design.
		byAddress := map[uint64][]string{}
		for _, ref := range bus {
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
			finding := findingForRequirement(req, fmt.Sprintf("I2C devices %s share address 0x%02X", strings.Join(refs, ", "), address))
			finding.ComponentRef = refs[0]
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
		for _, busRefs := range scopedI2CBuses(design, scope) {
			for _, ref := range busRefs {
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
	out := make([]i2cSignalNet, 0)
	for _, net := range sortedIRNets(design.Nets) {
		if scope.Net != "" && net.Name != scope.Net {
			continue
		}
		role := i2cNetRole(net)
		if role == "" {
			continue
		}
		out = append(out, i2cSignalNet{Net: net, Role: role, Key: i2cBusKey(net.Name, role)})
	}
	return out
}

// scopedI2CBuses groups I2C components by inferred bus key.
func scopedI2CBuses(design *ir.DesignIR, scope ContractScope) [][]string {
	signals := scopedI2CSignalNets(design, scope)
	byKey := map[string]map[string]struct{}{}
	for _, signal := range signals {
		if byKey[signal.Key] == nil {
			byKey[signal.Key] = map[string]struct{}{}
		}
		for _, pin := range signal.Net.Pins {
			if passiveRef(pin.Ref) {
				continue
			}
			byKey[signal.Key][pin.Ref] = struct{}{}
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][]string, 0, len(keys))
	for _, key := range keys {
		refs := make([]string, 0, len(byKey[key]))
		for ref := range byKey[key] {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		out = append(out, refs)
	}
	return out
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
		if passiveRef(ref) && !looksLikeResistor(part) {
			continue
		}
		if !looksLikeResistor(part) && pullupOhms(part) == 0 {
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
			if contractIR != nil {
				if netContract, ok := contractIR.Net(conn.Net); ok && netContract.VoltageNominal != nil {
					out = append(out, pullupCandidate{Ref: ref, Ohms: ohms, RailNet: conn.Net, SignalNet: signalNet})
					break
				}
			}
			if looksLikeRailNet(conn.Net) {
				out = append(out, pullupCandidate{Ref: ref, Ohms: ohms, RailNet: conn.Net, SignalNet: signalNet})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
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

// parseInteger parses decimal or 0x-prefixed integer strings.
func parseInteger(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" {
		return 0, fmt.Errorf("empty integer")
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		base = 0
	}
	return strconv.ParseUint(value, base, 16)
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
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "Ω", "ohm")
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
	for _, key := range []string{"architon_component_type", "component_type", "type"} {
		if strings.EqualFold(strings.TrimSpace(fields[key]), "resistor") {
			return true
		}
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
