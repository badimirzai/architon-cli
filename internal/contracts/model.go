package contracts

// ContractType identifies the deterministic built-in contract rule family.
type ContractType string

const (
	ContractSupplyAbsMax           ContractType = "supply_abs_max"
	ContractSupplyRecommendedRange ContractType = "supply_recommended_range"
	ContractGPIOAbsMax             ContractType = "gpio_abs_max"
	ContractMotorDriverVMRange     ContractType = "motor_driver_vm_range"
	ContractRegulatorOutputCurrent ContractType = "regulator_output_current"
	ContractCommonGround           ContractType = "common_ground"
	ContractPullupOhms             ContractType = "pullup_ohms"
	ContractVoltageCompatible      ContractType = "voltage_compatible"
	ContractCurrentBudget          ContractType = "current_budget"
	ContractNoI2CAddressConflict   ContractType = "no_i2c_address_conflict"
	// ContractConnected is the YAML require.connected check. Findings use the
	// RuleInterface* IDs below, not this type name.
	ContractConnected ContractType = "connected"
	// ContractTerminated counts two-pin parts of a required resistance across
	// two nets. Findings use the termination rule IDs below, not this type name.
	ContractTerminated ContractType = "terminated"
)

// Interface findings. A missing part or net is reported alone; not-connected
// is only emitted once both the component and the net exist. Optional pin
// bindings compare a contract token with the netlist pin name or pin number.
const (
	RuleInterfaceComponentMissing = "interface_component_missing"
	RuleInterfaceNetMissing       = "interface_net_missing"
	RuleInterfaceNotConnected     = "interface_not_connected"
	RuleInterfacePinMismatch      = "interface_pin_mismatch"
	RuleInterfacePinConflict      = "interface_pin_conflict"
	// A terminated bus has fewer two-pin parts of the required resistance than count.
	RuleTerminationCountLow = "termination_count_low"
	// A terminated bus has more two-pin parts of the required resistance than count.
	RuleTerminationCountHigh = "termination_count_high"
	// Two SPI chip-select entries name one net, or two slaves each have a pin on it.
	RuleSPICSShared = "spi_cs_shared"
)

// ContractSourceKind is the report-facing provenance enum for contract-backed
// findings. Source keeps the existing internal source name for compatibility.
type ContractSourceKind string

const (
	ContractSourceBuiltIn  ContractSourceKind = "built_in"
	ContractSourceUserYAML ContractSourceKind = "user_yaml"
	ContractSourceMetaYAML ContractSourceKind = "meta_yaml"
	ContractSourceInferred ContractSourceKind = "inferred"
)

// SystemContract is a deterministic component contract. It is intentionally
// small: it describes known electrical requirements for one concrete MPN or
// alias set, not a generic searchable parts database.
type SystemContract struct {
	ID           string             `json:"id,omitempty"`
	MPN          string             `json:"mpn"`
	Manufacturer string             `json:"manufacturer,omitempty"`
	Aliases      []string           `json:"aliases,omitempty"`
	Description  string             `json:"description,omitempty"`
	Scope        ContractScope      `json:"scope,omitempty"`
	Requirements []Requirement      `json:"requirements"`
	GroundPins   []string           `json:"-"`
	SourceKind   ContractSourceKind `json:"source_kind,omitempty"`
	ContractFile string             `json:"contract_file,omitempty"`
	Provenance   Provenance         `json:"provenance"`
}

// I2CBusNets names the two signal nets that define one explicit I2C bus.
type I2CBusNets struct {
	SDA string `json:"sda,omitempty"`
	SCL string `json:"scl,omitempty"`
}

// InterfaceParticipant is one component that must sit on every net of a connected interface.
// Role is a master/slave label stored from the contract. It is not looked up on the MCU.
// Pins optionally names the pin that must land on each connected net. Keys are net names
// from the contract. Values are pin names or pin numbers. An empty map checks connectivity only.
type InterfaceParticipant struct {
	Ref  string            `json:"ref"`
	Role string            `json:"role"`
	Pins map[string]string `json:"pins,omitempty"`
}

// ChipSelect binds one slave to the net that must be its chip-select.
// That net is not one of the shared connected.nets. The master may also sit on it.
type ChipSelect struct {
	Ref string `json:"ref"`
	Net string `json:"net"`
}

// ContractScope says where a requirement applies after a part is matched.
type ContractScope struct {
	ComponentRef  string      `json:"component_ref,omitempty"`
	ComponentType string      `json:"component_type,omitempty"`
	BusType       string      `json:"bus_type,omitempty"`
	BusID         string      `json:"bus_id,omitempty"`
	Nets          *I2CBusNets `json:"nets,omitempty"`
	MPN           string      `json:"mpn,omitempty"`
	Pins          []string    `json:"pins,omitempty"`
	Net           string      `json:"net,omitempty"`
	Rail          string      `json:"rail,omitempty"`
	Role          PinRole     `json:"role,omitempty"`
}

// Requirement is the normalized rule input for one electrical constraint.
type Requirement struct {
	Type              ContractType  `json:"type"`
	Scope             ContractScope `json:"scope"`
	MinVoltage        *float64      `json:"min_voltage,omitempty"`
	MaxVoltage        *float64      `json:"max_voltage,omitempty"`
	MaxCurrent        *float64      `json:"max_current,omitempty"`
	MinOhms           *float64      `json:"min_ohms,omitempty"`
	MaxOhms           *float64      `json:"max_ohms,omitempty"`
	MaxUtilizationPct *float64      `json:"max_utilization_pct,omitempty"`
	// Nets and Participants are set for ContractConnected.
	// Nets are schematic net names, not peripheral names such as SPI2.
	// ChipSelects is set for an SPI connected contract that names per-slave chip-select nets.
	// ResistanceOhms and TerminatorCount are set for ContractTerminated.
	// Terminated nets are the two signal nets a terminator must span.
	Nets            []string               `json:"nets,omitempty"`
	Participants    []InterfaceParticipant `json:"participants,omitempty"`
	ChipSelects     []ChipSelect           `json:"chip_selects,omitempty"`
	ResistanceOhms  *float64               `json:"resistance_ohms,omitempty"`
	TerminatorCount *int                   `json:"terminator_count,omitempty"`
	Severity        string                 `json:"severity,omitempty"`
	Message         string                 `json:"message,omitempty"`
	Fix             string                 `json:"fix,omitempty"`
	ContractID      string                 `json:"contract_id,omitempty"`
	ContractSource  ContractSourceKind     `json:"contract_source,omitempty"`
	ContractFile    string                 `json:"contract_file,omitempty"`
	Provenance      Provenance             `json:"provenance,omitempty"`
}

// AppliedRequirement is a Requirement bound to a concrete DesignIR component.
type AppliedRequirement struct {
	Requirement
	ComponentRef string     `json:"component_ref"`
	ComponentMPN string     `json:"component_mpn,omitempty"`
	Source       string     `json:"source"`
	Provenance   Provenance `json:"provenance"`
}

// PartMatch records a deterministic match between a DesignIR part and a
// SystemContract.
type PartMatch struct {
	Ref         string     `json:"ref"`
	MPN         string     `json:"mpn"`
	ContractMPN string     `json:"contract_mpn"`
	Kind        string     `json:"kind"`
	Source      string     `json:"source"`
	Provenance  Provenance `json:"provenance"`
}
