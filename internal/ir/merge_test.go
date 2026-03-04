package ir

import (
	"testing"
	"time"
)

func TestMergeProjectIR_MergesBOMAndNetlistDeterministically(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	bom := &DesignIR{
		Version: "0",
		Source:  "kicad_bom_csv",
		Parts: []Part{
			{
				Ref:       "R1",
				Value:     "",
				Footprint: "",
				Fields: map[string]string{
					"Reference": "R1",
					"MPN":       "RC0603",
				},
			},
			{
				Ref:       "C1",
				Value:     "100nF",
				Footprint: "Capacitor_SMD:C_0603_1608Metric",
				Fields: map[string]string{
					"Reference": "C1",
				},
			},
		},
		Metadata: IRMetadata{
			InputFile: "bom.csv",
			ParsedAt:  now.Add(-time.Hour).Format(time.RFC3339),
			Delimiter: ",",
		},
		ParseWarnings: []string{"warning"},
	}
	netlist := &DesignIR{
		Version: "0",
		Source:  "kicad_netlist_sexpr",
		Parts: []Part{
			{Ref: "U1", Value: "MCU", Footprint: "Package_QFP:LQFP-48"},
			{Ref: "R1", Value: "10k", Footprint: "Resistor_SMD:R_0603_1608Metric"},
		},
		Nets: []Net{
			{
				Name: "GND",
				Pins: []PinRef{
					{Ref: "U1", Pin: "5"},
					{Ref: "C1", Pin: "2"},
				},
			},
			{
				Name: "/VBUS",
				Pins: []PinRef{
					{Ref: "U1", Pin: "15"},
				},
			},
		},
	}

	merged := MergeProjectIR(bom, netlist, "/tmp/project", now)

	if merged.Source != "kicad_project" {
		t.Fatalf("expected source kicad_project, got %q", merged.Source)
	}
	if merged.Metadata.InputFile != "/tmp/project" {
		t.Fatalf("expected input file /tmp/project, got %q", merged.Metadata.InputFile)
	}
	if merged.Metadata.ParsedAt != now.Format(time.RFC3339) {
		t.Fatalf("expected parsed_at %q, got %q", now.Format(time.RFC3339), merged.Metadata.ParsedAt)
	}
	if merged.Metadata.Delimiter != "," {
		t.Fatalf("expected delimiter to be preserved from BOM, got %q", merged.Metadata.Delimiter)
	}
	if len(merged.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(merged.Parts))
	}
	if merged.Parts[0].Ref != "C1" || merged.Parts[1].Ref != "R1" || merged.Parts[2].Ref != "U1" {
		t.Fatalf("unexpected merged part order: %+v", merged.Parts)
	}
	if merged.Parts[1].Value != "10k" {
		t.Fatalf("expected R1 value to be filled from netlist, got %q", merged.Parts[1].Value)
	}
	if merged.Parts[1].Footprint != "Resistor_SMD:R_0603_1608Metric" {
		t.Fatalf("expected R1 footprint to be filled from netlist, got %q", merged.Parts[1].Footprint)
	}
	if merged.Parts[1].Fields["MPN"] != "RC0603" {
		t.Fatalf("expected BOM fields to be preserved, got %+v", merged.Parts[1].Fields)
	}
	if len(merged.Nets) != 2 {
		t.Fatalf("expected 2 nets, got %d", len(merged.Nets))
	}
	if merged.Nets[0].Name != "/VBUS" || merged.Nets[1].Name != "GND" {
		t.Fatalf("unexpected net order: %+v", merged.Nets)
	}
	if merged.Nets[1].Pins[0].Ref != "C1" || merged.Nets[1].Pins[1].Ref != "U1" {
		t.Fatalf("expected net pins to be sorted, got %+v", merged.Nets[1].Pins)
	}
	if len(merged.ParseWarnings) != 1 || merged.ParseWarnings[0] != "warning" {
		t.Fatalf("expected BOM diagnostics to be preserved, got %+v", merged.ParseWarnings)
	}
}
