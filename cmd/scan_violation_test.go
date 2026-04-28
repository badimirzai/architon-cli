package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan_OvervoltageYieldsExit2(t *testing.T) {
	tmp := t.TempDir()

	netlistFixture := kicadFixturePath(t, filepath.Join("overvoltage", "netlist_overvoltage.net"))
	metaFixture := kicadFixturePath(t, filepath.Join("overvoltage", "meta_overvoltage.yaml"))

	dstNet := filepath.Join(tmp, "netlist_overvoltage.net")
	data, err := os.ReadFile(netlistFixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(dstNet, data, 0o644); err != nil {
		t.Fatalf("write net: %v", err)
	}

	dstMeta := filepath.Join(tmp, "meta_overvoltage.yaml")
	data, err = os.ReadFile(metaFixture)
	if err != nil {
		t.Fatalf("read meta fixture: %v", err)
	}
	if err := os.WriteFile(dstMeta, data, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	// Build binary
	bin := filepath.Join(tmp, "rv-test")
	build := exec.Command("go", "build", "-o", bin, "../")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}

	run := exec.Command(bin, "scan", filepath.Base(dstNet), "--meta", filepath.Base(dstMeta))
	run.Dir = tmp
	runOut, runErr := run.CombinedOutput()
	output := string(runOut)

	// Expect exit code 2 (violations)
	if runErr == nil {
		t.Fatalf("expected non-zero exit code, got 0\n%s", output)
	}

	ee, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got: %T (%v)\n%s", runErr, runErr, output)
	}
	if ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %d\n%s", ee.ExitCode(), output)
	}
	if !strings.Contains(output, "Rules: 1\n") {
		t.Fatalf("expected output to show 1 rule finding\n%s", output)
	}
	if !strings.Contains(output, "Result: FAIL") {
		t.Fatalf("expected output to show failed result\n%s", output)
	}
	if !strings.Contains(output, "Violations: 1\n") {
		t.Fatalf("expected output to show 1 violation\n%s", output)
	}
	if !strings.Contains(output, "RULE_OVERVOLTAGE") {
		t.Fatalf("expected output to show RULE_OVERVOLTAGE\n%s", output)
	}
	if !strings.Contains(output, "U1 pin 1 on net /+5V is 5.00V (max 3.30V)") {
		t.Fatalf("expected output to show overvoltage message\n%s", output)
	}
	if !strings.Contains(output, "exit code: 2\n") {
		t.Fatalf("expected output to show exit code 2\n%s", output)
	}
}
