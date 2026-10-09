package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/importers/kicad"
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
	Pin        string                        `yaml:"pin,omitempty"`
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

func TestConnectionsApplyAcceptedSDALabels(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	schPath := filepath.Join(root, "board.kicad_sch")
	childPath := filepath.Join(root, "sheets", "child.kicad_sch")
	pcbPath := filepath.Join(root, "board.kicad_pcb")
	contractsPath := filepath.Join(root, ".architon", "contracts.yaml")
	proposalPath := filepath.Join(root, ".architon", "connections.proposal.yaml")
	schBefore := []byte(twoSDASchematic())
	childBefore := []byte(indentKiCad(`(kicad_sch
>(version 20250114)
>(generator "eeschema")
>(generator_version "9.0")
>(uuid "99999999-9999-4999-8999-999999999999")
>(paper "A4")
>(embedded_fonts no)
)
`))
	pcbBefore := []byte("(kicad_pcb (version 20241229))\n")
	contractsBefore := []byte("contracts: []\n")
	proposalBefore := []byte(acceptedSDAProposal)
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, childPath, string(childBefore))
	writeScanTestFile(t, pcbPath, string(pcbBefore))
	writeScanTestFile(t, contractsPath, string(contractsBefore))
	writeScanTestFile(t, proposalPath, string(proposalBefore))

	stdout, err := runConnectionsCommand(t, "apply", dir)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if stdout != schPath+"\nsda-U2-U3\nRun rv scan or the verify tool.\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "PASS") || strings.Contains(stdout, "FAIL") || strings.Contains(stdout, "violations") || strings.Contains(stdout, "ARCHITON") {
		t.Fatalf("apply printed a verdict: %q", stdout)
	}
	got, err := os.ReadFile(schPath)
	if err != nil {
		t.Fatalf("read schematic: %v", err)
	}
	assertOnlyNetLabelsInserted(t, string(schBefore), string(got), "100 80", "130 80")
	if strings.Contains(string(got), "(wire") || strings.Contains(string(got), "(no_connect") {
		t.Fatal("apply added a wire or no-connect")
	}
	assertBytesUnchanged(t, childPath, childBefore)
	assertBytesUnchanged(t, pcbPath, pcbBefore)
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	assertBytesUnchanged(t, proposalPath, proposalBefore)
	if _, statErr := os.Stat(filepath.Join(root, "architon-report.json")); !os.IsNotExist(statErr) {
		t.Fatalf("apply wrote a scan report: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".architon", "generated.net")); !os.IsNotExist(statErr) {
		t.Fatalf("apply wrote a netlist: %v", statErr)
	}

	info, err := os.Stat(schPath)
	if err != nil {
		t.Fatalf("stat schematic: %v", err)
	}
	stdout, err = runConnectionsCommand(t, "apply", dir)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if stdout != "sda-U2-U3\nRun rv scan or the verify tool.\n" {
		t.Fatalf("second stdout = %q", stdout)
	}
	assertBytesUnchanged(t, schPath, got)
	infoAfter, err := os.Stat(schPath)
	if err != nil {
		t.Fatalf("stat schematic after second apply: %v", err)
	}
	if !infoAfter.ModTime().Equal(info.ModTime()) || infoAfter.Size() != info.Size() {
		t.Fatal("second apply rewrote the schematic")
	}
	assertBytesUnchanged(t, childPath, childBefore)
	assertBytesUnchanged(t, pcbPath, pcbBefore)
	assertBytesUnchanged(t, proposalPath, proposalBefore)
}

func TestConnectionsApplySkipsUndecidedEntries(t *testing.T) {
	dir := t.TempDir()
	schPath := filepath.Join(dir, "board.kicad_sch")
	schBefore := []byte(twoSDASchematic())
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, filepath.Join(dir, ".architon", "connections.proposal.yaml"), undecidedConnectionsProposal)

	stdout, err := runConnectionsCommand(t, "apply", dir)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if stdout != "Run rv scan or the verify tool.\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	assertBytesUnchanged(t, schPath, schBefore)
}

func TestConnectionsApplyDifferentNetWritesNothing(t *testing.T) {
	dir := t.TempDir()
	schPath := filepath.Join(dir, "board.kicad_sch")
	pcbPath := filepath.Join(dir, "board.kicad_pcb")
	schBefore := []byte(schematicWithOtherNet(twoSDASchematic()))
	pcbBefore := []byte("(kicad_pcb (version 20241229))\n")
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, pcbPath, string(pcbBefore))
	writeScanTestFile(t, filepath.Join(dir, ".architon", "connections.proposal.yaml"), acceptedSDAProposal)

	stdout, err := runConnectionsCommand(t, "apply", dir)
	if err == nil {
		t.Fatal("expected a pin on another net to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected exit 3, got %v", err)
	}
	if !strings.Contains(err.Error(), "pin U2 SDA is already on net OTHER") {
		t.Fatalf("error = %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	assertBytesUnchanged(t, schPath, schBefore)
	assertBytesUnchanged(t, pcbPath, pcbBefore)
}

func TestConnectionsApplyMissingPinWritesNothing(t *testing.T) {
	dir := t.TempDir()
	schPath := filepath.Join(dir, "board.kicad_sch")
	schBefore := []byte(twoSDASchematic())
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, filepath.Join(dir, ".architon", "connections.proposal.yaml"), strings.Replace(acceptedSDAProposal, "ref: U3", "ref: U9", 1))

	stdout, err := runConnectionsCommand(t, "apply", dir)
	if err == nil {
		t.Fatal("expected a missing pin to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected exit 3, got %v", err)
	}
	if !strings.Contains(err.Error(), "pin U9 SDA not found") {
		t.Fatalf("error = %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	assertBytesUnchanged(t, schPath, schBefore)
}

func TestAcceptedEntriesKeepNumericPins(t *testing.T) {
	entries, err := acceptedNetLabelEntries([]byte(`
connections:
  - id: sda-U2-U3
    status: accepted
    net: I2C_SDA
    parts:
      - ref: U2
        pin: 1
      - ref: U3
        pin: "24"
  - id: sda-U1
    status: needs_choice
    net: I2C_SDA
    ref: U1
  - id: sda-U4-U5
    status: decided
    net: I2C_SDA
    parts:
      - ref: U4
        pin: SDA
      - ref: U5
        pin: SDA
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "sda-U2-U3" || entries[0].Net != "I2C_SDA" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Pins[0] != (kicad.NetLabelPin{Ref: "U2", Pin: "1"}) || entries[0].Pins[1] != (kicad.NetLabelPin{Ref: "U3", Pin: "24"}) {
		t.Fatalf("pins = %+v", entries[0].Pins)
	}
}

func TestV019FixtureChoiceAcceptsOnlyAListedPin(t *testing.T) {
	dir := t.TempDir()
	writeScanTestFile(t, filepath.Join(dir, "design.net"), i2cProposalNetlist)
	document, _, err := buildConnectionProposal(dir)
	if err != nil {
		t.Fatal(err)
	}
	doc := mustConnectionProposal(t, document.YAML)
	var choice *connectionProposalEntry
	for i := range doc.Connections {
		entry := &doc.Connections[i]
		if entry.ID == "sda-U1" && entry.Status == "needs_choice" && entry.Ref == "U1" {
			choice = entry
		}
	}
	if choice == nil || len(choice.Candidates) < 2 || len(choice.Candidates[0].Pins) == 0 {
		t.Fatalf("MCU choice = %+v", choice)
	}
	listed := choice.Candidates[0].Pins[0]
	choice.Status = "accepted"
	choice.Pin = listed
	raw, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := acceptedNetLabelEntries(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "sda-U1" || entries[0].Pins[0] != (kicad.NetLabelPin{Ref: "U1", Pin: listed}) {
		t.Fatalf("entries = %+v", entries)
	}

	choice.Pin = "PA0"
	raw, err = yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptedNetLabelEntries(raw); err == nil || !strings.Contains(err.Error(), "pin PA0 is not in candidates") {
		t.Fatalf("err = %v", err)
	}
}

func TestAcceptedChoiceRequiresListedPin(t *testing.T) {
	_, err := acceptedNetLabelEntries([]byte(unlistedChoiceProposal))
	if err == nil || !strings.Contains(err.Error(), "pin SDA is not in candidates") {
		t.Fatalf("err = %v", err)
	}

	entries, err := acceptedNetLabelEntries([]byte(listedChoiceProposal))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "sda-U2" || entries[0].Net != "I2C_SDA" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Pins[0] != (kicad.NetLabelPin{Ref: "U2", Pin: "SDA"}) || entries[0].Pins[1] != (kicad.NetLabelPin{}) {
		t.Fatalf("pins = %+v", entries[0].Pins)
	}

	numbered, err := acceptedNetLabelEntries([]byte(strings.Replace(listedChoiceProposal, "pin: SDA", "pin: \"1\"", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(numbered) != 1 || numbered[0].Pins[0].Pin != "1" {
		t.Fatalf("numbered = %+v", numbered)
	}

	alias, err := acceptedNetLabelEntries([]byte(strings.Replace(listedChoiceProposal, "pin: SDA", "pin: GPIO21", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(alias) != 1 || alias[0].Pins[0].Pin != "GPIO21" {
		t.Fatalf("alias = %+v", alias)
	}

	skipped, err := acceptedNetLabelEntries([]byte(strings.Replace(unlistedChoiceProposal, "status: accepted", "status: needs_choice", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("needs_choice was applied: %+v", skipped)
	}
}

func TestConnectionsApplyUnlistedCandidateWritesNothing(t *testing.T) {
	dir := t.TempDir()
	schPath := filepath.Join(dir, "board.kicad_sch")
	schBefore := []byte(twoSDASchematic())
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, filepath.Join(dir, ".architon", "connections.proposal.yaml"), unlistedChoiceProposal)

	info, err := os.Stat(schPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := runConnectionsCommand(t, "apply", dir)
	if err == nil {
		t.Fatal("expected a pin outside candidates to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("expected exit 3, got %v", err)
	}
	if !strings.Contains(err.Error(), "not in candidates") {
		t.Fatalf("error = %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	assertBytesUnchanged(t, schPath, schBefore)
	infoAfter, err := os.Stat(schPath)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(info.ModTime()) || infoAfter.Size() != info.Size() {
		t.Fatal("apply rewrote the schematic")
	}
}

func TestConnectionsApplyListedCandidateWritesLabel(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	schPath := filepath.Join(root, "board.kicad_sch")
	proposalPath := filepath.Join(root, ".architon", "connections.proposal.yaml")
	schBefore := []byte(twoSDASchematic())
	proposalBefore := []byte(listedChoiceProposal)
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, proposalPath, string(proposalBefore))

	stdout, err := runConnectionsCommand(t, "apply", proposalPath)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if stdout != schPath+"\nsda-U2\nRun rv scan or the verify tool.\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "PASS") || strings.Contains(stdout, "FAIL") || strings.Contains(stdout, "exit") {
		t.Fatalf("apply printed a verdict: %q", stdout)
	}
	got, err := os.ReadFile(schPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(schBefore, got) || !strings.Contains(string(got), "(label \"I2C_SDA\"") {
		t.Fatalf("listed pin was not labeled:\n%s", got)
	}
	if strings.Contains(string(got), "(wire") || strings.Contains(string(got), "(no_connect") {
		t.Fatal("apply added a wire or no-connect")
	}
	assertBytesUnchanged(t, proposalPath, proposalBefore)
}

func assertOnlyNetLabelsInserted(t *testing.T, before string, after string, positions ...string) {
	t.Helper()
	idx := strings.LastIndex(before, ")")
	if idx < 0 {
		t.Fatal("schematic has no closing parenthesis")
	}
	if !strings.HasPrefix(after, before[:idx]) || !strings.HasSuffix(after, before[idx:]) {
		t.Fatal("schematic changed outside the inserted labels")
	}
	inserted := after[idx : len(after)-len(before[idx:])]
	if strings.Contains(inserted, "(wire") || strings.Contains(inserted, "(symbol") || strings.Contains(inserted, "(no_connect") {
		t.Fatalf("inserted more than labels:\n%s", inserted)
	}
	if strings.Count(inserted, "(label \"I2C_SDA\"") != len(positions) {
		t.Fatalf("inserted labels:\n%s", inserted)
	}
	for _, position := range positions {
		needle := "\t(label \"I2C_SDA\"\n\t\t(at " + position + " 0)\n\t\t(effects\n\t\t\t(font\n\t\t\t\t(size 1.27 1.27)\n\t\t\t)\n\t\t\t(justify left bottom)\n\t\t)\n\t\t(uuid \""
		if !strings.Contains(inserted, needle) {
			t.Fatalf("label at %s missing:\n%s", position, inserted)
		}
	}
}

func schematicWithOtherNet(base string) string {
	extra := indentKiCad(`>(wire
>>(pts
>>>(xy 100 80) (xy 100 60)
>>)
>>(stroke
>>>(width 0)
>>>(type default)
>>)
>>(uuid "44444444-4444-4444-8444-444444444444")
>)
>(label "OTHER"
>>(at 100 60 0)
>>(effects
>>>(font
>>>>(size 1.27 1.27)
>>>)
>>>(justify left bottom)
>>)
>>(uuid "55555555-5555-4555-8555-555555555555")
>)
`)
	const marker = "\t(embedded_fonts no)\n"
	if !strings.Contains(base, marker) {
		panic("schematic marker missing")
	}
	return strings.Replace(base, marker, extra+marker, 1)
}

func indentKiCad(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		n := 0
		for n < len(line) && line[n] == '>' {
			n++
		}
		if n > 0 {
			lines[i] = strings.Repeat("\t", n) + line[n:]
		}
	}
	return strings.Join(lines, "\n")
}

func twoSDASchematic() string {
	return indentKiCad(`(kicad_sch
>(version 20250114)
>(generator "eeschema")
>(generator_version "9.0")
>(uuid "11111111-1111-4111-8111-111111111111")
>(paper "A4")
>(lib_symbols
>>(symbol "Sensor:Part"
>>>(symbol "Part_1_1"
>>>>(pin bidirectional line
>>>>>(at 0 0 0)
>>>>>(length 2.54)
>>>>>(name "SDA"
>>>>>>(effects
>>>>>>>(font
>>>>>>>(size 1.27 1.27)
>>>>>>)
>>>>>)
>>>>)
>>>>>(number "1"
>>>>>>(effects
>>>>>>>(font
>>>>>>>(size 1.27 1.27)
>>>>>>)
>>>>>)
>>>>)
>>>>)
>>>)
>>)
>)
>(symbol
>>(lib_id "Sensor:Part")
>>(at 100 80 0)
>>(unit 1)
>>(exclude_from_sim no)
>>(in_bom yes)
>>(on_board yes)
>>(dnp no)
>>(uuid "22222222-2222-4222-8222-222222222222")
>>(property "Reference" "U2"
>>>(at 100 70 0)
>>>(effects
>>>>(font
>>>>>(size 1.27 1.27)
>>>>)
>>>)
>>)
>>(pin "1"
>>>(uuid "33333333-3333-4333-8333-333333333333")
>>)
>>(instances
>>>(project "fixture"
>>>>(path "/11111111-1111-4111-8111-111111111111"
>>>>>(reference "U2")
>>>>>(unit 1)
>>>>)
>>>)
>>)
>)
>(symbol
>>(lib_id "Sensor:Part")
>>(at 130 80 0)
>>(unit 1)
>>(exclude_from_sim no)
>>(in_bom yes)
>>(on_board yes)
>>(dnp no)
>>(uuid "66666666-6666-4666-8666-666666666666")
>>(property "Reference" "U3"
>>>(at 130 70 0)
>>>(effects
>>>>(font
>>>>>(size 1.27 1.27)
>>>>)
>>>)
>>)
>>(pin "1"
>>>(uuid "77777777-7777-4777-8777-777777777777")
>>)
>>(instances
>>>(project "fixture"
>>>>(path "/11111111-1111-4111-8111-111111111111"
>>>>>(reference "U3")
>>>>>(unit 1)
>>>>)
>>>)
>>)
>)
>(embedded_fonts no)
)
`)
}

const acceptedSDAProposal = `# A decided entry is not a scan result, and a needs_choice entry is not accepted.
connections:
  - id: "sda-U2-U3"
    status: accepted
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

const listedChoiceProposal = `# A decided entry is not a scan result, and a needs_choice entry is not accepted.
connections:
  - id: sda-U2
    status: accepted
    signal: SDA
    net: I2C_SDA
    ref: U2
    pin: SDA
    candidates:
      - pins: [SDA, GPIO21]
        number: "1"
`

const unlistedChoiceProposal = `# A decided entry is not a scan result, and a needs_choice entry is not accepted.
connections:
  - id: sda-U2
    status: accepted
    signal: SDA
    net: I2C_SDA
    ref: U2
    pin: SDA
    candidates:
      - pins: [IO21, GPIO21]
        number: "33"
`

const undecidedConnectionsProposal = `# A decided entry is not a scan result, and a needs_choice entry is not accepted.
connections:
  - id: "sda-U2-U3"
    status: decided
    signal: SDA
    net: I2C_SDA
    parts:
      - ref: U2
        mpn: "MPU-6050"
        pin: SDA
      - ref: U3
        mpn: BNO055
        pin: SDA
  - id: "sda-U1"
    status: needs_choice
    signal: SDA
    net: I2C_SDA
    ref: U1
    mpn: "ESP32-WROOM-32"
    candidates:
      - pins: [IO21, GPIO21]
        number: "33"
  - id: "scl-U2-U3"
    status: conflict
    signal: SCL
    parts:
      - ref: U2
        pin: SCL
        net: SCL_A
      - ref: U3
        pin: SCL
        net: SCL_B
`
