package contracts_test

import (
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestDraftFromDesignEmptyAndPartialNets(t *testing.T) {
	doc := contracts.DraftFromDesign(nil)
	if len(doc.IDs) != 0 {
		t.Fatalf("expected no ids, got %v", doc.IDs)
	}
	if !strings.HasPrefix(doc.YAML, "# The user fills pin tokens and power_budget currents.\n") {
		t.Fatalf("missing draft comment:\n%s", doc.YAML)
	}
	if _, err := contracts.ParseYAML([]byte(doc.YAML), "contracts.draft.yaml"); err != nil {
		t.Fatalf("empty draft should validate: %v\n%s", err, doc.YAML)
	}

	plain := contracts.DraftFromDesign(&ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", Value: "1.5", Fields: map[string]string{"max_current_a": "3"}}},
		Nets:  []ir.Net{{Name: "PWR", Pins: []ir.PinRef{{Ref: "U1", Pin: "1", Name: "PB15"}}}},
	})
	if len(plain.IDs) != 0 {
		t.Fatalf("expected no contract for PWR, got %v\n%s", plain.IDs, plain.YAML)
	}
	if strings.Contains(plain.YAML, "PWR") || strings.Contains(plain.YAML, "PB15") || strings.Contains(strings.TrimPrefix(plain.YAML, "# The user fills pin tokens and power_budget currents.\n"), "power_budget") {
		t.Fatalf("draft invented a contract:\n%s", plain.YAML)
	}

	partial := contracts.DraftFromDesign(&ir.DesignIR{
		Nets: []ir.Net{
			{Name: "SPI_SCK", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
			{Name: "SPI_MOSI", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}}},
			{Name: "CANH", Pins: []ir.PinRef{{Ref: "U1", Pin: "3"}}},
			{Name: "SDA", Pins: []ir.PinRef{{Ref: "U1", Pin: "4"}}},
		},
	})
	if len(partial.IDs) != 0 {
		t.Fatalf("partial buses should not become contracts, got %v\n%s", partial.IDs, partial.YAML)
	}
}

func TestDraftFromDesignSPIMasterChipSelectAndSharedNet(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U1", Value: "MCU"},
			{Ref: "U2", Value: "PB15"},
			{Ref: "U3", Value: "0.4"},
		},
		Nets: []ir.Net{
			{Name: "SPI_SCK", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}, {Ref: "U3", Pin: "1"}}},
			{Name: "SPI_MOSI", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "2"}, {Ref: "U3", Pin: "2"}}},
			{Name: "SPI_MISO", Pins: []ir.PinRef{{Ref: "U1", Pin: "3"}, {Ref: "U2", Pin: "3"}, {Ref: "U3", Pin: "3"}}},
			{Name: "SHARED_CS", Pins: []ir.PinRef{{Ref: "U1", Pin: "4"}, {Ref: "U2", Pin: "4"}, {Ref: "U3", Pin: "4"}}},
			{Name: "PWR", Pins: []ir.PinRef{{Ref: "U2", Pin: "5"}}},
			{Name: "EXTRA", Pins: []ir.PinRef{{Ref: "U1", Pin: "6"}, {Ref: "U1", Pin: "7"}}},
		},
	}
	doc := contracts.DraftFromDesign(design)
	if strings.Join(doc.IDs, ",") != "spi" {
		t.Fatalf("ids = %v", doc.IDs)
	}
	loaded := mustParseDraft(t, doc.YAML)
	req := connectedReq(t, loaded, "spi")
	if len(req.Participants) != 3 || req.Participants[0].Ref != "U1" || req.Participants[0].Role != "master" {
		t.Fatalf("expected U1 master first, got %+v", req.Participants)
	}
	if req.Participants[1].Ref != "U2" || req.Participants[2].Ref != "U3" {
		t.Fatalf("expected slaves U2 U3, got %+v", req.Participants)
	}
	for _, participant := range req.Participants {
		if len(participant.Pins) != 0 {
			t.Fatalf("pins map should be omitted, got %+v", participant)
		}
	}
	if len(req.ChipSelects) != 2 {
		t.Fatalf("expected both slaves on SHARED_CS, got %+v", req.ChipSelects)
	}
	if req.ChipSelects[0] != (contracts.ChipSelect{Ref: "U2", Net: "SHARED_CS"}) || req.ChipSelects[1] != (contracts.ChipSelect{Ref: "U3", Net: "SHARED_CS"}) {
		t.Fatalf("chip selects = %+v", req.ChipSelects)
	}
	if strings.Contains(doc.YAML, "PB15") || strings.Contains(doc.YAML, "pins:") || strings.Contains(bodyAfterComment(doc.YAML), "power_budget") {
		t.Fatalf("draft guessed pins or power:\n%s", doc.YAML)
	}

	again := contracts.DraftFromDesign(design)
	if again.YAML != doc.YAML || strings.Join(again.IDs, ",") != strings.Join(doc.IDs, ",") {
		t.Fatal("draft changed across runs")
	}
}

func TestDraftFromDesignMasterIsMostOtherPins(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "SPI_SCK", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}}},
			{Name: "SPI_MOSI", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "2"}}},
			{Name: "SPI_MISO", Pins: []ir.PinRef{{Ref: "U1", Pin: "3"}, {Ref: "U2", Pin: "3"}}},
			{Name: "A", Pins: []ir.PinRef{{Ref: "U2", Pin: "4"}}},
			{Name: "B", Pins: []ir.PinRef{{Ref: "U2", Pin: "5"}}},
			{Name: "C", Pins: []ir.PinRef{{Ref: "U2", Pin: "6"}}},
			{Name: "D", Pins: []ir.PinRef{{Ref: "U1", Pin: "4"}}},
		},
	}
	req := connectedReq(t, mustParseDraft(t, contracts.DraftFromDesign(design).YAML), "spi")
	if req.Participants[0].Ref != "U2" || req.Participants[0].Role != "master" || req.Participants[1].Ref != "U1" {
		t.Fatalf("expected U2 master, got %+v", req.Participants)
	}
	if len(req.ChipSelects) != 0 {
		t.Fatalf("non-CS nets are not chip selects, got %+v", req.ChipSelects)
	}
}

func TestDraftFromDesignOmitsSlaveWithTwoChipSelectNets(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "SPI_SCK", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}, {Ref: "U3", Pin: "1"}}},
			{Name: "SPI_MOSI", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "2"}, {Ref: "U3", Pin: "2"}}},
			{Name: "SPI_MISO", Pins: []ir.PinRef{{Ref: "U1", Pin: "3"}, {Ref: "U2", Pin: "3"}, {Ref: "U3", Pin: "3"}}},
			{Name: "IMU_CS", Pins: []ir.PinRef{{Ref: "U2", Pin: "4"}, {Ref: "U3", Pin: "4"}}},
			{Name: "MAG_CS", Pins: []ir.PinRef{{Ref: "U2", Pin: "5"}}},
			{Name: "OTHER", Pins: []ir.PinRef{{Ref: "U1", Pin: "9"}, {Ref: "U1", Pin: "10"}, {Ref: "U1", Pin: "11"}}},
		},
	}
	req := connectedReq(t, mustParseDraft(t, contracts.DraftFromDesign(design).YAML), "spi")
	if req.Participants[0].Ref != "U1" || req.Participants[1].Ref != "U2" || req.Participants[1].Role != "slave" {
		t.Fatalf("expected U2 to stay a slave, got %+v", req.Participants)
	}
	if len(req.ChipSelects) != 1 || req.ChipSelects[0].Ref != "U3" || req.ChipSelects[0].Net != "IMU_CS" {
		t.Fatalf("U2 has two CS nets and should be omitted, got %+v", req.ChipSelects)
	}
}

func TestDraftFromDesignLeadingSlashAndCANCount(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "R1", Value: "10"},
			{Ref: "R2", Value: "4.7k"},
		},
		Nets: []ir.Net{
			{Name: "/SPI_SCK", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}}},
			{Name: "/SPI_MOSI", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "2"}}},
			{Name: "/SPI_MISO", Pins: []ir.PinRef{{Ref: "U1", Pin: "3"}, {Ref: "U2", Pin: "3"}}},
			{Name: "/IMU_CS", Pins: []ir.PinRef{{Ref: "U1", Pin: "4"}, {Ref: "U2", Pin: "4"}}},
			{Name: "/OTHER", Pins: []ir.PinRef{{Ref: "U1", Pin: "5"}}},
			{Name: "/CANH", Pins: []ir.PinRef{{Ref: "U1", Pin: "6"}, {Ref: "U9", Pin: "1"}, {Ref: "R1", Pin: "1"}}},
			{Name: "/CANL", Pins: []ir.PinRef{{Ref: "U1", Pin: "7"}, {Ref: "R1", Pin: "2"}}},
		},
	}
	doc := contracts.DraftFromDesign(design)
	if strings.Join(doc.IDs, ",") != "spi,can" {
		t.Fatalf("ids = %v\n%s", doc.IDs, doc.YAML)
	}
	loaded := mustParseDraft(t, doc.YAML)
	spi := connectedReq(t, loaded, "spi")
	if spi.Nets[0] != "/SPI_SCK" || spi.Nets[1] != "/SPI_MOSI" || spi.Nets[2] != "/SPI_MISO" {
		t.Fatalf("spi nets = %+v", spi.Nets)
	}
	if spi.Participants[0].Ref != "U1" || spi.Participants[0].Role != "master" {
		t.Fatalf("expected U1 master, got %+v", spi.Participants)
	}
	if len(spi.ChipSelects) != 1 || spi.ChipSelects[0].Ref != "U2" || spi.ChipSelects[0].Net != "/IMU_CS" {
		t.Fatalf("chip select = %+v", spi.ChipSelects)
	}
	can := connectedReq(t, loaded, "can")
	if can.Nets[0] != "/CANH" || can.Nets[1] != "/CANL" {
		t.Fatalf("can nets = %+v", can.Nets)
	}
	refs := make([]string, len(can.Participants))
	for i, participant := range can.Participants {
		refs[i] = participant.Ref + ":" + participant.Role
	}
	if strings.Join(refs, ",") != "U1:master,R1:slave,U9:slave" {
		t.Fatalf("can participants = %s", strings.Join(refs, ","))
	}
	term := terminatedReq(t, loaded, "can")
	if term.ResistanceOhms == nil || *term.ResistanceOhms != 120 || term.TerminatorCount == nil || *term.TerminatorCount != 2 {
		t.Fatalf("terminator bounds = ohms %v count %v", term.ResistanceOhms, term.TerminatorCount)
	}
	if strings.Contains(doc.YAML, "4.7k") || strings.Contains(doc.YAML, "count: 1") {
		t.Fatalf("count followed resistor values:\n%s", doc.YAML)
	}

	noResistors := contracts.DraftFromDesign(&ir.DesignIR{
		Nets: []ir.Net{
			{Name: "CANH", Pins: []ir.PinRef{{Ref: "U4", Pin: "1"}}},
			{Name: "CANL", Pins: []ir.PinRef{{Ref: "U4", Pin: "2"}}},
		},
	})
	term = terminatedReq(t, mustParseDraft(t, noResistors.YAML), "can")
	if term.TerminatorCount == nil || *term.TerminatorCount != 2 {
		t.Fatalf("count should stay 2 with no resistors, got %+v", term)
	}
}

func TestDraftFromDesignI2CPairs(t *testing.T) {
	design := &ir.DesignIR{
		Nets: []ir.Net{
			{Name: "/SDA", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}}},
			{Name: "SCL", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}}},
			{Name: "I2C_SDA", Pins: []ir.PinRef{{Ref: "U2", Pin: "1"}}},
			{Name: "/I2C_SCL", Pins: []ir.PinRef{{Ref: "U2", Pin: "2"}}},
		},
	}
	doc := contracts.DraftFromDesign(design)
	if strings.Join(doc.IDs, ",") != "i2c,i2c_bus" {
		t.Fatalf("ids = %v\n%s", doc.IDs, doc.YAML)
	}
	loaded := mustParseDraft(t, doc.YAML)
	if loaded[0].Scope.BusType != "i2c" || loaded[0].Scope.BusID != "i2c" || loaded[0].Scope.Nets == nil || loaded[0].Scope.Nets.SDA != "/SDA" || loaded[0].Scope.Nets.SCL != "SCL" {
		t.Fatalf("i2c scope = %+v", loaded[0].Scope)
	}
	if loaded[1].ID != "i2c_bus" || loaded[1].Scope.Nets == nil || loaded[1].Scope.Nets.SDA != "I2C_SDA" || loaded[1].Scope.Nets.SCL != "/I2C_SCL" {
		t.Fatalf("i2c_bus scope = %+v", loaded[1].Scope)
	}
	for _, contract := range loaded {
		if contract.Scope.BusType == "" || contract.Scope.BusID == "" {
			t.Fatalf("missing scope: %+v", contract.Scope)
		}
		seen := map[contracts.ContractType]bool{}
		for _, req := range contract.Requirements {
			seen[req.Type] = true
			if req.Severity != "ERROR" {
				t.Fatalf("severity = %s", req.Severity)
			}
			if req.Type == contracts.ContractPullupOhms && (req.MinOhms == nil || *req.MinOhms != 2200 || req.MaxOhms == nil || *req.MaxOhms != 10000) {
				t.Fatalf("pullup bounds = min %v max %v", req.MinOhms, req.MaxOhms)
			}
		}
		if !seen[contracts.ContractCommonGround] || !seen[contracts.ContractPullupOhms] {
			t.Fatalf("i2c requirements = %+v", contract.Requirements)
		}
		if seen[contracts.ContractPowerBudget] {
			t.Fatal("draft emitted power_budget")
		}
	}
	again := contracts.DraftFromDesign(design)
	if again.YAML != doc.YAML {
		t.Fatal("i2c draft changed across runs")
	}
}

func mustParseDraft(t *testing.T, yamlText string) []contracts.SystemContract {
	t.Helper()
	loaded, err := contracts.ParseYAML([]byte(yamlText), "contracts.draft.yaml")
	if err != nil {
		t.Fatalf("parse draft: %v\n%s", err, yamlText)
	}
	return loaded
}

func connectedReq(t *testing.T, loaded []contracts.SystemContract, id string) contracts.Requirement {
	t.Helper()
	return requireDraftReq(t, loaded, id, contracts.ContractConnected)
}

func terminatedReq(t *testing.T, loaded []contracts.SystemContract, id string) contracts.Requirement {
	t.Helper()
	return requireDraftReq(t, loaded, id, contracts.ContractTerminated)
}

func requireDraftReq(t *testing.T, loaded []contracts.SystemContract, id string, kind contracts.ContractType) contracts.Requirement {
	t.Helper()
	for _, contract := range loaded {
		if contract.ID != id {
			continue
		}
		if contract.Scope.BusType == "" || contract.Scope.BusID == "" {
			t.Fatalf("contract %s missing scope: %+v", id, contract.Scope)
		}
		for _, req := range contract.Requirements {
			if req.Type == kind {
				if req.Severity != "ERROR" {
					t.Fatalf("severity = %s", req.Severity)
				}
				return req
			}
		}
	}
	t.Fatalf("missing %s requirement on %s: %+v", kind, id, loaded)
	return contracts.Requirement{}
}

func bodyAfterComment(yamlText string) string {
	const comment = "# The user fills pin tokens and power_budget currents.\n"
	return strings.TrimPrefix(yamlText, comment)
}
