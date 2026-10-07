package cmd

import (
	"bytes"
	"errors"
	"os"
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

const fixedContractsDraftYAML = `# The user fills pin tokens and power_budget currents.
contracts:
  - id: spi
    scope:
      bus_type: spi
      bus_id: spi
    require:
      connected:
        nets: [SPI_SCK, SPI_MOSI, SPI_MISO]
        chip_selects:
          - ref: U2
            net: IMU_CS
          - ref: U3
            net: MAG_CS
        participants:
          - ref: U1
            role: master
          - ref: U2
            role: slave
          - ref: U3
            role: slave
    severity: error
  - id: can
    scope:
      bus_type: can
      bus_id: can
    require:
      connected:
        nets: [CANH, CANL]
        participants:
          - ref: U1
            role: master
          - ref: R1
            role: slave
          - ref: R2
            role: slave
          - ref: U4
            role: slave
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: 120
        count: 2
    severity: error
`

func TestContractsDraftFixedExample(t *testing.T) {
	repo := filepath.Join("..", "examples", "agent-loop", "fixed")
	dir := t.TempDir()
	copyFixture(t, filepath.Join(repo, "design.net"), filepath.Join(dir, "design.net"))
	contractsPath := filepath.Join(dir, ".architon", "contracts.yaml")
	copyFixture(t, filepath.Join(repo, ".architon", "contracts.yaml"), contractsPath)
	original, err := os.ReadFile(contractsPath)
	if err != nil {
		t.Fatalf("read contracts.yaml: %v", err)
	}

	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	draftPath := filepath.Join(filepath.Clean(root), ".architon", "contracts.draft.yaml")

	stdout, err := runContractsCommand(t, "draft", dir)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 || lines[0] != draftPath || lines[1] != "spi" || lines[2] != "can" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "PASS") || strings.Contains(stdout, "FAIL") || strings.Contains(stdout, "violations") || strings.Contains(stdout, "ARCHITON") {
		t.Fatalf("draft printed a verdict: %q", stdout)
	}

	got, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if string(got) != fixedContractsDraftYAML {
		t.Fatalf("draft YAML:\n%s\nwant:\n%s", got, fixedContractsDraftYAML)
	}
	if strings.Contains(string(got), "PB15") || strings.Contains(string(got), "PB13") || strings.Contains(string(got), "pins:") {
		t.Fatalf("draft guessed pin tokens:\n%s", got)
	}
	body := strings.TrimPrefix(string(got), "# The user fills pin tokens and power_budget currents.\n")
	if strings.Contains(body, "power_budget") || strings.Contains(body, "current_a") {
		t.Fatalf("draft emitted power_budget:\n%s", got)
	}

	unchanged, err := os.ReadFile(contractsPath)
	if err != nil {
		t.Fatalf("read contracts.yaml after draft: %v", err)
	}
	if !bytes.Equal(original, unchanged) {
		t.Fatal(".architon/contracts.yaml changed")
	}

	info, err := os.Stat(draftPath)
	if err != nil {
		t.Fatalf("stat draft: %v", err)
	}
	stdout, err = runContractsCommand(t, "draft", dir)
	if err == nil {
		t.Fatal("second draft without --force should fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected exit 3, got %v", err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists, got %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("second run wrote stdout %q", stdout)
	}
	after, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft after second run: %v", err)
	}
	if !bytes.Equal(got, after) {
		t.Fatal("second run changed the draft")
	}
	infoAfter, err := os.Stat(draftPath)
	if err != nil {
		t.Fatalf("stat draft after second run: %v", err)
	}
	if !infoAfter.ModTime().Equal(info.ModTime()) || infoAfter.Size() != info.Size() {
		t.Fatal("second run changed the draft file")
	}
	still, err := os.ReadFile(contractsPath)
	if err != nil {
		t.Fatalf("read contracts.yaml after second run: %v", err)
	}
	if !bytes.Equal(original, still) {
		t.Fatal(".architon/contracts.yaml changed on the second run")
	}

	validateOut, err := runContractsCommand(t, "validate", draftPath)
	if err != nil {
		t.Fatalf("validate draft: %v", err)
	}
	if !strings.Contains(validateOut, "contracts valid") {
		t.Fatalf("validate output = %q", validateOut)
	}

	if err := os.WriteFile(draftPath, []byte("contracts: []\n"), 0o644); err != nil {
		t.Fatalf("overwrite draft: %v", err)
	}
	stdout, err = runContractsCommand(t, "draft", dir, "--force")
	if err != nil {
		t.Fatalf("draft --force: %v", err)
	}
	if !strings.Contains(stdout, draftPath) || !strings.Contains(stdout, "spi") || !strings.Contains(stdout, "can") {
		t.Fatalf("force stdout = %q", stdout)
	}
	forced, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read forced draft: %v", err)
	}
	if string(forced) != fixedContractsDraftYAML {
		t.Fatalf("forced draft YAML:\n%s", forced)
	}
	finalContracts, err := os.ReadFile(contractsPath)
	if err != nil {
		t.Fatalf("read contracts.yaml after force: %v", err)
	}
	if !bytes.Equal(original, finalContracts) {
		t.Fatal("--force changed .architon/contracts.yaml")
	}
}

func TestContractsDraftNetlistWithoutRecognizedNets(t *testing.T) {
	dir := t.TempDir()
	netPath := filepath.Join(dir, "design.net")
	writeScanTestFile(t, netPath, `(export
  (version D)
  (design
    (source "plain.kicad_sch"))
  (components
    (comp (ref "U1") (value "REG")))
  (libparts)
  (nets
    (net (code "1") (name "PWR")
      (node (ref "U1") (pin "1") (pinfunction "PB15")))))
`)
	draftPath := filepath.Join(dir, ".architon", "contracts.draft.yaml")
	contractsPath := filepath.Join(dir, ".architon", "contracts.yaml")

	stdout, err := runContractsCommand(t, "draft", netPath)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1 || lines[0] != draftPath {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "PASS") || strings.Contains(stdout, "FAIL") {
		t.Fatalf("draft printed a verdict: %q", stdout)
	}
	got, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	want := "# The user fills pin tokens and power_budget currents.\ncontracts: []\n"
	if string(got) != want {
		t.Fatalf("draft YAML:\n%s", got)
	}
	if strings.Contains(string(got), "PWR") || strings.Contains(string(got), "PB15") {
		t.Fatalf("draft invented a contract:\n%s", got)
	}
	if _, err := os.Stat(contractsPath); !os.IsNotExist(err) {
		t.Fatalf("contracts.yaml should stay absent, stat err = %v", err)
	}
	validateOut, err := runContractsCommand(t, "validate", draftPath)
	if err != nil {
		t.Fatalf("validate empty draft: %v", err)
	}
	if !strings.Contains(validateOut, "contracts valid") {
		t.Fatalf("validate output = %q", validateOut)
	}
}

func runContractsCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newContractsCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func copyFixture(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	writeScanTestFile(t, dst, string(data))
}
