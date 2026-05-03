package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPartsListCommand(t *testing.T) {
	cmd := newPartsListCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("parts list failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "ESP32-WROOM-32") {
		t.Fatalf("expected built-in parts in output, got %q", stdout.String())
	}
}

func TestPartsShowCommand(t *testing.T) {
	cmd := newPartsShowCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"TXS0108E"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("parts show failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"TXS0108E", "Supplies:", "Logic:", "Confidence:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}
