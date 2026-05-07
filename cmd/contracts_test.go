package cmd

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractsValidateValidFileReturnsZero(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contracts.yaml")
	writeScanTestFile(t, path, `contracts:
  - id: i2c_policy
    scope:
      bus_type: i2c
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	cmd := newContractsValidateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected valid contracts file, got %v", err)
	}
	if !strings.Contains(stdout.String(), "contracts valid") {
		t.Fatalf("expected success output, got %q", stdout.String())
	}
}

func TestContractsValidateInvalidFileReturnsThree(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contracts.yaml")
	writeScanTestFile(t, path, `contracts:
  - id: bad
    require:
      pullup_ohms: {}
    severity: error
`)

	cmd := newContractsValidateCmd()
	cmd.SetArgs([]string{path})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid contracts file")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}
}
