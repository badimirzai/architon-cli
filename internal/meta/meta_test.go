package meta

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return p
}

func TestParse_AllowsSkeletonValues(t *testing.T) {
	dir := t.TempDir()

	path := writeTempFile(t, dir, "meta.yaml", `
version: "0"
sources:
  - net: VBAT
    voltage: 0
regulators: []
components:
  - ref: U1
    max_voltage: 0
`)

	m, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if m.Version != "0" {
		t.Fatalf("expected version 0, got %q", m.Version)
	}
	if IsConfigured(m) {
		t.Fatalf("expected IsConfigured=false for skeleton values")
	}
}

func TestValidateStrict_AllowsMissingSources(t *testing.T) {
	m := &Meta{
		Version:    "0",
		Sources:    nil,
		Regulators: nil,
		Components: []Component{{Ref: "U1", MaxVoltage: 5.0}},
	}
	if err := ValidateStrict(m); err != nil {
		t.Fatalf("expected missing sources to be valid when voltages can be inferred, got %v", err)
	}
}

func TestValidateStrict_RejectsZeroVoltageSource(t *testing.T) {
	m := &Meta{
		Version: "0",
		Sources: []Source{{Net: "VBAT", Voltage: 0}},
		Components: []Component{
			{Ref: "U1", MaxVoltage: 5.0},
		},
	}
	if err := ValidateStrict(m); err == nil {
		t.Fatalf("expected strict validation error for source voltage")
	}
}

func TestValidateStrict_RejectsZeroMaxVoltage(t *testing.T) {
	m := &Meta{
		Version: "0",
		Sources: []Source{{Net: "VBAT", Voltage: 12.0}},
		Components: []Component{
			{Ref: "U1", MaxVoltage: 0},
		},
	}
	if err := ValidateStrict(m); err == nil {
		t.Fatalf("expected strict validation error for max_voltage")
	}
}

func TestIsConfigured_TrueWhenSourceAndComponentSet(t *testing.T) {
	m := &Meta{
		Version: "0",
		Sources: []Source{{Net: "VBAT", Voltage: 12.0}},
		Components: []Component{
			{Ref: "U1", MaxVoltage: 5.0},
		},
	}
	if !IsConfigured(m) {
		t.Fatalf("expected IsConfigured=true")
	}
}
