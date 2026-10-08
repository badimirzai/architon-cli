package contracts

import (
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
	"gopkg.in/yaml.v3"
)

func TestProposeDedicatedSDAJoinLeavesESP32Undecided(t *testing.T) {
	design := i2cProposalDesign(false)
	first := ProposeConnections(design)
	second := ProposeConnections(design)
	if first.YAML != second.YAML {
		t.Fatal("proposal changed across runs")
	}
	if first.Decided != 1 || first.NeedsChoice != 2 || first.Conflict != 0 {
		t.Fatalf("counts = decided %d needs_choice %d conflict %d", first.Decided, first.NeedsChoice, first.Conflict)
	}
	if !strings.HasPrefix(first.YAML, connectionProposalComment) {
		t.Fatalf("missing proposal comment:\n%s", first.YAML)
	}
	if !strings.Contains(first.YAML, decidedSDASnippet) {
		t.Fatalf("decided SDA join missing:\n%s", first.YAML)
	}
	if !strings.Contains(first.YAML, esp32SDAChoiceSnippet) {
		t.Fatalf("ESP32 SDA choice missing:\n%s", first.YAML)
	}
	if !strings.Contains(first.YAML, esp32SCLChoiceSnippet) {
		t.Fatalf("ESP32 SCL choice missing:\n%s", first.YAML)
	}
	for _, forbidden := range []string{"accepted:", "chosen:", "selected:", "rank:", "winner:", "pin: IO", "pin: GPIO", "pin: PB", "pin: RXD", "pin: TXD", "PASS", "FAIL"} {
		if strings.Contains(first.YAML, forbidden) {
			t.Fatalf("proposal marked a GPIO or a verdict with %q:\n%s", forbidden, first.YAML)
		}
	}

	doc := mustProposalDoc(t, first.YAML)
	if len(doc.Connections) != 3 {
		t.Fatalf("entries = %+v", doc.Connections)
	}
	decided := doc.Connections[0]
	if decided.ID != "sda-U2-U3" || decided.Status != "decided" || decided.Signal != "SDA" || decided.Net != "I2C_SDA" {
		t.Fatalf("decided entry = %+v", decided)
	}
	if len(decided.Parts) != 2 || decided.Parts[0].Ref != "U2" || decided.Parts[1].Ref != "U3" {
		t.Fatalf("decided parts = %+v", decided.Parts)
	}
	if decided.Parts[0].Pin != "SDA" || decided.Parts[1].Pin != "SDA" {
		t.Fatalf("decided pins = %+v", decided.Parts)
	}
	if decided.Parts[0].Citation.Datasheet == "" || decided.Parts[1].Citation.Datasheet == "" {
		t.Fatalf("decided citations = %+v", decided.Parts)
	}
	if decided.Parts[0].Citation.Datasheet == decided.Parts[1].Citation.Datasheet {
		t.Fatalf("expected two citations, got %+v", decided.Parts)
	}
	if len(decided.Candidates) != 0 || decided.Ref != "" {
		t.Fatalf("decided entry chose an MCU pin: %+v", decided)
	}

	sdaChoice := doc.Connections[1]
	sclChoice := doc.Connections[2]
	if sdaChoice.Status != "needs_choice" || sdaChoice.Ref != "U1" || sdaChoice.MPN != "ESP32-WROOM-32" || sdaChoice.Net != "I2C_SDA" {
		t.Fatalf("SDA choice = %+v", sdaChoice)
	}
	if sclChoice.Status != "needs_choice" || sclChoice.Ref != "U1" || sclChoice.Signal != "SCL" || sclChoice.Net != "" {
		t.Fatalf("SCL choice = %+v", sclChoice)
	}
	assertEveryGPIOCandidate(t, "ESP32-WROOM-32", sdaChoice.Candidates)
	assertEveryGPIOCandidate(t, "ESP32-WROOM-32", sclChoice.Candidates)
	if len(sdaChoice.Parts) != 0 {
		t.Fatalf("choice named a decided pin: %+v", sdaChoice.Parts)
	}

	swapped := i2cProposalDesign(true)
	again := ProposeConnections(swapped)
	if again.YAML != first.YAML || again.Decided != 1 {
		t.Fatal("swapping which dedicated SDA pin is on I2C_SDA changed the proposal")
	}
}

func TestProposeGPIOOnASDANetDoesNotDecide(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U1", MPN: "ESP32-WROOM-32", Value: "ESP32-WROOM-32"},
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
		},
		Nets: []ir.Net{{
			Name: "I2C_SDA",
			Pins: []ir.PinRef{{Ref: "U1", Pin: "IO21", Name: "IO21"}},
		}},
	}
	got := ProposeConnections(design)
	if got.Decided != 0 || got.Conflict != 0 || got.NeedsChoice != 2 {
		t.Fatalf("counts = %+v", got)
	}
	if strings.Contains(got.YAML, "net:") || strings.Contains(got.YAML, "status: decided") {
		t.Fatalf("GPIO on I2C_SDA decided a join:\n%s", got.YAML)
	}
	doc := mustProposalDoc(t, got.YAML)
	for _, entry := range doc.Connections {
		if entry.Status != "needs_choice" || entry.Ref != "U1" || entry.Net != "" {
			t.Fatalf("unexpected entry %+v", entry)
		}
	}
}

func TestProposeDedicatedPinsOnDifferentNetsConflict(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U1", MPN: "ESP32-WROOM-32", Value: "ESP32-WROOM-32"},
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
			{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
		},
		Nets: []ir.Net{
			{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "U2", Pin: "SDA", Name: "SDA"}}},
			{Name: "SENSOR_SDA", Pins: []ir.PinRef{{Ref: "U3", Pin: "20", Name: "COM0"}}},
		},
	}
	got := ProposeConnections(design)
	if got.Decided != 0 || got.Conflict != 1 {
		t.Fatalf("counts = decided %d needs_choice %d conflict %d\n%s", got.Decided, got.NeedsChoice, got.Conflict, got.YAML)
	}
	doc := mustProposalDoc(t, got.YAML)
	var conflict proposalEntryDoc
	for _, entry := range doc.Connections {
		if entry.Status == "conflict" {
			conflict = entry
		}
		if entry.Status == "needs_choice" && entry.Net != "" {
			t.Fatalf("conflict still named a join net: %+v", entry)
		}
		if entry.Status == "decided" {
			t.Fatalf("conflict proposed a join: %+v", entry)
		}
	}
	if conflict.ID != "sda-U2-U3" || conflict.Signal != "SDA" || conflict.Net != "" {
		t.Fatalf("conflict = %+v", conflict)
	}
	if len(conflict.Parts) != 2 || conflict.Parts[0].Net != "I2C_SDA" || conflict.Parts[1].Net != "SENSOR_SDA" {
		t.Fatalf("conflict parts = %+v", conflict.Parts)
	}
	if conflict.Parts[1].Pin != "SDA" {
		t.Fatalf("COM0 alias should stay named SDA, got %+v", conflict.Parts[1])
	}
}

func TestProposeAlreadySharedNetIsDecided(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
			{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
		},
		Nets: []ir.Net{{
			Name: "/I2C_SDA",
			Pins: []ir.PinRef{
				{Ref: "U2", Pin: "24"},
				{Ref: "U3", Pin: "SDA", Name: "SDA"},
			},
		}},
	}
	got := ProposeConnections(design)
	if got.Decided != 1 || got.NeedsChoice != 0 || got.Conflict != 0 {
		t.Fatalf("counts = decided %d needs_choice %d conflict %d", got.Decided, got.NeedsChoice, got.Conflict)
	}
	doc := mustProposalDoc(t, got.YAML)
	if doc.Connections[0].Net != "/I2C_SDA" || doc.Connections[0].Parts[0].Pin != "SDA" {
		t.Fatalf("pin number 24 should decide datasheet pin SDA on the stored net, got %+v", doc.Connections[0])
	}
}

func TestProposeSlashFormsAreOneNet(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
			{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
		},
		Nets: []ir.Net{
			{Name: "/I2C_SDA", Pins: []ir.PinRef{{Ref: "U2", Pin: "SDA", Name: "SDA"}}},
			{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "U3", Pin: "SDA", Name: "SDA"}}},
		},
	}
	got := ProposeConnections(design)
	doc := mustProposalDoc(t, got.YAML)
	if got.Conflict != 0 || got.Decided != 1 || doc.Connections[0].Net != "I2C_SDA" || doc.Connections[0].Status != "decided" {
		t.Fatalf("slash forms conflicted: %+v\n%s", got, got.YAML)
	}
}

func TestProposeUnconnectedKiCadNetCanJoin(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
			{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
		},
		Nets: []ir.Net{
			{Name: "unconnected-(U2-SDA-Pad24)", Pins: []ir.PinRef{{Ref: "U2", Pin: "SDA", Name: "SDA"}}},
			{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "U3", Pin: "SDA", Name: "SDA"}}},
		},
	}
	got := ProposeConnections(design)
	doc := mustProposalDoc(t, got.YAML)
	if got.Decided != 1 || got.Conflict != 0 || doc.Connections[0].Net != "I2C_SDA" || doc.Connections[0].Status != "decided" {
		t.Fatalf("unconnected net was treated as a join target: %+v\n%s", got, got.YAML)
	}
}

func TestProposeNothingMatchesWritesEmptyList(t *testing.T) {
	cases := []*ir.DesignIR{
		nil,
		{Parts: []ir.Part{{Ref: "R1", Value: "10k"}}, Nets: []ir.Net{{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "R1", Pin: "1"}}}}},
		{
			Parts: []ir.Part{
				{Ref: "U2", MPN: "AP2114H-3.3", Value: "AP2114H-3.3"},
				{Ref: "U3", MPN: "AP2114H-3.3", Value: "AP2114H-3.3"},
			},
			Nets: []ir.Net{{Name: "VIN", Pins: []ir.PinRef{{Ref: "U2", Pin: "VIN", Name: "VIN"}}}},
		},
		{
			Parts: []ir.Part{
				{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
				{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
			},
		},
	}
	for _, design := range cases {
		got := ProposeConnections(design)
		if got.Decided != 0 || got.NeedsChoice != 0 || got.Conflict != 0 {
			t.Fatalf("expected an empty proposal, got %+v\n%s", got, got.YAML)
		}
		if got.YAML != connectionProposalComment+"connections: []\n" {
			t.Fatalf("empty YAML =\n%s", got.YAML)
		}
	}
}

func TestProposeWrongSharedNetIsNotAScanFinding(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
			{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
		},
		Nets: []ir.Net{{
			Name: "I2C_SCL",
			Pins: []ir.PinRef{
				{Ref: "U2", Pin: "SDA", Name: "SDA"},
				{Ref: "U3", Pin: "SDA", Name: "SDA"},
			},
		}},
	}
	got := ProposeConnections(design)
	if got.Decided != 1 || got.Conflict != 0 {
		t.Fatalf("counts = %+v", got)
	}
	if strings.Contains(got.YAML, "pin_function_mismatch") || strings.Contains(got.YAML, "ERROR") {
		t.Fatalf("proposal evaluated a rule:\n%s", got.YAML)
	}
	doc := mustProposalDoc(t, got.YAML)
	if doc.Connections[0].Net != "I2C_SCL" || doc.Connections[0].Status != "decided" {
		t.Fatalf("entry = %+v", doc.Connections[0])
	}
}

func i2cProposalDesign(mpuOnNet bool) *ir.DesignIR {
	sdaRef := "U3"
	if mpuOnNet {
		sdaRef = "U2"
	}
	return &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U1", MPN: "ESP32-WROOM-32", Value: "ESP32-WROOM-32"},
			{Ref: "U2", MPN: "MPU-6050", Value: "MPU-6050"},
			{Ref: "U3", MPN: "BNO055", Value: "BNO055"},
		},
		Nets: []ir.Net{
			{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: sdaRef, Pin: "SDA", Name: "SDA"}}},
			{Name: "LED", Pins: []ir.PinRef{{Ref: "U1", Pin: "33", Name: "IO21"}}},
			{Name: "+3V3", Pins: []ir.PinRef{{Ref: "U2", Pin: "13", Name: "VDD"}}},
		},
	}
}

func assertEveryGPIOCandidate(t *testing.T, mpn string, got []proposalCandidateDoc) {
	t.Helper()
	match := MatchPart(ir.Part{MPN: mpn, Value: mpn}, nil)
	if !match.Matched {
		t.Fatalf("expected %s", mpn)
	}
	var numbers []string
	namesByNumber := map[string][]string{}
	seenNumber := map[string]bool{}
	allNames := map[string]struct{}{}
	for _, fn := range match.Contract.PinFunctions {
		if fn.Kind != PinFunctionGPIOCandidate || !fn.Citation.cited() {
			continue
		}
		name := strings.TrimSpace(fn.Name)
		if name == "" {
			continue
		}
		allNames[name] = struct{}{}
		number := strings.TrimSpace(fn.Number)
		if !seenNumber[number] {
			seenNumber[number] = true
			numbers = append(numbers, number)
		}
		if !containsString(namesByNumber[number], name) {
			namesByNumber[number] = append(namesByNumber[number], name)
		}
	}
	if len(numbers) < 2 || len(got) != len(numbers) {
		t.Fatalf("%s candidates = %d, catalog pins = %d", mpn, len(got), len(numbers))
	}
	flat := map[string]struct{}{}
	for i, candidate := range got {
		if candidate.Number != numbers[i] || strings.Join(candidate.Pins, ",") != strings.Join(namesByNumber[numbers[i]], ",") {
			t.Fatalf("candidate %d = %+v, want number %s pins %v", i, candidate, numbers[i], namesByNumber[numbers[i]])
		}
		if candidate.Citation.Datasheet == "" || candidate.Citation.Revision == "" || (candidate.Citation.Table == "" && candidate.Citation.Section == "") {
			t.Fatalf("candidate %d missing citation: %+v", i, candidate)
		}
		for _, pin := range candidate.Pins {
			flat[pin] = struct{}{}
		}
	}
	if len(flat) != len(allNames) {
		t.Fatalf("%s listed %d gpio names, catalog has %d", mpn, len(flat), len(allNames))
	}
}

func mustProposalDoc(t *testing.T, raw string) proposalDoc {
	t.Helper()
	var doc proposalDoc
	dec := yaml.NewDecoder(strings.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode proposal: %v\n%s", err, raw)
	}
	return doc
}

type proposalDoc struct {
	Connections []proposalEntryDoc `yaml:"connections"`
}

type proposalEntryDoc struct {
	ID         string                 `yaml:"id"`
	Status     string                 `yaml:"status"`
	Signal     string                 `yaml:"signal"`
	Net        string                 `yaml:"net"`
	Ref        string                 `yaml:"ref"`
	MPN        string                 `yaml:"mpn"`
	Parts      []proposalPartDoc      `yaml:"parts"`
	Candidates []proposalCandidateDoc `yaml:"candidates"`
}

type proposalPartDoc struct {
	Ref      string              `yaml:"ref"`
	MPN      string              `yaml:"mpn"`
	Pin      string              `yaml:"pin"`
	Net      string              `yaml:"net"`
	Nets     []string            `yaml:"nets"`
	Citation proposalCitationDoc `yaml:"citation"`
}

type proposalCandidateDoc struct {
	Pins     []string            `yaml:"pins"`
	Number   string              `yaml:"number"`
	Citation proposalCitationDoc `yaml:"citation"`
}

type proposalCitationDoc struct {
	Datasheet string `yaml:"datasheet"`
	Revision  string `yaml:"revision"`
	Table     string `yaml:"table"`
	Section   string `yaml:"section"`
}

const decidedSDASnippet = `  - id: "sda-U2-U3"
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

const esp32SDAChoiceSnippet = `  - id: "sda-U1"
    status: needs_choice
    signal: SDA
    net: I2C_SDA
    ref: U1
    mpn: "ESP32-WROOM-32"
    candidates:
      - pins: [IO32, GPIO32]
        number: "8"
`

const esp32SCLChoiceSnippet = `  - id: "scl-U1"
    status: needs_choice
    signal: SCL
    ref: U1
    mpn: "ESP32-WROOM-32"
    candidates:
`
