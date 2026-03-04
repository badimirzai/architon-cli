package kicad

import "testing"

func TestImportKiCadNetlist_ParsesPartsAndNets(t *testing.T) {
	design, err := ImportKiCadNetlist(fixturePath(t, "netlist_simple.net"))
	if err != nil {
		t.Fatalf("ImportKiCadNetlist returned error: %v", err)
	}

	if design.Source != "kicad_netlist_sexpr" {
		t.Fatalf("expected source kicad_netlist_sexpr, got %q", design.Source)
	}
	if len(design.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(design.Parts))
	}
	if len(design.Nets) != 2 {
		t.Fatalf("expected 2 nets, got %d", len(design.Nets))
	}
	if design.Parts[0].Ref != "C1" || design.Parts[0].Value != "" {
		t.Fatalf("unexpected first part: %+v", design.Parts[0])
	}
	if design.Parts[1].Ref != "R1" || design.Parts[1].Value != "10k" || design.Parts[1].Footprint != "" {
		t.Fatalf("unexpected second part: %+v", design.Parts[1])
	}
	if design.Parts[2].Ref != "U1" || design.Parts[2].Footprint == "" {
		t.Fatalf("unexpected third part: %+v", design.Parts[2])
	}
	if design.Nets[0].Name != "/VBUS" || len(design.Nets[0].Pins) != 2 {
		t.Fatalf("unexpected first net: %+v", design.Nets[0])
	}
	if design.Nets[1].Name != "GND" || len(design.Nets[1].Pins) != 3 {
		t.Fatalf("unexpected second net: %+v", design.Nets[1])
	}
}

func TestImportKiCadNetlist_SortsDeterministically(t *testing.T) {
	design, err := ImportKiCadNetlist(fixturePath(t, "netlist_simple.net"))
	if err != nil {
		t.Fatalf("ImportKiCadNetlist returned error: %v", err)
	}

	gotRefs := []string{design.Parts[0].Ref, design.Parts[1].Ref, design.Parts[2].Ref}
	wantRefs := []string{"C1", "R1", "U1"}
	for i := range wantRefs {
		if gotRefs[i] != wantRefs[i] {
			t.Fatalf("expected refs %v, got %v", wantRefs, gotRefs)
		}
	}

	gotNets := []string{design.Nets[0].Name, design.Nets[1].Name}
	wantNets := []string{"/VBUS", "GND"}
	for i := range wantNets {
		if gotNets[i] != wantNets[i] {
			t.Fatalf("expected nets %v, got %v", wantNets, gotNets)
		}
	}

	gotPins := []string{
		design.Nets[1].Pins[0].Ref + ":" + design.Nets[1].Pins[0].Pin,
		design.Nets[1].Pins[1].Ref + ":" + design.Nets[1].Pins[1].Pin,
		design.Nets[1].Pins[2].Ref + ":" + design.Nets[1].Pins[2].Pin,
	}
	wantPins := []string{"C1:2", "R1:1", "U1:5"}
	for i := range wantPins {
		if gotPins[i] != wantPins[i] {
			t.Fatalf("expected pins %v, got %v", wantPins, gotPins)
		}
	}
}

func TestImportKiCadNetlist_InvalidFixtureReturnsError(t *testing.T) {
	_, err := ImportKiCadNetlist(fixturePath(t, "netlist_invalid.net"))
	if err == nil {
		t.Fatal("expected error")
	}
}
