package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConnectionsProposeI2CFixture(t *testing.T) {
	dir := t.TempDir()
	writeScanTestFile(t, filepath.Join(dir, "design.net"), i2cProposalNetlist)
	contractsPath := filepath.Join(dir, ".architon", "contracts.yaml")
	rootSchPath := filepath.Join(dir, "board.kicad_sch")
	childSchPath := filepath.Join(dir, "sheets", "child.kicad_sch")
	contractsBefore := []byte("# keep this contracts file\ncontracts: []\n")
	rootSchBefore := []byte("(kicad_sch (version 20231120) (generator eeschema) (uuid root-sch))\n")
	childSchBefore := []byte("(kicad_sch (version 20231120) (generator eeschema) (uuid child-sch))\n")
	writeScanTestFile(t, contractsPath, string(contractsBefore))
	writeScanTestFile(t, rootSchPath, string(rootSchBefore))
	writeScanTestFile(t, childSchPath, string(childSchBefore))

	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	proposalPath := filepath.Join(filepath.Clean(root), ".architon", "connections.proposal.yaml")

	stdout, err := runConnectionsCommand(t, "propose", dir)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 || lines[0] != proposalPath || lines[1] != "decided: 1" || lines[2] != "needs_choice: 2" || lines[3] != "conflict: 0" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "PASS") || strings.Contains(stdout, "FAIL") || strings.Contains(stdout, "violations") || strings.Contains(stdout, "ARCHITON") {
		t.Fatalf("proposal printed a verdict: %q", stdout)
	}

	got, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	if !strings.Contains(string(got), decidedSDAFileSnippet) {
		t.Fatalf("decided SDA join missing:\n%s", got)
	}
	if !strings.Contains(string(got), "A decided entry is not a scan result, and a needs_choice entry is not accepted.") {
		t.Fatalf("proposal comment missing:\n%s", got)
	}
	doc := mustConnectionProposal(t, string(got))
	if len(doc.Connections) != 3 {
		t.Fatalf("entries = %d", len(doc.Connections))
	}
	decided := doc.Connections[0]
	if decided.Status != "decided" || decided.Signal != "SDA" || decided.Net != "I2C_SDA" {
		t.Fatalf("decided = %+v", decided)
	}
	if len(decided.Parts) != 2 || decided.Parts[0].Pin != "SDA" || decided.Parts[1].Pin != "SDA" {
		t.Fatalf("decided parts = %+v", decided.Parts)
	}
	if decided.Parts[0].Ref == "U1" || decided.Parts[1].Ref == "U1" {
		t.Fatalf("ESP32 pin was chosen: %+v", decided.Parts)
	}
	if decided.Parts[0].Citation.Datasheet == "" || decided.Parts[1].Citation.Datasheet == "" {
		t.Fatalf("citations = %+v", decided.Parts)
	}
	choice := doc.Connections[1]
	if choice.Status != "needs_choice" || choice.Ref != "U1" || choice.MPN != "ESP32-WROOM-32" || choice.Net != "I2C_SDA" {
		t.Fatalf("choice = %+v", choice)
	}
	if len(choice.Candidates) < 2 {
		t.Fatalf("expected every ESP32 candidate, got %d", len(choice.Candidates))
	}
	for _, candidate := range choice.Candidates {
		if len(candidate.Pins) == 0 {
			t.Fatalf("empty candidate: %+v", candidate)
		}
	}
	if strings.Contains(string(got), "accepted:") || strings.Contains(string(got), "pin: IO") || strings.Contains(string(got), "pin: GPIO") {
		t.Fatalf("a GPIO was marked accepted:\n%s", got)
	}
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	assertBytesUnchanged(t, rootSchPath, rootSchBefore)
	assertBytesUnchanged(t, childSchPath, childSchBefore)
	if _, statErr := os.Stat(filepath.Join(dir, ".architon", "contracts.draft.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("propose wrote contracts.draft.yaml: %v", statErr)
	}

	info, err := os.Stat(proposalPath)
	if err != nil {
		t.Fatalf("stat proposal: %v", err)
	}
	stdout, err = runConnectionsCommand(t, "propose", dir)
	if err == nil {
		t.Fatal("second propose without --force should fail")
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
	after, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("read proposal after second run: %v", err)
	}
	if !bytes.Equal(got, after) {
		t.Fatal("second run changed the proposal")
	}
	infoAfter, err := os.Stat(proposalPath)
	if err != nil {
		t.Fatalf("stat proposal after second run: %v", err)
	}
	if !infoAfter.ModTime().Equal(info.ModTime()) || infoAfter.Size() != info.Size() {
		t.Fatal("second run changed the proposal file")
	}
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	assertBytesUnchanged(t, rootSchPath, rootSchBefore)
	assertBytesUnchanged(t, childSchPath, childSchBefore)

	if err := os.WriteFile(proposalPath, []byte("connections: []\n"), 0o644); err != nil {
		t.Fatalf("overwrite proposal: %v", err)
	}
	stdout, err = runConnectionsCommand(t, "propose", dir, "--force")
	if err != nil {
		t.Fatalf("propose --force: %v", err)
	}
	if !strings.Contains(stdout, proposalPath) || !strings.Contains(stdout, "decided: 1") || !strings.Contains(stdout, "needs_choice: 2") || !strings.Contains(stdout, "conflict: 0") {
		t.Fatalf("force stdout = %q", stdout)
	}
	forced, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("read forced proposal: %v", err)
	}
	if !bytes.Equal(got, forced) {
		t.Fatalf("forced proposal changed:\n%s", forced)
	}
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	assertBytesUnchanged(t, rootSchPath, rootSchBefore)
	assertBytesUnchanged(t, childSchPath, childSchBefore)
}

func TestConnectionsProposeNothingMatches(t *testing.T) {
	dir := t.TempDir()
	netPath := filepath.Join(dir, "design.net")
	writeScanTestFile(t, netPath, `(export
  (version D)
  (design
    (source "plain.kicad_sch"))
  (components
    (comp (ref "R1") (value "10k")))
  (libparts)
  (nets
    (net (code "1") (name "PWR")
      (node (ref "R1") (pin "1") (pinfunction "1")))))
`)
	proposalPath := filepath.Join(dir, ".architon", "connections.proposal.yaml")
	stdout, err := runConnectionsCommand(t, "propose", netPath)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 || lines[0] != proposalPath || lines[1] != "decided: 0" || lines[2] != "needs_choice: 0" || lines[3] != "conflict: 0" {
		t.Fatalf("stdout = %q", stdout)
	}
	got, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	want := "# A decided entry is not a scan result, and a needs_choice entry is not accepted.\nconnections: []\n"
	if string(got) != want {
		t.Fatalf("proposal =\n%s", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".architon", "contracts.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("contracts.yaml should stay absent, stat err = %v", statErr)
	}
}

func TestConnectionsProposeDoesNotTouchSchematicWithoutNetlist(t *testing.T) {
	dir := t.TempDir()
	schPath := filepath.Join(dir, "board.kicad_sch")
	contractsPath := filepath.Join(dir, ".architon", "contracts.yaml")
	schBefore := []byte("(kicad_sch (version 20231120) (generator eeschema))\n")
	contractsBefore := []byte("contracts: []\n")
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, contractsPath, string(contractsBefore))

	_, err := runConnectionsCommand(t, "propose", dir)
	if err == nil {
		t.Fatal("expected a missing netlist to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected exit 3, got %v", err)
	}
	assertBytesUnchanged(t, schPath, schBefore)
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	if _, statErr := os.Stat(filepath.Join(dir, ".architon", "connections.proposal.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("proposal should stay absent, stat err = %v", statErr)
	}
}

func runConnectionsCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newConnectionsCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func assertBytesUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("%s changed", path)
	}
}

func mustConnectionProposal(t *testing.T, raw string) connectionProposalDoc {
	t.Helper()
	var doc connectionProposalDoc
	dec := yaml.NewDecoder(strings.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode proposal: %v\n%s", err, raw)
	}
	return doc
}

type connectionProposalDoc struct {
	Connections []connectionProposalEntry `yaml:"connections"`
}

type connectionProposalEntry struct {
	ID         string                        `yaml:"id"`
	Status     string                        `yaml:"status"`
	Signal     string                        `yaml:"signal"`
	Net        string                        `yaml:"net"`
	Ref        string                        `yaml:"ref"`
	MPN        string                        `yaml:"mpn"`
	Parts      []connectionProposalPart      `yaml:"parts"`
	Candidates []connectionProposalCandidate `yaml:"candidates"`
}

type connectionProposalPart struct {
	Ref      string                     `yaml:"ref"`
	MPN      string                     `yaml:"mpn"`
	Pin      string                     `yaml:"pin"`
	Net      string                     `yaml:"net"`
	Nets     []string                   `yaml:"nets"`
	Citation connectionProposalCitation `yaml:"citation"`
}

type connectionProposalCandidate struct {
	Pins     []string                   `yaml:"pins"`
	Number   string                     `yaml:"number"`
	Citation connectionProposalCitation `yaml:"citation"`
}

type connectionProposalCitation struct {
	Datasheet string `yaml:"datasheet"`
	Revision  string `yaml:"revision"`
	Table     string `yaml:"table"`
	Section   string `yaml:"section"`
}

const decidedSDAFileSnippet = `  - id: "sda-U2-U3"
    status: decided
    signal: SDA
    net: I2C_SDA
    parts:
      - ref: U2
        mpn: "MPU-6050"
        pin: SDA
        citation:
          datasheet: "MPU-6000 and MPU-6050 Product Specification"
          revision: "3.4"
          section: "7.1 Pin Out and Signal Description"
      - ref: U3
        mpn: BNO055
        pin: SDA
        citation:
          datasheet: "BNO055 Intelligent 9-axis absolute orientation sensor"
          revision: "1.8"
          table: "5-1 Pin description"
          section: "5.1 Pin-out"
`

const i2cProposalNetlist = `(export
  (version D)
  (design
    (source "board.kicad_sch"))
  (components
    (comp (ref "U1")
      (value "ESP32-WROOM-32")
      (fields
        (field (name "MPN") "ESP32-WROOM-32")))
    (comp (ref "U2")
      (value "MPU-6050")
      (fields
        (field (name "MPN") "MPU-6050")))
    (comp (ref "U3")
      (value "BNO055")
      (fields
        (field (name "MPN") "BNO055"))))
  (libparts)
  (nets
    (net (code "1") (name "I2C_SDA")
      (node (ref "U3") (pin "20") (pinfunction "SDA")))
    (net (code "2") (name "LED")
      (node (ref "U1") (pin "33") (pinfunction "IO21")))
    (net (code "3") (name "+3V3")
      (node (ref "U2") (pin "13") (pinfunction "VDD")))))
`
