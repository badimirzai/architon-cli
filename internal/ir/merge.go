package ir

import (
	"sort"
	"time"
)

// MergeProjectIR combines BOM and netlist imports into a single project-level DesignIR.
func MergeProjectIR(bom *DesignIR, netlist *DesignIR, projectPath string, now time.Time) *DesignIR {
	merged := &DesignIR{
		Version: SchemaVersion,
		Source:  "kicad_project",
		Metadata: IRMetadata{
			InputFile: projectPath,
			ParsedAt:  now.UTC().Format(time.RFC3339),
		},
	}

	if bom != nil {
		merged.Metadata.Delimiter = bom.Metadata.Delimiter
		merged.ParseErrors = append(merged.ParseErrors, bom.ParseErrors...)
		merged.ParseWarnings = append(merged.ParseWarnings, bom.ParseWarnings...)
	}
	if netlist != nil {
		merged.ParseErrors = append(merged.ParseErrors, netlist.ParseErrors...)
		merged.ParseWarnings = append(merged.ParseWarnings, netlist.ParseWarnings...)
	}

	partIndex := make(map[string]int)
	if bom != nil {
		merged.Parts = make([]Part, 0, len(bom.Parts)+netlistParts(netlist))
		for _, part := range bom.Parts {
			cloned := clonePart(part)
			partIndex[cloned.Ref] = len(merged.Parts)
			merged.Parts = append(merged.Parts, cloned)
		}
	}

	if netlist != nil {
		for _, part := range netlist.Parts {
			cloned := clonePart(part)
			if idx, ok := partIndex[cloned.Ref]; ok {
				if merged.Parts[idx].Value == "" && cloned.Value != "" {
					merged.Parts[idx].Value = cloned.Value
				}
				if merged.Parts[idx].Footprint == "" && cloned.Footprint != "" {
					merged.Parts[idx].Footprint = cloned.Footprint
				}
				continue
			}
			partIndex[cloned.Ref] = len(merged.Parts)
			merged.Parts = append(merged.Parts, cloned)
		}
		merged.Nets = cloneNets(netlist.Nets)
	}

	sort.Slice(merged.Parts, func(i, j int) bool {
		if merged.Parts[i].Ref != merged.Parts[j].Ref {
			return merged.Parts[i].Ref < merged.Parts[j].Ref
		}
		if merged.Parts[i].Value != merged.Parts[j].Value {
			return merged.Parts[i].Value < merged.Parts[j].Value
		}
		return merged.Parts[i].Footprint < merged.Parts[j].Footprint
	})
	sort.Slice(merged.Nets, func(i, j int) bool {
		if merged.Nets[i].Name != merged.Nets[j].Name {
			return merged.Nets[i].Name < merged.Nets[j].Name
		}
		return len(merged.Nets[i].Pins) < len(merged.Nets[j].Pins)
	})
	for i := range merged.Nets {
		sort.Slice(merged.Nets[i].Pins, func(left, right int) bool {
			if merged.Nets[i].Pins[left].Ref != merged.Nets[i].Pins[right].Ref {
				return merged.Nets[i].Pins[left].Ref < merged.Nets[i].Pins[right].Ref
			}
			return merged.Nets[i].Pins[left].Pin < merged.Nets[i].Pins[right].Pin
		})
	}

	if merged.Version == "" {
		merged.Version = SchemaVersion
	}

	return merged
}

func clonePart(part Part) Part {
	cloned := part
	if len(part.Fields) == 0 {
		cloned.Fields = nil
		return cloned
	}
	cloned.Fields = make(map[string]string, len(part.Fields))
	for key, value := range part.Fields {
		cloned.Fields[key] = value
	}
	return cloned
}

func cloneNets(nets []Net) []Net {
	if len(nets) == 0 {
		return nil
	}
	cloned := make([]Net, len(nets))
	for i, net := range nets {
		cloned[i].Name = net.Name
		if len(net.Pins) == 0 {
			continue
		}
		cloned[i].Pins = make([]PinRef, len(net.Pins))
		copy(cloned[i].Pins, net.Pins)
	}
	return cloned
}

func netlistParts(netlist *DesignIR) int {
	if netlist == nil {
		return 0
	}
	return len(netlist.Parts)
}
