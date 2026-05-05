package contracts

import (
	"math"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// PinRole describes what a pin electrically does in a design-neutral way.
// Rules use these roles instead of package-, symbol-, or EDA-specific names.
type PinRole string

const (
	RolePowerIn      PinRole = "power_in"
	RolePowerOut     PinRole = "power_out"
	RoleRegulatorOut PinRole = "regulator_out"
	RoleSource       PinRole = "source"
	RoleGPIO         PinRole = "gpio"
	RoleI2CSDA       PinRole = "i2c_sda"
	RoleI2CSCL       PinRole = "i2c_scl"
	RoleSPI          PinRole = "spi"
	RoleUART         PinRole = "uart"
	RoleMotorOut     PinRole = "motor_out"
	RoleGround       PinRole = "ground"
	RoleUnknown      PinRole = "unknown"
)

// Direction captures signal flow at the contract level.
type Direction string

const (
	DirectionInput         Direction = "input"
	DirectionOutput        Direction = "output"
	DirectionBidirectional Direction = "bidirectional"
	DirectionPassive       Direction = "passive"
	DirectionUnknown       Direction = "unknown"
)

// ContractIR is the complete contract view attached to a DesignIR.
// It can be assembled from several independent sources before rules run.
type ContractIR struct {
	Components          map[string]ComponentContract `json:"components"`
	Nets                map[string]NetContract       `json:"nets"`
	PartMatches         []PartMatch                  `json:"part_matches,omitempty"`
	AppliedRequirements []AppliedRequirement         `json:"applied_requirements,omitempty"`
	MissingContractData []MissingContractData        `json:"missing_contract_data,omitempty"`
}

// ComponentContract stores component-level limits and per-pin contracts.
type ComponentContract struct {
	Ref        string                 `json:"ref"`
	MPN        string                 `json:"mpn,omitempty"`
	Pins       map[string]PinContract `json:"pins,omitempty"`
	VoltageMax *float64               `json:"voltage_max,omitempty"`
	Source     string                 `json:"source,omitempty"`
}

// PinContract is the rule-facing electrical contract for a single component pin.
type PinContract struct {
	Ref            string    `json:"ref,omitempty"`
	Pin            string    `json:"pin,omitempty"`
	Role           PinRole   `json:"role"`
	VoltageMin     *float64  `json:"voltage_min,omitempty"`
	VoltageNominal *float64  `json:"voltage_nominal,omitempty"`
	VoltageMax     *float64  `json:"voltage_max,omitempty"`
	CurrentMax     *float64  `json:"current_max,omitempty"`
	LogicFamily    string    `json:"logic_family,omitempty"`
	Direction      Direction `json:"direction"`
	OpenDrain      *bool     `json:"open_drain,omitempty"`
	Source         string    `json:"source,omitempty"`
}

// NetContract stores facts that apply to a whole net, such as inferred or
// explicitly configured rail voltage.
type NetContract struct {
	Net            string   `json:"net"`
	VoltageMin     *float64 `json:"voltage_min,omitempty"`
	VoltageNominal *float64 `json:"voltage_nominal,omitempty"`
	VoltageMax     *float64 `json:"voltage_max,omitempty"`
	LogicFamily    string   `json:"logic_family,omitempty"`
	Source         string   `json:"source,omitempty"`
}

// MissingContractData records known analysis gaps. These become warnings so the
// report is transparent about what was not checked.
type MissingContractData struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref,omitempty"`
	Pin     string `json:"pin,omitempty"`
	Net     string `json:"net,omitempty"`
	Message string `json:"message"`
}

// CoverageSummary is report-facing coverage data for contract completeness.
type CoverageSummary struct {
	ComponentsTotal          int      `json:"components_total"`
	ComponentsWithContracts  int      `json:"components_with_contracts"`
	PinsTotal                int      `json:"pins_total"`
	PinsWithContracts        int      `json:"pins_with_contracts"`
	NetsTotal                int      `json:"nets_total"`
	NetsWithContracts        int      `json:"nets_with_contracts"`
	PartsMatched             int      `json:"parts_matched"`
	ContractsApplied         int      `json:"contracts_applied"`
	CoveragePercentage       float64  `json:"coverage_percentage"`
	UnknownPowerCriticalRefs []string `json:"unknown_power_critical_refs,omitempty"`
	EnabledContractRules     []string `json:"enabled_contract_rules,omitempty"`
	MissingWarnings          []string `json:"missing_warnings,omitempty"`
}

// NewContractIR creates an initialized contract container.
func NewContractIR() *ContractIR {
	return &ContractIR{
		Components: map[string]ComponentContract{},
		Nets:       map[string]NetContract{},
	}
}

// Float64 makes pointer-valued numeric fields readable in struct literals.
// Go does not allow taking the address of a literal such as &3.3.
func Float64(v float64) *float64 {
	return &v
}

// EnsureComponent returns a mutable component contract entry, creating it when
// needed. Call PutComponent after changing the returned value.
func (c *ContractIR) EnsureComponent(ref string) ComponentContract {
	if c.Components == nil {
		c.Components = map[string]ComponentContract{}
	}
	ref = strings.TrimSpace(ref)
	component := c.Components[ref]
	if component.Ref == "" {
		component.Ref = ref
	}
	if component.Pins == nil {
		component.Pins = map[string]PinContract{}
	}
	c.Components[ref] = component
	return component
}

// PutComponent writes a component contract back into the ContractIR.
func (c *ContractIR) PutComponent(component ComponentContract) {
	if c.Components == nil {
		c.Components = map[string]ComponentContract{}
	}
	component.Ref = strings.TrimSpace(component.Ref)
	if component.Ref == "" {
		return
	}
	if component.Pins == nil {
		component.Pins = map[string]PinContract{}
	}
	c.Components[component.Ref] = component
}

// PutPin attaches or merges a pin contract for ref/pin.
func (c *ContractIR) PutPin(ref string, pin string, contract PinContract) {
	ref = strings.TrimSpace(ref)
	pin = strings.TrimSpace(pin)
	if ref == "" || pin == "" {
		return
	}
	component := c.EnsureComponent(ref)
	existing := component.Pins[pin]
	component.Pins[pin] = mergePinContract(existing, contract, ref, pin)
	c.Components[ref] = component
}

// PutNet attaches or merges a net contract.
func (c *ContractIR) PutNet(net string, contract NetContract) {
	net = strings.TrimSpace(net)
	if net == "" {
		return
	}
	if c.Nets == nil {
		c.Nets = map[string]NetContract{}
	}
	existing := c.Nets[net]
	if existing.Net == "" {
		existing.Net = net
	}
	c.Nets[net] = mergeNetContract(existing, contract, net)
}

// PutAppliedRequirement records a concrete requirement after source matching.
func (c *ContractIR) PutAppliedRequirement(req AppliedRequirement) {
	req.ComponentRef = strings.TrimSpace(req.ComponentRef)
	if req.ComponentRef == "" || req.Type == "" {
		return
	}
	req.Provenance = mergeProvenance(req.Provenance, req.Requirement.Provenance)
	if req.Source == "" {
		req.Source = req.Provenance.Source
	}
	if req.Source == "" {
		req.Source = "contract"
	}
	c.AppliedRequirements = append(c.AppliedRequirements, req)
}

// Pin looks up a pin contract by component reference and pin name/number.
func (c *ContractIR) Pin(ref string, pin string) (PinContract, bool) {
	if c == nil {
		return PinContract{}, false
	}
	component, ok := c.Components[strings.TrimSpace(ref)]
	if !ok || component.Pins == nil {
		return PinContract{}, false
	}
	contract, ok := component.Pins[strings.TrimSpace(pin)]
	return contract, ok
}

// Net looks up a contract for a normalized DesignIR net name.
func (c *ContractIR) Net(net string) (NetContract, bool) {
	if c == nil {
		return NetContract{}, false
	}
	contract, ok := c.Nets[strings.TrimSpace(net)]
	return contract, ok
}

// Merge combines another ContractIR into this one. Existing data wins so source
// order can express precedence without losing later missing-data warnings.
func (c *ContractIR) Merge(other *ContractIR) {
	if other == nil {
		return
	}
	if c.Components == nil {
		c.Components = map[string]ComponentContract{}
	}
	if c.Nets == nil {
		c.Nets = map[string]NetContract{}
	}
	for ref, component := range other.Components {
		existing := c.Components[ref]
		if existing.Ref == "" {
			existing.Ref = component.Ref
		}
		if existing.MPN == "" {
			existing.MPN = component.MPN
		}
		if existing.VoltageMax == nil {
			existing.VoltageMax = component.VoltageMax
		}
		if existing.Source == "" {
			existing.Source = component.Source
		}
		if existing.Pins == nil {
			existing.Pins = map[string]PinContract{}
		}
		for pin, pinContract := range component.Pins {
			existing.Pins[pin] = mergePinContract(existing.Pins[pin], pinContract, ref, pin)
		}
		c.Components[ref] = existing
	}
	for net, contract := range other.Nets {
		c.PutNet(net, contract)
	}
	for _, match := range other.PartMatches {
		c.putPartMatch(match)
	}
	for _, req := range other.AppliedRequirements {
		if c.hasHigherPrecedenceRequirement(req) {
			continue
		}
		if c.hasAppliedRequirement(req) {
			continue
		}
		c.PutAppliedRequirement(req)
	}
	c.MissingContractData = append(c.MissingContractData, other.MissingContractData...)
}

// SummarizeCoverage compares DesignIR connectivity with available contracts.
func SummarizeCoverage(design *ir.DesignIR, contractIR *ContractIR) CoverageSummary {
	summary := CoverageSummary{}
	if design == nil {
		return summary
	}
	summary.ComponentsTotal = len(design.Parts)
	summary.NetsTotal = len(design.Nets)

	seenPins := map[string]ir.Pin{}
	for _, pin := range design.Pins {
		key := pin.Ref + "\x00" + pin.Pin
		seenPins[key] = pin
	}
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			key := pin.Ref + "\x00" + pin.Pin
			seenPins[key] = ir.Pin{Ref: pin.Ref, Pin: pin.Pin}
		}
	}
	summary.PinsTotal = len(seenPins)

	if contractIR == nil {
		return summary
	}
	for _, part := range design.Parts {
		component, ok := contractIR.Components[part.Ref]
		if ok && hasComponentContract(component) {
			summary.ComponentsWithContracts++
		}
	}
	for _, pin := range seenPins {
		if _, ok := contractIR.Pin(pin.Ref, pin.Pin); ok {
			summary.PinsWithContracts++
		}
	}
	for _, net := range design.Nets {
		if contract, ok := contractIR.Nets[net.Name]; ok && hasNetContract(contract) {
			summary.NetsWithContracts++
		}
	}
	for _, missing := range contractIR.MissingContractData {
		if strings.TrimSpace(missing.Message) != "" {
			summary.MissingWarnings = append(summary.MissingWarnings, missing.Message)
		}
	}
	summary.PartsMatched = len(uniquePartMatches(contractIR.PartMatches))
	summary.ContractsApplied = len(contractIR.AppliedRequirements)
	summary.EnabledContractRules = EnabledRuleIDs()
	summary.UnknownPowerCriticalRefs = UnknownPowerCriticalRefs(design, contractIR)
	if summary.ComponentsTotal > 0 {
		summary.CoveragePercentage = roundPercentage(float64(summary.ComponentsWithContracts) / float64(summary.ComponentsTotal) * 100)
	}
	sort.Strings(summary.MissingWarnings)
	return summary
}

func (c *ContractIR) putPartMatch(match PartMatch) {
	match.Ref = strings.TrimSpace(match.Ref)
	if match.Ref == "" {
		return
	}
	for i, existing := range c.PartMatches {
		if existing.Ref == match.Ref {
			c.PartMatches[i] = match
			return
		}
	}
	c.PartMatches = append(c.PartMatches, match)
}

func (c *ContractIR) hasHigherPrecedenceRequirement(req AppliedRequirement) bool {
	// Contract sources are merged in precedence order. Once an earlier source
	// has supplied the same requirement type for the same component, later
	// sources cannot replace it.
	for _, existing := range c.AppliedRequirements {
		if existing.ComponentRef == req.ComponentRef && existing.Type == req.Type && existing.Source != req.Source {
			return true
		}
	}
	component, ok := c.Components[req.ComponentRef]
	if !ok {
		return false
	}
	if component.Source == "meta.yaml" && (component.VoltageMax != nil || len(component.Pins) > 0) {
		switch req.Type {
		case ContractSupplyAbsMax, ContractSupplyRecommendedRange, ContractGPIOAbsMax:
			return true
		}
	}
	for _, pin := range req.Scope.Pins {
		if pinContract, ok := component.Pins[pin]; ok && pinContract.Source != "" && pinContract.Source != req.Source {
			switch req.Type {
			case ContractSupplyAbsMax, ContractSupplyRecommendedRange, ContractGPIOAbsMax, ContractMotorDriverVMRange:
				return true
			}
		}
	}
	return false
}

func (c *ContractIR) hasAppliedRequirement(req AppliedRequirement) bool {
	key := appliedRequirementKey(req)
	for _, existing := range c.AppliedRequirements {
		if appliedRequirementKey(existing) == key {
			return true
		}
	}
	return false
}

func appliedRequirementKey(req AppliedRequirement) string {
	pins := append([]string(nil), req.Scope.Pins...)
	sort.Strings(pins)
	return req.ComponentRef + "\x00" + string(req.Type) + "\x00" + strings.Join(pins, ",")
}

func uniquePartMatches(matches []PartMatch) map[string]PartMatch {
	out := make(map[string]PartMatch, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.Ref) == "" {
			continue
		}
		out[match.Ref] = match
	}
	return out
}

func UnknownPowerCriticalRefs(design *ir.DesignIR, contractIR *ContractIR) []string {
	if design == nil || contractIR == nil {
		return nil
	}
	matched := uniquePartMatches(contractIR.PartMatches)
	refs := map[string]struct{}{}
	for _, net := range design.Nets {
		netContract, ok := contractIR.Net(net.Name)
		if !ok || netContract.VoltageNominal == nil || *netContract.VoltageNominal <= 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(net.Name), "GND") {
			continue
		}
		for _, pin := range net.Pins {
			ref := strings.TrimSpace(pin.Ref)
			if ref == "" {
				continue
			}
			if _, ok := matched[ref]; ok {
				continue
			}
			if _, ok := contractIR.Components[ref]; ok {
				continue
			}
			if isPowerCriticalRef(ref) {
				refs[ref] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func isPowerCriticalRef(ref string) bool {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	return strings.HasPrefix(ref, "U") || strings.HasPrefix(ref, "IC")
}

func roundPercentage(value float64) float64 {
	return math.Round(value*100) / 100
}

// mergePinContract fills missing fields in an existing pin contract from a
// later source while keeping already-established contract data stable.
func mergePinContract(existing PinContract, next PinContract, ref string, pin string) PinContract {
	if existing.Ref == "" {
		existing.Ref = ref
	}
	if existing.Pin == "" {
		existing.Pin = pin
	}
	if existing.Role == "" || existing.Role == RoleUnknown {
		existing.Role = next.Role
	}
	if existing.Role == "" {
		existing.Role = RoleUnknown
	}
	if existing.VoltageMin == nil {
		existing.VoltageMin = next.VoltageMin
	}
	if existing.VoltageNominal == nil {
		existing.VoltageNominal = next.VoltageNominal
	}
	if existing.VoltageMax == nil {
		existing.VoltageMax = next.VoltageMax
	}
	if existing.CurrentMax == nil {
		existing.CurrentMax = next.CurrentMax
	}
	if existing.LogicFamily == "" {
		existing.LogicFamily = next.LogicFamily
	}
	if existing.Direction == "" || existing.Direction == DirectionUnknown {
		existing.Direction = next.Direction
	}
	if existing.Direction == "" {
		existing.Direction = DirectionUnknown
	}
	if existing.OpenDrain == nil {
		existing.OpenDrain = next.OpenDrain
	}
	if existing.Source == "" {
		existing.Source = next.Source
	}
	return existing
}

// mergeNetContract applies the same "first source wins" merge policy for nets.
func mergeNetContract(existing NetContract, next NetContract, net string) NetContract {
	if existing.Net == "" {
		existing.Net = net
	}
	if existing.VoltageMin == nil {
		existing.VoltageMin = next.VoltageMin
	}
	if existing.VoltageNominal == nil {
		existing.VoltageNominal = next.VoltageNominal
	}
	if existing.VoltageMax == nil {
		existing.VoltageMax = next.VoltageMax
	}
	if existing.LogicFamily == "" {
		existing.LogicFamily = next.LogicFamily
	}
	if existing.Source == "" {
		existing.Source = next.Source
	}
	return existing
}

func hasComponentContract(component ComponentContract) bool {
	return component.VoltageMax != nil || len(component.Pins) > 0 || component.MPN != ""
}

func hasNetContract(contract NetContract) bool {
	return contract.VoltageMin != nil || contract.VoltageNominal != nil || contract.VoltageMax != nil || contract.LogicFamily != ""
}
