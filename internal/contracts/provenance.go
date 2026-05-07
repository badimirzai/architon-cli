package contracts

import "strings"

// Provenance records where a deterministic contract or finding came from.
type Provenance struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func mergeProvenance(primary Provenance, fallback Provenance) Provenance {
	if strings.TrimSpace(primary.Source) == "" {
		primary.Source = fallback.Source
	}
	if strings.TrimSpace(primary.SourceID) == "" {
		primary.SourceID = fallback.SourceID
	}
	if strings.TrimSpace(primary.Detail) == "" {
		primary.Detail = fallback.Detail
	}
	return primary
}

// ReportContractSource maps internal source names to the v0.4 report enum.
func ReportContractSource(source string) ContractSourceKind {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "built-in", "built_in":
		return ContractSourceBuiltIn
	case "user-yaml", "user_yaml", "contracts.yaml":
		return ContractSourceUserYAML
	case "meta.yaml", "meta_yaml":
		return ContractSourceMetaYAML
	default:
		return ContractSourceInferred
	}
}
