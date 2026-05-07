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
	BusType       string       `yaml:"bus_type"`
	BusID         string       `yaml:"bus_id"`
	ComponentType string       `yaml:"component_type"`
	ComponentRef  string       `yaml:"component_ref"`
	Net           string       `yaml:"net"`
	Rail          string       `yaml:"rail"`
	Nets          *i2cNetsYAML `yaml:"nets"`
}

type i2cNetsYAML struct {
	SDA string `yaml:"sda"`
	SCL string `yaml:"scl"`
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
	var root yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&root); err != nil {
		return nil, fmt.Errorf("parse contracts yaml: %w", err)
	}
	if err := validateContractsYAMLNode(&root); err != nil {
		return nil, err
	}

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

// validateContractsYAMLNode enforces required fields and unknown-key checks
// before typed decoding so missing maps do not collapse into zero values.
func validateContractsYAMLNode(root *yaml.Node) error {
	if root == nil || root.Kind == 0 {
		return errors.New("contracts: document must not be empty")
	}
	doc := root
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return errors.New("contracts: document must not be empty")
		}
		doc = root.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return errors.New("contracts: document must be a mapping")
	}
	top, err := mappingFromNode(doc, "contracts")
	if err != nil {
		return err
	}
	for _, key := range sortedMappingKeys(top) {
		if key != "contracts" {
			return fmt.Errorf("contracts: unknown top-level field %q", key)
		}
	}
	contractsNode := top["contracts"]
	if contractsNode == nil {
		return errors.New("contracts: top-level contracts list is required")
	}
	if contractsNode.Kind != yaml.SequenceNode {
		return errors.New("contracts: top-level contracts must be a list")
	}

	for i, node := range contractsNode.Content {
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("Invalid contract contracts[%d]: contract entry must be a mapping", i)
		}
		entries, err := mappingFromNode(node, fmt.Sprintf("contracts[%d]", i))
		if err != nil {
			return err
		}
		id := contractIDFromNode(entries["id"])
		label := contractValidationLabel(i, id)
		for _, key := range sortedMappingKeys(entries) {
			switch key {
			case "id", "description", "scope", "require", "severity":
			default:
				return contractValidationError(label, "unknown field %q", key)
			}
		}
		for _, key := range []string{"id", "severity", "scope", "require"} {
			if entries[key] == nil {
				return contractValidationError(label, "%s is required", key)
			}
		}
		if err := validateScopeYAMLNode(label, entries["scope"]); err != nil {
			return err
		}
		if err := validateRequirementYAMLNode(label, entries["require"]); err != nil {
			return err
		}
	}
	return nil
}

func validateScopeYAMLNode(label string, node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return contractValidationError(label, "scope must be an object")
	}
	entries, err := mappingFromNode(node, label+".scope")
	if err != nil {
		return err
	}
	for _, key := range sortedMappingKeys(entries) {
		switch key {
		case "bus_type", "bus_id", "component_type", "component_ref", "net", "rail", "nets":
		default:
			return contractValidationError(label, "unknown scope key %q", key)
		}
	}
	if netsNode := entries["nets"]; netsNode != nil {
		if netsNode.Kind != yaml.MappingNode {
			return contractValidationError(label, "scope.nets must be an object")
		}
		nets, err := mappingFromNode(netsNode, label+".scope.nets")
		if err != nil {
			return err
		}
		for _, key := range sortedMappingKeys(nets) {
			switch key {
			case "sda", "scl":
				value := nets[key]
				if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
					return contractValidationError(label, "scope.nets.%s must be a non-empty string", key)
				}
			default:
				return contractValidationError(label, "unknown scope.nets key %q", key)
			}
		}
	}
	return nil
}

func validateRequirementYAMLNode(label string, node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return contractValidationError(label, "require must be an object")
	}
	entries, err := mappingFromNode(node, label+".require")
	if err != nil {
		return err
	}
	for _, key := range sortedMappingKeys(entries) {
		value := entries[key]
		switch key {
		case "common_ground", "pullup_ohms", "voltage_compatible", "current_budget", "no_i2c_address_conflict":
		default:
			return contractValidationError(label, "unknown requirement key %q", key)
		}
		if key == "pullup_ohms" {
			if value.Kind != yaml.MappingNode {
				return contractValidationError(label, "pullup_ohms must be an object")
			}
			pullup, err := mappingFromNode(value, label+".require.pullup_ohms")
			if err != nil {
				return err
			}
			for _, nested := range sortedMappingKeys(pullup) {
				if nested != "min" && nested != "max" {
					return contractValidationError(label, "unknown pullup_ohms key %q", nested)
				}
			}
		}
		if key == "current_budget" {
			if value.Kind != yaml.MappingNode {
				return contractValidationError(label, "current_budget must be an object")
			}
			budget, err := mappingFromNode(value, label+".require.current_budget")
			if err != nil {
				return err
			}
			for _, nested := range sortedMappingKeys(budget) {
				if nested != "max_utilization_pct" {
					return contractValidationError(label, "unknown current_budget key %q", nested)
				}
			}
		}
	}
	return nil
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
		id := contract.ID
		label := contractValidationLabel(i, strings.TrimSpace(id))
		if id == "" {
			return contractValidationError(label, "id must not be empty")
		}
		if strings.TrimSpace(id) != id {
			return contractValidationError(label, "id must not have leading or trailing whitespace")
		}
		if !validContractID(id) {
			return contractValidationError(label, "id must match ^[a-zA-Z0-9_.:-]+$")
		}
		if _, ok := seenIDs[id]; ok {
			return contractValidationError(label, "id %q is duplicated", id)
		}
		seenIDs[id] = struct{}{}
		if _, ok := normalizeYAMLSeverity(contract.Severity); !ok {
			return contractValidationError(label, "severity must be one of error, warn, info")
		}
		if err := validateScopeYAML(label, contract.Scope); err != nil {
			return err
		}
		if err := validateRequirementYAML(label, contract.Require); err != nil {
			return err
		}
	}
	return nil
}

// validateScopeYAML rejects whitespace-only or padded scope values.
func validateScopeYAML(label string, scope scopeYAML) error {
	selectors := 0
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "bus_type", value: scope.BusType},
		{name: "bus_id", value: scope.BusID},
		{name: "component_type", value: scope.ComponentType},
		{name: "component_ref", value: scope.ComponentRef},
		{name: "net", value: scope.Net},
		{name: "rail", value: scope.Rail},
	} {
		name := field.name
		value := field.value
		if strings.TrimSpace(value) != value {
			return contractValidationError(label, "scope.%s must not have leading or trailing whitespace", name)
		}
		if value != "" {
			selectors++
		}
	}
	if scope.BusType != "" && !strings.EqualFold(scope.BusType, "i2c") {
		return contractValidationError(label, "scope.bus_type must be i2c")
	}
	if scope.Nets != nil {
		selectors++
		if strings.TrimSpace(scope.Nets.SDA) != scope.Nets.SDA {
			return contractValidationError(label, "scope.nets.sda must not have leading or trailing whitespace")
		}
		if strings.TrimSpace(scope.Nets.SCL) != scope.Nets.SCL {
			return contractValidationError(label, "scope.nets.scl must not have leading or trailing whitespace")
		}
		if scope.Nets.SDA == "" || scope.Nets.SCL == "" {
			return contractValidationError(label, "scope.nets.sda and scope.nets.scl are required when scope.nets is present")
		}
	}
	if selectors == 0 {
		return contractValidationError(label, "scope must set at least one selector")
	}
	return nil
}

// validateRequirementYAML checks that at least one valid requirement is active.
func validateRequirementYAML(label string, req requirementYAML) error {
	count := 0
	if req.CommonGround != nil && *req.CommonGround {
		count++
	}
	if req.PullupOhms != nil {
		count++
		if req.PullupOhms.Min == nil && req.PullupOhms.Max == nil {
			return contractValidationError(label, "pullup_ohms must set min or max")
		}
		if req.PullupOhms.Min != nil && *req.PullupOhms.Min <= 0 {
			return contractValidationError(label, "pullup_ohms.min must be > 0")
		}
		if req.PullupOhms.Max != nil && *req.PullupOhms.Max <= 0 {
			return contractValidationError(label, "pullup_ohms.max must be > 0")
		}
		if req.PullupOhms.Min != nil && req.PullupOhms.Max != nil && *req.PullupOhms.Min > *req.PullupOhms.Max {
			return contractValidationError(label, "pullup_ohms.min must be <= pullup_ohms.max")
		}
	}
	if req.VoltageCompatible != nil && *req.VoltageCompatible {
		count++
	}
	if req.CurrentBudget != nil {
		count++
		if req.CurrentBudget.MaxUtilizationPct == nil {
			return contractValidationError(label, "current_budget.max_utilization_pct is required")
		}
		if *req.CurrentBudget.MaxUtilizationPct <= 0 || *req.CurrentBudget.MaxUtilizationPct > 100 {
			return contractValidationError(label, "current_budget.max_utilization_pct must be > 0 and <= 100")
		}
	}
	if req.NoI2CAddressConflict != nil && *req.NoI2CAddressConflict {
		count++
	}
	if count == 0 {
		return contractValidationError(label, "require must set at least one enabled requirement")
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
			BusID:         strings.TrimSpace(raw.Scope.BusID),
			ComponentType: strings.TrimSpace(raw.Scope.ComponentType),
			ComponentRef:  strings.TrimSpace(raw.Scope.ComponentRef),
			Net:           strings.TrimSpace(raw.Scope.Net),
			Rail:          strings.TrimSpace(raw.Scope.Rail),
		}
		if raw.Scope.Nets != nil {
			scope.Nets = &I2CBusNets{
				SDA: strings.TrimSpace(raw.Scope.Nets.SDA),
				SCL: strings.TrimSpace(raw.Scope.Nets.SCL),
			}
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

func mappingFromNode(node *yaml.Node, label string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", label)
	}
	out := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s contains a non-scalar key", label)
		}
		key := strings.TrimSpace(keyNode.Value)
		if key == "" {
			return nil, fmt.Errorf("%s contains an empty key", label)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("%s contains duplicate key %q", label, key)
		}
		out[key] = valueNode
	}
	return out, nil
}

func sortedMappingKeys(values map[string]*yaml.Node) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contractIDFromNode(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func contractValidationLabel(index int, id string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf("contracts[%d]", index)
}

func contractValidationError(label string, format string, args ...any) error {
	return fmt.Errorf("Invalid contract %s: %s", label, fmt.Sprintf(format, args...))
}

func validContractID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '_', '.', ':', '-':
			continue
		default:
			return false
		}
	}
	return true
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
	if child.BusID == "" {
		child.BusID = parent.BusID
	}
	if child.Nets == nil {
		child.Nets = cloneI2CBusNets(parent.Nets)
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
