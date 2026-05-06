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

// ContractScope says where a requirement applies after a part is matched.
type ContractScope struct {
	ComponentRef  string   `json:"component_ref,omitempty"`
	ComponentType string   `json:"component_type,omitempty"`
	BusType       string   `json:"bus_type,omitempty"`
	MPN           string   `json:"mpn,omitempty"`
	Pins          []string `json:"pins,omitempty"`
	Net           string   `json:"net,omitempty"`
	Rail          string   `json:"rail,omitempty"`
	Role          PinRole  `json:"role,omitempty"`
}

// Requirement is the normalized rule input for one electrical constraint.
type Requirement struct {
	Type              ContractType       `json:"type"`
	Scope             ContractScope      `json:"scope"`
	MinVoltage        *float64           `json:"min_voltage,omitempty"`
	MaxVoltage        *float64           `json:"max_voltage,omitempty"`
	MaxCurrent        *float64           `json:"max_current,omitempty"`
	MinOhms           *float64           `json:"min_ohms,omitempty"`
	MaxOhms           *float64           `json:"max_ohms,omitempty"`
	MaxUtilizationPct *float64           `json:"max_utilization_pct,omitempty"`
	Severity          string             `json:"severity,omitempty"`
	Message           string             `json:"message,omitempty"`
	Fix               string             `json:"fix,omitempty"`
	ContractID        string             `json:"contract_id,omitempty"`
	ContractSource    ContractSourceKind `json:"contract_source,omitempty"`
	ContractFile      string             `json:"contract_file,omitempty"`
	Provenance        Provenance         `json:"provenance,omitempty"`
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
