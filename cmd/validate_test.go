package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cleanCheckSpec = `name: "clean"
power:
  battery:
    voltage_v: 12
    max_current_a: 30
  logic_rail:
    voltage_v: 3.3
    max_current_a: 2
mcu:
  name: "MCU"
  logic_voltage_v: 3.3
motor_driver:
  name: "Driver"
  motor_supply_min_v: 6
  motor_supply_max_v: 15
  continuous_per_channel_a: 2.0
  peak_per_channel_a: 6.0
  channels: 2
  logic_voltage_min_v: 3.0
  logic_voltage_max_v: 5.5
motors:
  - name: "Wheel motor"
    count: 1
    voltage_min_v: 6
    voltage_max_v: 12
    stall_current_a: 2.0
    nominal_current_a: 1.0
`

const warningOnlyCheckSpec = `name: "warning-only"
power:
  battery:
    voltage_v: 12
    max_current_a: 30
  logic_rail:
    voltage_v: 3.3
mcu:
  name: "MCU"
  logic_voltage_v: 3.3
motor_driver:
  name: "Driver"
  motor_supply_min_v: 6
  motor_supply_max_v: 15
  continuous_per_channel_a: 2.0
  peak_per_channel_a: 6.0
  channels: 2
  logic_voltage_min_v: 3.0
  logic_voltage_max_v: 5.5
motors:
  - name: "Wheel motor"
    count: 1
    voltage_min_v: 6
    voltage_max_v: 12
    stall_current_a: 2.0
    nominal_current_a: 1.0
`

const errorAndWarningCheckSpec = `name: "error-and-warning"
power:
  battery:
    voltage_v: 20
    max_current_a: 30
  logic_rail:
    voltage_v: 3.3
mcu:
  name: "MCU"
  logic_voltage_v: 3.3
motor_driver:
  name: "Driver"
  motor_supply_min_v: 6
  motor_supply_max_v: 15
  continuous_per_channel_a: 2.0
  peak_per_channel_a: 6.0
  channels: 2
  logic_voltage_min_v: 3.0
  logic_voltage_max_v: 5.5
motors:
  - name: "Wheel motor"
    count: 1
    voltage_min_v: 6
    voltage_max_v: 12
    stall_current_a: 2.0
    nominal_current_a: 1.0
`

const missingRequiredFieldsSpec = `name: "missing-required"
power:
  battery:
    voltage_v: 12
    max_current_a: 30
  logic_rail:
    voltage_v: 3.3
    max_current_a: 2
motor_driver:
  name: "Driver"
  motor_supply_min_v: 6
  motor_supply_max_v: 15
  continuous_per_channel_a: 2.0
  peak_per_channel_a: 6.0
  channels: 2
  logic_voltage_min_v: 3.0
  logic_voltage_max_v: 5.5
motors:
  - name: "Wheel motor"
    count: 1
    voltage_min_v: 6
    voltage_max_v: 12
    stall_current_a: 2.0
    nominal_current_a: 1.0
`

func executeCheckCommandForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := newCheckCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeCheckSpec(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

func exitCodeFromError(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	return exitErr.Code
}

func TestCheckExitCodeCleanInputIsZero(t *testing.T) {
	specPath := writeCheckSpec(t, cleanCheckSpec)

	stdout, stderr, err := executeCheckCommandForTest(t, specPath)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "exit code: 0") {
		t.Fatalf("expected exit code 0 in output, got %q", stdout)
	}
}

func TestCheckExitCodeWarningOnlyIsOne(t *testing.T) {
	specPath := writeCheckSpec(t, warningOnlyCheckSpec)

	stdout, stderr, err := executeCheckCommandForTest(t, specPath)
	if code := exitCodeFromError(t, err); code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "WARN RAIL_I_UNKNOWN") {
		t.Fatalf("expected warning finding in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "exit code: 1") {
		t.Fatalf("expected exit code 1 in output, got %q", stdout)
	}
}

func TestCheckExitCodeWarningOnlyIsTwoWithWarnAsError(t *testing.T) {
	specPath := writeCheckSpec(t, warningOnlyCheckSpec)

	stdout, stderr, err := executeCheckCommandForTest(t, "--warn-as-error", specPath)
	if code := exitCodeFromError(t, err); code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "WARN RAIL_I_UNKNOWN") {
		t.Fatalf("expected warning finding in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "exit code: 2") {
		t.Fatalf("expected exit code 2 in output, got %q", stdout)
	}
}

func TestCheckExitCodeErrorsTakePrecedenceOverWarnings(t *testing.T) {
	specPath := writeCheckSpec(t, errorAndWarningCheckSpec)

	stdout, stderr, err := executeCheckCommandForTest(t, specPath)
	if code := exitCodeFromError(t, err); code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "ERROR DRV_SUPPLY_RANGE") {
		t.Fatalf("expected error finding in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "WARN RAIL_I_UNKNOWN") {
		t.Fatalf("expected warning finding in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "exit code: 2") {
		t.Fatalf("expected exit code 2 in output, got %q", stdout)
	}
}

func TestCheckExitCodeErrorsRemainTwoWithWarnAsError(t *testing.T) {
	specPath := writeCheckSpec(t, errorAndWarningCheckSpec)

	stdout, stderr, err := executeCheckCommandForTest(t, "--warn-as-error", specPath)
	if code := exitCodeFromError(t, err); code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "ERROR DRV_SUPPLY_RANGE") {
		t.Fatalf("expected error finding in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "WARN RAIL_I_UNKNOWN") {
		t.Fatalf("expected warning finding in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "exit code: 2") {
		t.Fatalf("expected exit code 2 in output, got %q", stdout)
	}
}

func TestCheckExitCodeInvalidYAMLIsThree(t *testing.T) {
	specPath := writeCheckSpec(t, "name: [\n")

	stdout, stderr, err := executeCheckCommandForTest(t, specPath)
	if code := exitCodeFromError(t, err); code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(stderr, "parse yaml") {
		t.Fatalf("expected parse yaml error, got %q", stderr)
	}
	if !strings.Contains(stdout, "exit code: 3") {
		t.Fatalf("expected exit code 3 in output, got %q", stdout)
	}
}

func TestCheckExitCodeMissingRequiredFieldsIsThree(t *testing.T) {
	specPath := writeCheckSpec(t, missingRequiredFieldsSpec)

	stdout, stderr, err := executeCheckCommandForTest(t, specPath)
	if code := exitCodeFromError(t, err); code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(stderr, "mcu.logic_voltage_v is missing") {
		t.Fatalf("expected missing required field error, got %q", stderr)
	}
	if !strings.Contains(stdout, "exit code: 3") {
		t.Fatalf("expected exit code 3 in output, got %q", stdout)
	}
}
