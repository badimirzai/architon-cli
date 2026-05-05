package contracts

// ContractType identifies the deterministic built-in contract rule family.
type ContractType string

const (
	ContractSupplyAbsMax           ContractType = "supply_abs_max"
	ContractSupplyRecommendedRange ContractType = "supply_recommended_range"
	ContractGPIOAbsMax             ContractType = "gpio_abs_max"
	ContractMotorDriverVMRange     ContractType = "motor_driver_vm_range"
	ContractRegulatorOutputCurrent ContractType = "regulator_output_current"
)

// SystemContract is a deterministic component contract. It is intentionally
// small: it describes known electrical requirements for one concrete MPN or
// alias set, not a generic searchable parts database.
type SystemContract struct {
	MPN          string        `json:"mpn"`
	Manufacturer string        `json:"manufacturer,omitempty"`
	Aliases      []string      `json:"aliases,omitempty"`
	Description  string        `json:"description,omitempty"`
	Requirements []Requirement `json:"requirements"`
	Provenance   Provenance    `json:"provenance"`
}

// ContractScope says where a requirement applies after a part is matched.
type ContractScope struct {
	ComponentRef string   `json:"component_ref,omitempty"`
	MPN          string   `json:"mpn,omitempty"`
	Pins         []string `json:"pins,omitempty"`
	Net          string   `json:"net,omitempty"`
	Role         PinRole  `json:"role,omitempty"`
}

// Requirement is the normalized rule input for one electrical constraint.
type Requirement struct {
	Type       ContractType  `json:"type"`
	Scope      ContractScope `json:"scope"`
	MinVoltage *float64      `json:"min_voltage,omitempty"`
	MaxVoltage *float64      `json:"max_voltage,omitempty"`
	MaxCurrent *float64      `json:"max_current,omitempty"`
	Severity   string        `json:"severity,omitempty"`
	Message    string        `json:"message,omitempty"`
	Fix        string        `json:"fix,omitempty"`
	Provenance Provenance    `json:"provenance,omitempty"`
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
