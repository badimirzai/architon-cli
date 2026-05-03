package contracts

import (
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
	MissingContractData []MissingContractData        `json:"missing_contract_data,omitempty"`
	PartMatches         []PartMatchSummary           `json:"part_matches,omitempty"`
}

// ComponentContract stores component-level limits and per-pin contracts.
type ComponentContract struct {
	Ref         string                 `json:"ref"`
	MPN         string                 `json:"mpn,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Pins        map[string]PinContract `json:"pins,omitempty"`
	VoltageMax  *float64               `json:"voltage_max,omitempty"`
	AbsMaxV     *float64               `json:"abs_max_v,omitempty"`
	Source      string                 `json:"source,omitempty"`
	Logic       *LogicContract         `json:"logic,omitempty"`
	MotorDriver *MotorDriverContract   `json:"motor_driver,omitempty"`
	Protection  *ProtectionContract    `json:"protection,omitempty"`
	Thermal     *ThermalContract       `json:"thermal,omitempty"`
}

// PinContract is the rule-facing electrical contract for a single component pin.
type PinContract struct {
	Ref                   string    `json:"ref,omitempty"`
	Pin                   string    `json:"pin,omitempty"`
	Name                  string    `json:"name,omitempty"`
	Role                  PinRole   `json:"role"`
	SupplyName            string    `json:"supply_name,omitempty"`
	VoltageMin            *float64  `json:"voltage_min,omitempty"`
	VoltageNominal        *float64  `json:"voltage_nominal,omitempty"`
	VoltageMax            *float64  `json:"voltage_max,omitempty"`
	RecommendedVoltageMin *float64  `json:"recommended_voltage_min,omitempty"`
	RecommendedVoltageMax *float64  `json:"recommended_voltage_max,omitempty"`
	AbsVoltageMin         *float64  `json:"abs_voltage_min,omitempty"`
	AbsVoltageMax         *float64  `json:"abs_voltage_max,omitempty"`
	CurrentMax            *float64  `json:"current_max,omitempty"`
	TypicalCurrent        *float64  `json:"typical_current,omitempty"`
	InrushCurrent         *float64  `json:"inrush_current,omitempty"`
	OutputCurrentMax      *float64  `json:"output_current_max,omitempty"`
	DropoutVoltage        *float64  `json:"dropout_voltage,omitempty"`
	RequiresInputSupply   string    `json:"requires_input_supply,omitempty"`
	LogicFamily           string    `json:"logic_family,omitempty"`
	VIHMin                *float64  `json:"vih_min,omitempty"`
	VILMax                *float64  `json:"vil_max,omitempty"`
	VOHMin                *float64  `json:"voh_min,omitempty"`
	VOLMax                *float64  `json:"vol_max,omitempty"`
	MotorSupply           bool      `json:"motor_supply,omitempty"`
	MotorOutput           bool      `json:"motor_output,omitempty"`
	ClampCurrentMaxMA     *float64  `json:"clamp_current_max_ma,omitempty"`
	Direction             Direction `json:"direction"`
	OpenDrain             *bool     `json:"open_drain,omitempty"`
	Source                string    `json:"source,omitempty"`
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

// LogicContract stores component-wide IO/logic limits.
type LogicContract struct {
	DefaultIODomain       string   `json:"default_io_domain,omitempty"`
	IOAbsMinV             *float64 `json:"io_abs_min_v,omitempty"`
	IOAbsMaxV             *float64 `json:"io_abs_max_v,omitempty"`
	IORecommendedMinV     *float64 `json:"io_recommended_min_v,omitempty"`
	IORecommendedMaxV     *float64 `json:"io_recommended_max_v,omitempty"`
	FiveVTolerant         *bool    `json:"five_v_tolerant,omitempty"`
	FiveVTolerantPins     []string `json:"five_v_tolerant_pins,omitempty"`
	NonFiveVTolerantPins  []string `json:"non_five_v_tolerant_pins,omitempty"`
	VIHMinV               *float64 `json:"vih_min_v,omitempty"`
	VILMaxV               *float64 `json:"vil_max_v,omitempty"`
	VOHMinV               *float64 `json:"voh_min_v,omitempty"`
	VOLMaxV               *float64 `json:"vol_max_v,omitempty"`
	MaxInjectionCurrentMA *float64 `json:"max_injection_current_ma,omitempty"`
}

// MotorDriverContract stores component-wide motor-driver limits.
type MotorDriverContract struct {
	VMSupplyName             string   `json:"vm_supply_name,omitempty"`
	LogicSupplyName          string   `json:"logic_supply_name,omitempty"`
	MotorOutputPins          []string `json:"motor_output_pins,omitempty"`
	RecommendedVMMinV        *float64 `json:"recommended_vm_min_v,omitempty"`
	RecommendedVMMaxV        *float64 `json:"recommended_vm_max_v,omitempty"`
	AbsVMMaxV                *float64 `json:"abs_vm_max_v,omitempty"`
	ContinuousOutputCurrentA *float64 `json:"continuous_output_current_a,omitempty"`
	PeakOutputCurrentA       *float64 `json:"peak_output_current_a,omitempty"`
	CurrentLimitA            *float64 `json:"current_limit_a,omitempty"`
	HasCurrentRegulation     *bool    `json:"has_current_regulation,omitempty"`
	HasThermalShutdown       *bool    `json:"has_thermal_shutdown,omitempty"`
	HasUVLO                  *bool    `json:"has_uvlo,omitempty"`
}

// ProtectionContract stores component-wide protection behavior.
type ProtectionContract struct {
	ReversePolarityProtected *bool    `json:"reverse_polarity_protected,omitempty"`
	OvercurrentProtected     *bool    `json:"overcurrent_protected,omitempty"`
	ThermalShutdown          *bool    `json:"thermal_shutdown,omitempty"`
	UVLOThresholdV           *float64 `json:"uvlo_threshold_v,omitempty"`
	OVPThresholdV            *float64 `json:"ovp_threshold_v,omitempty"`
	ClampVoltageV            *float64 `json:"clamp_voltage_v,omitempty"`
	MaxClampCurrentMA        *float64 `json:"max_clamp_current_ma,omitempty"`
}

// ThermalContract stores thermal limits used by higher-level checks.
type ThermalContract struct {
	MaxJunctionTempC       *float64 `json:"max_junction_temp_c,omitempty"`
	RecommendedAmbientMaxC *float64 `json:"recommended_ambient_max_c,omitempty"`
	ThetaJACPerW           *float64 `json:"theta_ja_c_per_w,omitempty"`
	PowerDissipationW      *float64 `json:"power_dissipation_w,omitempty"`
}

// PartMatchSummary records deterministic library matching coverage.
type PartMatchSummary struct {
	Ref           string `json:"ref"`
	MPN           string `json:"mpn,omitempty"`
	Matched       bool   `json:"matched"`
	MatchedMPN    string `json:"matched_mpn,omitempty"`
	Category      string `json:"category,omitempty"`
	Source        string `json:"source,omitempty"`
	Reason        string `json:"reason,omitempty"`
	PowerCritical bool   `json:"power_critical,omitempty"`
}

// CoverageSummary is report-facing coverage data for contract completeness.
type CoverageSummary struct {
	ComponentsTotal         int      `json:"components_total"`
	ComponentsWithContracts int      `json:"components_with_contracts"`
	PinsTotal               int      `json:"pins_total"`
	PinsWithContracts       int      `json:"pins_with_contracts"`
	NetsTotal               int      `json:"nets_total"`
	NetsWithContracts       int      `json:"nets_with_contracts"`
	MissingWarnings         []string `json:"missing_warnings,omitempty"`
	PartsMatched            int      `json:"parts_matched,omitempty"`
	PartsTotal              int      `json:"parts_total,omitempty"`
	PowerContractsApplied   int      `json:"power_contracts_applied,omitempty"`
	ContractCoveragePct     float64  `json:"contract_coverage_pct,omitempty"`
	UnknownPowerCritical    []string `json:"unknown_power_critical,omitempty"`
	RulesEnabledByContracts []string `json:"rules_enabled_by_contracts,omitempty"`
}

// NewContractIR creates an initialized contract container.
func NewContractIR() *ContractIR {
	return &ContractIR{
		Components: map[string]ComponentContract{},
		Nets:       map[string]NetContract{},
	}
}

// Float64 is a small helper for tests and contract builders.
func Float64(v float64) *float64 {
	return &v
}

// Bool is a small helper for tests and contract builders.
func Bool(v bool) *bool {
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
		if existing.Category == "" {
			existing.Category = component.Category
		}
		if existing.VoltageMax == nil {
			existing.VoltageMax = component.VoltageMax
		}
		if existing.AbsMaxV == nil {
			existing.AbsMaxV = component.AbsMaxV
		}
		if existing.Source == "" {
			existing.Source = component.Source
		}
		if existing.Logic == nil {
			existing.Logic = cloneLogicContract(component.Logic)
		}
		if existing.MotorDriver == nil {
			existing.MotorDriver = cloneMotorDriverContract(component.MotorDriver)
		}
		if existing.Protection == nil {
			existing.Protection = cloneProtectionContract(component.Protection)
		}
		if existing.Thermal == nil {
			existing.Thermal = cloneThermalContract(component.Thermal)
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
	c.MissingContractData = append(c.MissingContractData, other.MissingContractData...)
	c.PartMatches = mergePartMatches(c.PartMatches, other.PartMatches)
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
	summary.PartsTotal = len(design.Parts)
	unknownPowerCritical := map[string]struct{}{}
	for _, match := range contractIR.PartMatches {
		if match.Matched {
			summary.PartsMatched++
			summary.PowerContractsApplied++
			continue
		}
		if match.PowerCritical && strings.TrimSpace(match.Ref) != "" {
			unknownPowerCritical[match.Ref] = struct{}{}
		}
	}
	if summary.PartsTotal > 0 {
		summary.ContractCoveragePct = float64(summary.PartsMatched) * 100 / float64(summary.PartsTotal)
	}
	for ref := range unknownPowerCritical {
		summary.UnknownPowerCritical = append(summary.UnknownPowerCritical, ref)
	}
	sort.Strings(summary.UnknownPowerCritical)
	summary.RulesEnabledByContracts = rulesEnabledByContracts(contractIR)
	sort.Strings(summary.MissingWarnings)
	return summary
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
	if existing.Name == "" {
		existing.Name = next.Name
	}
	if existing.Role == "" || existing.Role == RoleUnknown {
		existing.Role = next.Role
	}
	if existing.Role == "" {
		existing.Role = RoleUnknown
	}
	if existing.SupplyName == "" {
		existing.SupplyName = next.SupplyName
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
	if existing.RecommendedVoltageMin == nil {
		existing.RecommendedVoltageMin = next.RecommendedVoltageMin
	}
	if existing.RecommendedVoltageMax == nil {
		existing.RecommendedVoltageMax = next.RecommendedVoltageMax
	}
	if existing.AbsVoltageMin == nil {
		existing.AbsVoltageMin = next.AbsVoltageMin
	}
	if existing.AbsVoltageMax == nil {
		existing.AbsVoltageMax = next.AbsVoltageMax
	}
	if existing.CurrentMax == nil {
		existing.CurrentMax = next.CurrentMax
	}
	if existing.TypicalCurrent == nil {
		existing.TypicalCurrent = next.TypicalCurrent
	}
	if existing.InrushCurrent == nil {
		existing.InrushCurrent = next.InrushCurrent
	}
	if existing.OutputCurrentMax == nil {
		existing.OutputCurrentMax = next.OutputCurrentMax
	}
	if existing.DropoutVoltage == nil {
		existing.DropoutVoltage = next.DropoutVoltage
	}
	if existing.RequiresInputSupply == "" {
		existing.RequiresInputSupply = next.RequiresInputSupply
	}
	if existing.LogicFamily == "" {
		existing.LogicFamily = next.LogicFamily
	}
	if existing.VIHMin == nil {
		existing.VIHMin = next.VIHMin
	}
	if existing.VILMax == nil {
		existing.VILMax = next.VILMax
	}
	if existing.VOHMin == nil {
		existing.VOHMin = next.VOHMin
	}
	if existing.VOLMax == nil {
		existing.VOLMax = next.VOLMax
	}
	if !existing.MotorSupply {
		existing.MotorSupply = next.MotorSupply
	}
	if !existing.MotorOutput {
		existing.MotorOutput = next.MotorOutput
	}
	if existing.ClampCurrentMaxMA == nil {
		existing.ClampCurrentMaxMA = next.ClampCurrentMaxMA
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
	return component.VoltageMax != nil ||
		component.AbsMaxV != nil ||
		len(component.Pins) > 0 ||
		component.MPN != "" ||
		component.Logic != nil ||
		component.MotorDriver != nil ||
		component.Protection != nil ||
		component.Thermal != nil
}

func hasNetContract(contract NetContract) bool {
	return contract.VoltageMin != nil || contract.VoltageNominal != nil || contract.VoltageMax != nil || contract.LogicFamily != ""
}

func mergePartMatches(existing []PartMatchSummary, next []PartMatchSummary) []PartMatchSummary {
	if len(next) == 0 {
		return existing
	}
	byRef := map[string]int{}
	out := append([]PartMatchSummary(nil), existing...)
	for i, match := range out {
		byRef[match.Ref] = i
	}
	for _, match := range next {
		ref := strings.TrimSpace(match.Ref)
		if ref == "" {
			continue
		}
		match.Ref = ref
		if idx, ok := byRef[ref]; ok {
			if !out[idx].Matched && match.Matched {
				out[idx] = match
			}
			continue
		}
		byRef[ref] = len(out)
		out = append(out, match)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Ref < out[j].Ref
	})
	return out
}

func rulesEnabledByContracts(contractIR *ContractIR) []string {
	if contractIR == nil {
		return nil
	}
	enabled := map[string]struct{}{}
	for _, component := range contractIR.Components {
		if component.Logic != nil {
			if component.Logic.IOAbsMaxV != nil {
				enabled["gpio_abs_max"] = struct{}{}
			}
			if component.Logic.VIHMinV != nil || component.Logic.VILMaxV != nil || component.Logic.VOHMinV != nil || component.Logic.VOLMaxV != nil {
				enabled["logic_level_margin"] = struct{}{}
			}
		}
		if component.MotorDriver != nil {
			if component.MotorDriver.AbsVMMaxV != nil || component.MotorDriver.RecommendedVMMinV != nil || component.MotorDriver.RecommendedVMMaxV != nil {
				enabled["motor_driver_vm"] = struct{}{}
			}
			if component.MotorDriver.ContinuousOutputCurrentA != nil || component.MotorDriver.PeakOutputCurrentA != nil {
				enabled["motor_driver_current"] = struct{}{}
			}
		}
		for _, pin := range component.Pins {
			if pin.AbsVoltageMax != nil {
				enabled["supply_abs_max"] = struct{}{}
			}
			if pin.RecommendedVoltageMin != nil || pin.RecommendedVoltageMax != nil {
				enabled["supply_recommended_range"] = struct{}{}
			}
			if pin.OutputCurrentMax != nil {
				enabled["regulator_current"] = struct{}{}
			}
			if pin.ClampCurrentMaxMA != nil {
				enabled["protection_clamp_current"] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(enabled))
	for rule := range enabled {
		out = append(out, rule)
	}
	sort.Strings(out)
	return out
}

func cloneLogicContract(in *LogicContract) *LogicContract {
	if in == nil {
		return nil
	}
	out := *in
	out.FiveVTolerantPins = append([]string(nil), in.FiveVTolerantPins...)
	out.NonFiveVTolerantPins = append([]string(nil), in.NonFiveVTolerantPins...)
	return &out
}

func cloneMotorDriverContract(in *MotorDriverContract) *MotorDriverContract {
	if in == nil {
		return nil
	}
	out := *in
	out.MotorOutputPins = append([]string(nil), in.MotorOutputPins...)
	return &out
}

func cloneProtectionContract(in *ProtectionContract) *ProtectionContract {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneThermalContract(in *ThermalContract) *ThermalContract {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
