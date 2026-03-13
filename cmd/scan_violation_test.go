package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScan_OvervoltageYieldsExit2(t *testing.T) {
	tmp := t.TempDir()

	fixture := filepath.Join("internal", "importers", "kicad", "testdata", "netlist_overvoltage.net")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("missing fixture %s: %v", fixture, err)
	}

	if err := os.MkdirAll(filepath.Join(tmp, "exports"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dstNet := filepath.Join(tmp, "exports", "test.net")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(dstNet, data, 0o644); err != nil {
		t.Fatalf("write net: %v", err)
	}

	// Write configured meta that should trigger an overvoltage.
	if err := os.MkdirAll(filepath.Join(tmp, ".architon"), 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".architon", "meta.yaml"), []byte(`
version: "0"
sources:
  - net: VBAT
    voltage: 24.0
regulators: []
components:
  - ref: U1
    max_voltage: 5.5
`), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	// Build binary
	bin := filepath.Join(tmp, "rv-test")
	build := exec.Command("go", "build", "-o", bin, "./")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}

	run := exec.Command(bin, "scan", ".")
	run.Dir = tmp
	runOut, runErr := run.CombinedOutput()

	// Expect exit code 2 (violations)
	if runErr == nil {
		t.Fatalf("expected non-zero exit code, got 0\n%s", string(runOut))
	}

	ee, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got: %T (%v)\n%s", runErr, runErr, string(runOut))
	}
	if ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %d\n%s", ee.ExitCode(), string(runOut))
	}
}
