package contracts

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	Connected            *connectedYAML     `yaml:"connected"`
	Terminated           *terminatedYAML    `yaml:"terminated"`
}

type connectedYAML struct {
	Nets         []string          `yaml:"nets"`
	Participants []participantYAML `yaml:"participants"`
	ChipSelects  []chipSelectYAML  `yaml:"chip_selects"`
}

type chipSelectYAML struct {
	Ref string `yaml:"ref"`
	Net string `yaml:"net"`
}

type terminatedYAML struct {
	Nets           []string `yaml:"nets"`
	ResistanceOhms *float64 `yaml:"resistance_ohms"`
	Count          *int     `yaml:"count"`
}

type participantYAML struct {
	Ref  string            `yaml:"ref"`
	Role string            `yaml:"role"`
	Pins map[string]string `yaml:"pins"`
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
		case "common_ground", "pullup_ohms", "voltage_compatible", "current_budget", "no_i2c_address_conflict", "connected", "terminated":
		default:
			return contractValidationError(label, "unknown requirement key %q", key)
		}
		if key == "connected" {
			if err := validateConnectedYAMLNode(label, value); err != nil {
				return err
			}
		}
		if key == "terminated" {
			if err := validateTerminatedYAMLNode(label, value); err != nil {
				return err
			}
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

// validateConnectedYAMLNode checks require.connected shape before typed decoding.
// Allowed keys are nets, participants, and chip_selects. Each participant needs ref and role.
// pins is an optional net-to-pin map. Keys must be net names from connected.nets.
// chip_selects is optional. Each entry names one slave and its chip-select net.
func validateConnectedYAMLNode(label string, node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return contractValidationError(label, "connected must be an object")
	}
	entries, err := mappingFromNode(node, label+".require.connected")
	if err != nil {
		return err
	}
	for _, key := range sortedMappingKeys(entries) {
		switch key {
		case "nets", "participants", "chip_selects":
		default:
			return contractValidationError(label, "unknown connected key %q", key)
		}
	}
	netsNode := entries["nets"]
	if netsNode == nil || netsNode.Kind != yaml.SequenceNode {
		return contractValidationError(label, "connected.nets must be a list")
	}
	for i, item := range netsNode.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || strings.TrimSpace(item.Value) == "" {
			return contractValidationError(label, "connected.nets[%d] must be a non-empty string", i)
		}
	}
	participantsNode := entries["participants"]
	if participantsNode == nil || participantsNode.Kind != yaml.SequenceNode {
		return contractValidationError(label, "connected.participants must be a list")
	}
	for i, item := range participantsNode.Content {
		if item.Kind != yaml.MappingNode {
			return contractValidationError(label, "connected.participants[%d] must be an object", i)
		}
		fields, err := mappingFromNode(item, fmt.Sprintf("%s.require.connected.participants[%d]", label, i))
		if err != nil {
			return err
		}
		for _, key := range sortedMappingKeys(fields) {
			switch key {
			case "ref", "role", "pins":
			default:
				return contractValidationError(label, "unknown connected.participants key %q", key)
			}
		}
		for _, key := range []string{"ref", "role"} {
			value := fields[key]
			if value == nil || value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
				return contractValidationError(label, "connected.participants.%s must be a non-empty string", key)
			}
		}
		if err := validateParticipantPinsYAMLNode(label, fields["pins"]); err != nil {
			return err
		}
	}
	if err := validateChipSelectsYAMLNode(label, entries["chip_selects"]); err != nil {
		return err
	}
	return nil
}

// validateChipSelectsYAMLNode checks the optional chip_selects list.
// Each entry is a slave ref and the net that must be that slave's chip-select.
// The same net on two entries is left for evaluation, which reports spi_cs_shared.
func validateChipSelectsYAMLNode(label string, node *yaml.Node) error {
	if node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return contractValidationError(label, "connected.chip_selects must be a list")
	}
	for i, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return contractValidationError(label, "connected.chip_selects[%d] must be an object", i)
		}
		fields, err := mappingFromNode(item, fmt.Sprintf("%s.require.connected.chip_selects[%d]", label, i))
		if err != nil {
			return err
		}
		for _, key := range sortedMappingKeys(fields) {
			switch key {
			case "ref", "net":
			default:
				return contractValidationError(label, "unknown connected.chip_selects key %q", key)
			}
		}
		for _, key := range []string{"ref", "net"} {
			value := fields[key]
			if value == nil || value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
				return contractValidationError(label, "connected.chip_selects.%s must be a non-empty string", key)
			}
			if strings.TrimSpace(value.Value) != value.Value {
				return contractValidationError(label, "connected.chip_selects.%s must not have leading or trailing whitespace", key)
			}
		}
	}
	return nil
}

// validateTerminatedYAMLNode checks require.terminated before typed decoding.
// A terminator spans exactly two nets, so the net list length is fixed here.
func validateTerminatedYAMLNode(label string, node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return contractValidationError(label, "terminated must be an object")
	}
	entries, err := mappingFromNode(node, label+".require.terminated")
	if err != nil {
		return err
	}
	for _, key := range sortedMappingKeys(entries) {
		switch key {
		case "nets", "resistance_ohms", "count":
		default:
			return contractValidationError(label, "unknown terminated key %q", key)
		}
	}
	netsNode := entries["nets"]
	if netsNode == nil || netsNode.Kind != yaml.SequenceNode {
		return contractValidationError(label, "terminated.nets must be a list")
	}
	if len(netsNode.Content) != 2 {
		return contractValidationError(label, "terminated.nets must name exactly two nets")
	}
	for i, item := range netsNode.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || strings.TrimSpace(item.Value) == "" {
			return contractValidationError(label, "terminated.nets[%d] must be a non-empty string", i)
		}
	}
	ohmsNode := entries["resistance_ohms"]
	if ohmsNode == nil {
		return contractValidationError(label, "terminated.resistance_ohms is required")
	}
	ohms, ok := yamlScalarFloat(ohmsNode)
	if !ok || math.IsNaN(ohms) || math.IsInf(ohms, 0) {
		return contractValidationError(label, "terminated.resistance_ohms must be a number")
	}
	if ohms <= 0 {
		return contractValidationError(label, "terminated.resistance_ohms must be > 0")
	}
	countNode := entries["count"]
	if countNode == nil {
		return contractValidationError(label, "terminated.count is required")
	}
	count, ok := yamlScalarInt(countNode)
	if !ok {
		return contractValidationError(label, "terminated.count must be an integer")
	}
	if count <= 0 {
		return contractValidationError(label, "terminated.count must be > 0")
	}
	return nil
}

// validateParticipantPinsYAMLNode checks the optional pins object before typed decoding.
// Each value must be a non-empty string, so a pin number is written as "42" rather than a bare integer.
// Whether each key is one of connected.nets is checked later, once the net list is decoded.
func validateParticipantPinsYAMLNode(label string, node *yaml.Node) error {
	if node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return contractValidationError(label, "connected.participants.pins must be an object")
	}
	pins, err := mappingFromNode(node, label+".require.connected.participants.pins")
	if err != nil {
		return err
	}
	for _, key := range sortedMappingKeys(pins) {
		if strings.TrimSpace(key) == "" {
			return contractValidationError(label, "connected.participants.pins keys must be non-empty net names")
		}
		value := pins[key]
		if value == nil || value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
			return contractValidationError(label, "connected.participants.pins values must be non-empty strings")
		}
		if strings.TrimSpace(value.Value) != value.Value {
			return contractValidationError(label, "connected.participants.pins values must not have leading or trailing whitespace")
		}
	}
	return nil
}

// connectedRequirementOnly reports whether this contract is an interface topology check.
// I2C requirements still require scope.bus_type i2c. connected and terminated may use spi, uart, or can.
func connectedRequirementOnly(req requirementYAML) bool {
	if req.Connected == nil && req.Terminated == nil {
		return false
	}
	if req.CommonGround != nil && *req.CommonGround {
		return false
	}
	if req.PullupOhms != nil || req.CurrentBudget != nil {
		return false
	}
	if req.VoltageCompatible != nil && *req.VoltageCompatible {
		return false
	}
	if req.NoI2CAddressConflict != nil && *req.NoI2CAddressConflict {
		return false
	}
	return true
}

// validInterfaceBusType accepts a short bus label such as spi, i2c, uart, or can.
func validInterfaceBusType(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '_' || r == '-'):
		default:
			return false
		}
	}
	return true
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
		if err := validateScopeYAML(label, contract.Scope, contract.Require); err != nil {
			return err
		}
		if err := validateRequirementYAML(label, contract.Require); err != nil {
			return err
		}
	}
	return nil
}

// validateScopeYAML rejects whitespace-only or padded scope values.
func validateScopeYAML(label string, scope scopeYAML, req requirementYAML) error {
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
		// Non-i2c bus types are interface labels. Electrical I2C rules stay i2c-only.
		if !connectedRequirementOnly(req) {
			return contractValidationError(label, "scope.bus_type must be i2c")
		}
		if !validInterfaceBusType(scope.BusType) {
			return contractValidationError(label, "scope.bus_type must be a bus name such as spi, i2c, uart, or can")
		}
	}
	if req.Connected != nil || req.Terminated != nil {
		kind := "connected"
		if req.Connected == nil {
			kind = "terminated"
		}
		if strings.TrimSpace(scope.BusType) == "" {
			return contractValidationError(label, "scope.bus_type is required for %s", kind)
		}
		if strings.TrimSpace(scope.BusID) == "" {
			return contractValidationError(label, "scope.bus_id is required for %s", kind)
		}
	}
	if req.Connected != nil && len(req.Connected.ChipSelects) > 0 && !strings.EqualFold(strings.TrimSpace(scope.BusType), "spi") {
		return contractValidationError(label, "connected.chip_selects requires scope.bus_type spi")
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
	if req.Connected != nil {
		count++
		if len(req.Connected.Nets) == 0 {
			return contractValidationError(label, "connected.nets must name at least one net")
		}
		seenNets := map[string]struct{}{}
		for _, net := range req.Connected.Nets {
			if strings.TrimSpace(net) == "" {
				return contractValidationError(label, "connected.nets entries must be non-empty strings")
			}
			if strings.TrimSpace(net) != net {
				return contractValidationError(label, "connected.nets entries must not have leading or trailing whitespace")
			}
			if _, ok := seenNets[net]; ok {
				return contractValidationError(label, "connected.nets entry %q is duplicated", net)
			}
			seenNets[net] = struct{}{}
		}
		if len(req.Connected.Participants) == 0 {
			return contractValidationError(label, "connected.participants must name at least one component")
		}
		seenRefs := map[string]struct{}{}
		slaveRefs := map[string]struct{}{}
		for _, participant := range req.Connected.Participants {
			ref := participant.Ref
			if strings.TrimSpace(ref) == "" {
				return contractValidationError(label, "connected.participants.ref must be a non-empty string")
			}
			if strings.TrimSpace(ref) != ref {
				return contractValidationError(label, "connected.participants.ref must not have leading or trailing whitespace")
			}
			if _, ok := seenRefs[ref]; ok {
				return contractValidationError(label, "connected.participants ref %q is duplicated", ref)
			}
			seenRefs[ref] = struct{}{}
			switch strings.ToLower(strings.TrimSpace(participant.Role)) {
			case "master":
			case "slave":
				slaveRefs[ref] = struct{}{}
			default:
				return contractValidationError(label, "connected.participants.role must be master or slave")
			}
			if err := validateParticipantPins(label, participant.Pins, seenNets); err != nil {
				return err
			}
		}
		if err := validateChipSelectRefs(label, req.Connected.ChipSelects, slaveRefs); err != nil {
			return err
		}
	}
	if req.Terminated != nil {
		count++
		if err := validateTerminatedYAML(label, req.Terminated); err != nil {
			return err
		}
	}
	if count == 0 {
		return contractValidationError(label, "require must set at least one enabled requirement")
	}
	return nil
}

// validateChipSelectRefs checks that each chip-select ref is one slave participant.
// Duplicate nets are valid YAML. Evaluation reports those as spi_cs_shared.
func validateChipSelectRefs(label string, selects []chipSelectYAML, slaveRefs map[string]struct{}) error {
	seenRefs := map[string]struct{}{}
	for _, sel := range selects {
		ref := sel.Ref
		net := sel.Net
		if strings.TrimSpace(ref) == "" || strings.TrimSpace(ref) != ref {
			return contractValidationError(label, "connected.chip_selects.ref must be a non-empty string")
		}
		if strings.TrimSpace(net) == "" || strings.TrimSpace(net) != net {
			return contractValidationError(label, "connected.chip_selects.net must be a non-empty string")
		}
		if _, ok := seenRefs[ref]; ok {
			return contractValidationError(label, "connected.chip_selects ref %q is duplicated", ref)
		}
		seenRefs[ref] = struct{}{}
		if _, ok := slaveRefs[ref]; !ok {
			return contractValidationError(label, "connected.chip_selects ref %q must be a slave participant", ref)
		}
	}
	return nil
}

// validateTerminatedYAML checks the two net names and the numeric bounds after decoding.
func validateTerminatedYAML(label string, raw *terminatedYAML) error {
	if raw == nil {
		return contractValidationError(label, "terminated must be an object")
	}
	if len(raw.Nets) != 2 {
		return contractValidationError(label, "terminated.nets must name exactly two nets")
	}
	seenNets := map[string]struct{}{}
	for _, net := range raw.Nets {
		if strings.TrimSpace(net) == "" {
			return contractValidationError(label, "terminated.nets entries must be non-empty strings")
		}
		if strings.TrimSpace(net) != net {
			return contractValidationError(label, "terminated.nets entries must not have leading or trailing whitespace")
		}
		if _, ok := seenNets[net]; ok {
			return contractValidationError(label, "terminated.nets entry %q is duplicated", net)
		}
		// CANH and /CANH are the same net.
		if _, ok := seenNets[normalizeNetName(net)]; ok {
			return contractValidationError(label, "terminated.nets entry %q is duplicated", net)
		}
		seenNets[net] = struct{}{}
		seenNets[normalizeNetName(net)] = struct{}{}
	}
	if raw.ResistanceOhms == nil || *raw.ResistanceOhms <= 0 || math.IsNaN(*raw.ResistanceOhms) || math.IsInf(*raw.ResistanceOhms, 0) {
		return contractValidationError(label, "terminated.resistance_ohms must be > 0")
	}
	if raw.Count == nil || *raw.Count <= 0 {
		return contractValidationError(label, "terminated.count must be > 0")
	}
	return nil
}

// validateParticipantPins rejects a pins key that is not a name in connected.nets.
// Repeating the same pin token on two nets is allowed here. Evaluation reports that as interface_pin_conflict.
func validateParticipantPins(label string, pins map[string]string, nets map[string]struct{}) error {
	if len(pins) == 0 {
		return nil
	}
	keys := make([]string, 0, len(pins))
	for key := range pins {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "" || strings.TrimSpace(key) != key {
			return contractValidationError(label, "connected.participants.pins keys must be non-empty net names")
		}
		value := pins[key]
		if value == "" || strings.TrimSpace(value) != value {
			return contractValidationError(label, "connected.participants.pins values must be non-empty strings")
		}
		if _, ok := nets[key]; !ok {
			return contractValidationError(label, "connected.participants.pins key %q is not in connected.nets", key)
		}
	}
	return nil
}

// normalizeContractsYAML converts public YAML contracts into SystemContract values.
func normalizeContractsYAML(file contractsYAMLFile, path string) []SystemContract {
	out := make([]SystemContract, 0, len(file.Contracts))
	for _, raw := range file.Contracts {
		id := strings.TrimSpace(raw.ID)
		severity, _ := normalizeYAMLSeverity(raw.Severity)
		busType := strings.TrimSpace(raw.Scope.BusType)
		if busType != "" && !strings.EqualFold(busType, "i2c") {
			busType = strings.ToLower(busType)
		}
		scope := ContractScope{
			BusType:       busType,
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
			Fix:     "Use pull-ups between 2.2k and 10k to the bus voltage rail, commonly 4.7k for a 3.3V I2C bus.",
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
	if raw.Connected != nil {
		// Keep role as a label and nets as schematic names. Pins are copied as written.
		// No peripheral lookup happens here. Chip-select nets stay off the shared net list.
		participants := make([]InterfaceParticipant, 0, len(raw.Connected.Participants))
		for _, participant := range raw.Connected.Participants {
			participants = append(participants, InterfaceParticipant{
				Ref:  strings.TrimSpace(participant.Ref),
				Role: strings.ToLower(strings.TrimSpace(participant.Role)),
				Pins: cloneStringMap(participant.Pins),
			})
		}
		nets := make([]string, 0, len(raw.Connected.Nets))
		for _, net := range raw.Connected.Nets {
			nets = append(nets, strings.TrimSpace(net))
		}
		var chipSelects []ChipSelect
		if len(raw.Connected.ChipSelects) > 0 {
			chipSelects = make([]ChipSelect, 0, len(raw.Connected.ChipSelects))
			for _, sel := range raw.Connected.ChipSelects {
				chipSelects = append(chipSelects, ChipSelect{
					Ref: strings.TrimSpace(sel.Ref),
					Net: strings.TrimSpace(sel.Net),
				})
			}
		}
		add(Requirement{
			Type:         ContractConnected,
			Nets:         nets,
			Participants: participants,
			ChipSelects:  chipSelects,
			Fix:          "Connect every listed component to every net named by the interface contract.",
		})
	}
	if raw.Terminated != nil {
		nets := make([]string, 0, len(raw.Terminated.Nets))
		for _, net := range raw.Terminated.Nets {
			nets = append(nets, strings.TrimSpace(net))
		}
		add(Requirement{
			Type:            ContractTerminated,
			Nets:            nets,
			ResistanceOhms:  cloneFloat(raw.Terminated.ResistanceOhms),
			TerminatorCount: cloneInt(raw.Terminated.Count),
			Fix:             "Place the required number of two-pin terminators between the named nets.",
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

// yamlScalarFloat accepts a YAML number such as 120 or 120.0. A quoted string is rejected.
func yamlScalarFloat(node *yaml.Node) (float64, bool) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return 0, false
	}
	switch node.Tag {
	case "!!int", "!!float":
	default:
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(node.Value), 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// yamlScalarInt accepts a whole number such as 2. 2.0 is rejected.
func yamlScalarInt(node *yaml.Node) (int, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(node.Value))
	if err != nil {
		return 0, false
	}
	return value, true
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
