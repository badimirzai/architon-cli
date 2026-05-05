package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
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

func TestPartsListJSON(t *testing.T) {
	stdout, err := runPartsCommand(t, "list", "--format", "json")
	if err != nil {
		t.Fatalf("expected parts list json to succeed, got %v", err)
	}
	if strings.Contains(stdout, "Built-in contract parts:") {
		t.Fatalf("expected JSON-only stdout, got %q", stdout)
	}
	var parts []contracts.SystemContract
	if err := json.Unmarshal([]byte(stdout), &parts); err != nil {
		t.Fatalf("expected valid JSON, got %v\n%s", err, stdout)
	}
	if len(parts) == 0 {
		t.Fatal("expected built-in parts")
	}
	if parts[0].MPN == "" {
		t.Fatalf("expected MPN in JSON output, got %+v", parts[0])
	}
}

func TestPartsShowJSON(t *testing.T) {
	stdout, err := runPartsCommand(t, "show", "ESP32-WROOM-32", "--format", "json")
	if err != nil {
		t.Fatalf("expected parts show json to succeed, got %v", err)
	}
	if strings.Contains(stdout, "MPN:") {
		t.Fatalf("expected JSON-only stdout, got %q", stdout)
	}
	var part contracts.SystemContract
	if err := json.Unmarshal([]byte(stdout), &part); err != nil {
		t.Fatalf("expected valid JSON, got %v\n%s", err, stdout)
	}
	if part.MPN != "ESP32-WROOM-32" {
		t.Fatalf("expected ESP32-WROOM-32, got %+v", part)
	}
}
