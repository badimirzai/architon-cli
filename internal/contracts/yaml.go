package contracts

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
	"gopkg.in/yaml.v3"
)

const userYAMLSourceName = "user_yaml"

// These structs mirror the public contracts.yaml schema. They stay separate
// from SystemContract so YAML validation can remain strict and user-facing.
type contractsYAMLFile struct {
	Contracts []contractYAML `yaml:"contracts"`
}

type contractYAML struct {
	ID          string          `yaml:"id"`
	Description string          `yaml:"description"`
	Scope       scopeYAML       `yaml:"scope"`
	Require     requirementYAML `yaml:"require"`
	Severity    string          `yaml:"severity"`
}

type scopeYAML struct {
	BusType       string `yaml:"bus_type"`
	ComponentType string `yaml:"component_type"`
	ComponentRef  string `yaml:"component_ref"`
	Net           string `yaml:"net"`
	Rail          string `yaml:"rail"`
}

type requirementYAML struct {
	CommonGround         *bool              `yaml:"common_ground"`
	PullupOhms           *pullupOhmsYAML    `yaml:"pullup_ohms"`
	VoltageCompatible    *bool              `yaml:"voltage_compatible"`
	CurrentBudget        *currentBudgetYAML `yaml:"current_budget"`
	NoI2CAddressConflict *bool              `yaml:"no_i2c_address_conflict"`
}

type pullupOhmsYAML struct {
	Min *float64 `yaml:"min"`
	Max *float64 `yaml:"max"`
}

type currentBudgetYAML struct {
	MaxUtilizationPct *float64 `yaml:"max_utilization_pct"`
}

// LoadYAMLFile parses and validates a v1 project contracts file.
func LoadYAMLFile(path string) ([]SystemContract, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return nil, errors.New("contracts: path must not be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contracts yaml: %w", err)
	}
	contracts, err := ParseYAML(data, path)
	if err != nil {
		return nil, err
	}
	return contracts, nil
}

// ParseYAML parses and validates v1 project contracts from bytes.
func ParseYAML(data []byte, path string) ([]SystemContract, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	// Unknown fields should fail early; otherwise a misspelled policy key could
	// look valid while silently doing nothing.
	decoder.KnownFields(true)

	var file contractsYAMLFile
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse contracts yaml: %w", err)
	}
	if err := validateContractsYAML(file); err != nil {
		return nil, err
	}
	return normalizeContractsYAML(file, filepath.Clean(strings.TrimSpace(path))), nil
}

// validateContractsYAML checks required fields and duplicate contract IDs.
func validateContractsYAML(file contractsYAMLFile) error {
	if file.Contracts == nil {
		return errors.New("contracts: top-level contracts list is required")
	}
	// Contract IDs become report provenance, so duplicates would make findings
	// hard to trace back to one YAML entry.
	seenIDs := map[string]struct{}{}
	for i, contract := range file.Contracts {
		prefix := fmt.Sprintf("contracts[%d]", i)
		id := strings.TrimSpace(contract.ID)
		if id == "" {
			return fmt.Errorf("%s.id must not be empty", prefix)
		}
		if _, ok := seenIDs[id]; ok {
			return fmt.Errorf("%s.id %q is duplicated", prefix, id)
		}
		seenIDs[id] = struct{}{}
		if _, ok := normalizeYAMLSeverity(contract.Severity); !ok {
			return fmt.Errorf("%s.severity must be one of error, warn, info", prefix)
		}
		if err := validateScopeYAML(prefix+".scope", contract.Scope); err != nil {
			return err
		}
		if err := validateRequirementYAML(prefix+".require", contract.Require); err != nil {
			return err
		}
	}
	return nil
}

// validateScopeYAML rejects whitespace-only or padded scope values.
func validateScopeYAML(prefix string, scope scopeYAML) error {
	for name, value := range map[string]string{
		"bus_type":       scope.BusType,
		"component_type": scope.ComponentType,
		"component_ref":  scope.ComponentRef,
		"net":            scope.Net,
		"rail":           scope.Rail,
	} {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("%s.%s must not have leading or trailing whitespace", prefix, name)
		}
	}
	return nil
}

// validateRequirementYAML checks that at least one valid requirement is active.
func validateRequirementYAML(prefix string, req requirementYAML) error {
	count := 0
	if req.CommonGround != nil && *req.CommonGround {
		count++
	}
	if req.PullupOhms != nil {
		count++
		if req.PullupOhms.Min == nil && req.PullupOhms.Max == nil {
			return fmt.Errorf("%s.pullup_ohms must set min or max", prefix)
		}
		if req.PullupOhms.Min != nil && *req.PullupOhms.Min <= 0 {
			return fmt.Errorf("%s.pullup_ohms.min must be > 0", prefix)
		}
		if req.PullupOhms.Max != nil && *req.PullupOhms.Max <= 0 {
			return fmt.Errorf("%s.pullup_ohms.max must be > 0", prefix)
		}
		if req.PullupOhms.Min != nil && req.PullupOhms.Max != nil && *req.PullupOhms.Min > *req.PullupOhms.Max {
			return fmt.Errorf("%s.pullup_ohms.min must be <= max", prefix)
		}
	}
	if req.VoltageCompatible != nil && *req.VoltageCompatible {
		count++
	}
	if req.CurrentBudget != nil {
		count++
		if req.CurrentBudget.MaxUtilizationPct == nil {
			return fmt.Errorf("%s.current_budget.max_utilization_pct is required", prefix)
		}
		if *req.CurrentBudget.MaxUtilizationPct <= 0 || *req.CurrentBudget.MaxUtilizationPct > 100 {
			return fmt.Errorf("%s.current_budget.max_utilization_pct must be > 0 and <= 100", prefix)
		}
	}
	if req.NoI2CAddressConflict != nil && *req.NoI2CAddressConflict {
		count++
	}
	if count == 0 {
		return fmt.Errorf("%s must set at least one requirement", prefix)
	}
	return nil
}

// normalizeContractsYAML converts public YAML contracts into SystemContract values.
func normalizeContractsYAML(file contractsYAMLFile, path string) []SystemContract {
	out := make([]SystemContract, 0, len(file.Contracts))
	for _, raw := range file.Contracts {
		id := strings.TrimSpace(raw.ID)
		severity, _ := normalizeYAMLSeverity(raw.Severity)
		scope := ContractScope{
			BusType:       strings.TrimSpace(raw.Scope.BusType),
			ComponentType: strings.TrimSpace(raw.Scope.ComponentType),
			ComponentRef:  strings.TrimSpace(raw.Scope.ComponentRef),
			Net:           strings.TrimSpace(raw.Scope.Net),
			Rail:          strings.TrimSpace(raw.Scope.Rail),
		}
		provenance := Provenance{
			Source:   userYAMLSourceName,
			SourceID: id,
			Detail:   path,
		}
		// Once normalized, user YAML follows the same SystemContract path as the
		// built-in catalog.
		contract := SystemContract{
			ID:           id,
			Description:  strings.TrimSpace(raw.Description),
			Scope:        scope,
			Requirements: normalizeRequirementsYAML(id, scope, severity, path, provenance, raw.Require),
			SourceKind:   ContractSourceUserYAML,
			ContractFile: path,
			Provenance:   provenance,
		}
		out = append(out, contract)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// normalizeRequirementsYAML expands one YAML require block into requirements.
func normalizeRequirementsYAML(id string, scope ContractScope, severity string, path string, provenance Provenance, raw requirementYAML) []Requirement {
	reqs := make([]Requirement, 0, 5)
	add := func(req Requirement) {
		// Copy common provenance into every requirement so findings can point
		// back to the exact YAML contract entry.
		req.Scope = scope
		req.Severity = severity
		req.ContractID = id
		req.ContractSource = ContractSourceUserYAML
		req.ContractFile = path
		req.Provenance = provenance
		reqs = append(reqs, req)
	}
	if raw.CommonGround != nil && *raw.CommonGround {
		add(Requirement{
			Type: ContractCommonGround,
			Fix:  "Connect all scoped components to a shared ground net.",
		})
	}
	if raw.PullupOhms != nil {
		add(Requirement{
			Type:    ContractPullupOhms,
			MinOhms: cloneFloat(raw.PullupOhms.Min),
			MaxOhms: cloneFloat(raw.PullupOhms.Max),
			Fix:     "Add or resize pull-up resistors so the effective pull-up resistance is within the contract range.",
		})
	}
	if raw.VoltageCompatible != nil && *raw.VoltageCompatible {
		add(Requirement{
			Type: ContractVoltageCompatible,
			Fix:  "Use a compatible rail voltage or add level shifting.",
		})
	}
	if raw.CurrentBudget != nil {
		add(Requirement{
			Type:              ContractCurrentBudget,
			MaxUtilizationPct: cloneFloat(raw.CurrentBudget.MaxUtilizationPct),
			Fix:               "Reduce rail load or choose a supply with a larger current rating.",
		})
	}
	if raw.NoI2CAddressConflict != nil && *raw.NoI2CAddressConflict {
		add(Requirement{
			Type: ContractNoI2CAddressConflict,
			Fix:  "Assign unique I2C addresses or isolate devices with a bus multiplexer.",
		})
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].Type < reqs[j].Type })
	return reqs
}

// normalizeYAMLSeverity maps YAML severity to report severity spelling.
func normalizeYAMLSeverity(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "error":
		return "ERROR", true
	case "warn":
		return "WARN", true
	case "info":
		return "INFO", true
	default:
		return "", false
	}
}

// UserYAMLSource adapts already validated project contracts into ContractIR.
type UserYAMLSource struct {
	path      string
	contracts []SystemContract
}

// NewUserYAMLSource wraps loaded YAML contracts as a ContractSource.
func NewUserYAMLSource(path string, loaded []SystemContract) UserYAMLSource {
	return UserYAMLSource{
		path:      filepath.Clean(strings.TrimSpace(path)),
		contracts: cloneSystemContracts(loaded),
	}
}

// Name returns the stable source name used in provenance.
func (s UserYAMLSource) Name() string {
	return userYAMLSourceName
}

// Enrich adds user YAML requirements to ContractIR.
func (s UserYAMLSource) Enrich(_ *ir.DesignIR) (*ContractIR, error) {
	out := NewContractIR()
	contracts := cloneSystemContracts(s.contracts)
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].ID < contracts[j].ID })
	for _, contract := range contracts {
		contractID := strings.TrimSpace(contract.ID)
		if contractID == "" {
			continue
		}
		for _, req := range contract.Requirements {
			// Binding happens here rather than in the evaluator so all contract
			// sources present the same AppliedRequirement shape.
			req.Scope = mergeRequirementScope(contract.Scope, req.Scope)
			req.ContractID = contractID
			req.ContractSource = ContractSourceUserYAML
			if strings.TrimSpace(req.ContractFile) == "" {
				req.ContractFile = s.path
			}
			if strings.TrimSpace(req.Provenance.Source) == "" {
				req.Provenance = contract.Provenance
			}
			out.PutAppliedRequirement(AppliedRequirement{
				Requirement:  req,
				ComponentRef: req.Scope.ComponentRef,
				Source:       s.Name(),
				Provenance:   req.Provenance,
			})
		}
	}
	return out, nil
}

// mergeRequirementScope lets per-requirement scope inherit contract scope.
func mergeRequirementScope(parent ContractScope, child ContractScope) ContractScope {
	if child.BusType == "" {
		child.BusType = parent.BusType
	}
	if child.ComponentType == "" {
		child.ComponentType = parent.ComponentType
	}
	if child.ComponentRef == "" {
		child.ComponentRef = parent.ComponentRef
	}
	if child.Net == "" {
		child.Net = parent.Net
	}
	if child.Rail == "" {
		child.Rail = parent.Rail
	}
	if len(child.Pins) == 0 {
		child.Pins = cloneStrings(parent.Pins)
	}
	if child.Role == "" {
		child.Role = parent.Role
	}
	if child.MPN == "" {
		child.MPN = parent.MPN
	}
	return child
}
