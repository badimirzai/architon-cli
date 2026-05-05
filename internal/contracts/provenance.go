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
