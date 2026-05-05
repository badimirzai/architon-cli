package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func runPartsCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newPartsCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func TestPartsList(t *testing.T) {
	stdout, err := runPartsCommand(t, "list")
	if err != nil {
		t.Fatalf("expected parts list to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "ESP32-WROOM-32") {
		t.Fatalf("expected ESP32-WROOM-32 in parts list, got %q", stdout)
	}
	if !strings.Contains(stdout, "TXS0108E") {
		t.Fatalf("expected TXS0108E in parts list, got %q", stdout)
	}
}

func TestPartsShowESP32(t *testing.T) {
	stdout, err := runPartsCommand(t, "show", "ESP32-WROOM-32")
	if err != nil {
		t.Fatalf("expected parts show to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "MPN: ESP32-WROOM-32") {
		t.Fatalf("expected MPN line, got %q", stdout)
	}
	if !strings.Contains(stdout, "supply_abs_max") {
		t.Fatalf("expected supply_abs_max contract, got %q", stdout)
	}
}
