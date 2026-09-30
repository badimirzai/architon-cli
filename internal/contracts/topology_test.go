package contracts_test

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

const canTerminationContract = `
contracts:
  - id: can_bus
    scope:
      bus_type: can
      bus_id: vehicle_can
    require:
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: 120
        count: 2
    severity: error
`

const canConnectedAndTerminated = `
contracts:
  - id: can_bus
    scope:
      bus_type: can
      bus_id: vehicle_can
    require:
      connected:
        nets: [CANH, CANL]
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: 120
        count: 2
    severity: error
`

const spiChipSelectContract = `
contracts:
  - id: imu_spi
    scope:
      bus_type: spi
      bus_id: imu_spi
    require:
      connected:
        nets: [SPI_SCK, SPI_MOSI, SPI_MISO]
        chip_selects:
          - ref: U2
            net: IMU_CS
          - ref: U3
            net: MAG_CS
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
          - { ref: U3, role: slave }
    severity: error
`

const spiSharedChipSelectContract = `
contracts:
  - id: imu_spi
    scope:
      bus_type: spi
      bus_id: imu_spi
    require:
      connected:
        nets: [SPI_SCK, SPI_MOSI, SPI_MISO]
        chip_selects:
          - ref: U2
            net: IMU_CS
          - ref: U3
            net: IMU_CS
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
          - { ref: U3, role: slave }
    severity: error
`

func TestTerminatedContractParses(t *testing.T) {
	loaded, err := contracts.ParseYAML([]byte(canTerminationContract), "contracts.yaml")
	if err != nil {
		t.Fatalf("parse terminated contract: %v", err)
	}
	if len(loaded) != 1 || len(loaded[0].Requirements) != 1 {
		t.Fatalf("expected one terminated requirement, got %+v", loaded)
	}
	req := loaded[0].Requirements[0]
	if req.Type != contracts.ContractTerminated || loaded[0].Scope.BusType != "can" || loaded[0].Scope.BusID != "vehicle_can" {
		t.Fatalf("unexpected terminated requirement: %+v scope=%+v", req, loaded[0].Scope)
	}
	if len(req.Nets) != 2 || req.Nets[0] != "CANH" || req.Nets[1] != "CANL" {
		t.Fatalf("unexpected terminator nets: %+v", req.Nets)
	}
	if req.ResistanceOhms == nil || *req.ResistanceOhms != 120 || req.TerminatorCount == nil || *req.TerminatorCount != 2 {
		t.Fatalf("unexpected terminator bounds: ohms=%v count=%v", req.ResistanceOhms, req.TerminatorCount)
	}
}

func TestSPIChipSelectContractParses(t *testing.T) {
	loaded, err := contracts.ParseYAML([]byte(spiChipSelectContract), "contracts.yaml")
	if err != nil {
		t.Fatalf("parse chip-select contract: %v", err)
	}
	if len(loaded) != 1 || len(loaded[0].Requirements) != 1 {
		t.Fatalf("expected one connected requirement, got %+v", loaded)
	}
	req := loaded[0].Requirements[0]
	if req.Type != contracts.ContractConnected || len(req.Nets) != 3 || len(req.ChipSelects) != 2 {
		t.Fatalf("unexpected chip-select requirement: %+v", req)
	}
	if req.ChipSelects[0] != (contracts.ChipSelect{Ref: "U2", Net: "IMU_CS"}) || req.ChipSelects[1] != (contracts.ChipSelect{Ref: "U3", Net: "MAG_CS"}) {
		t.Fatalf("unexpected chip selects: %+v", req.ChipSelects)
	}
}

func TestTopologyContractRejectsBadShape(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "one terminator net",
			body: `
contracts:
  - id: can_bus
    scope: {bus_type: can, bus_id: vehicle_can}
    require:
      terminated:
        nets: [CANH]
        resistance_ohms: 120
        count: 2
    severity: error
`,
			wantErr: "terminated.nets must name exactly two nets",
		},
		{
			name: "duplicate terminator nets",
			body: `
contracts:
  - id: can_bus
    scope: {bus_type: can, bus_id: vehicle_can}
    require:
      terminated:
        nets: [CANH, CANH]
        resistance_ohms: 120
        count: 2
    severity: error
`,
			wantErr: "duplicated",
		},
		{
			name: "count must be positive",
			body: `
contracts:
  - id: can_bus
    scope: {bus_type: can, bus_id: vehicle_can}
    require:
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: 120
        count: 0
    severity: error
`,
			wantErr: "terminated.count must be > 0",
		},
		{
			name: "resistance must be a number",
			body: `
contracts:
  - id: can_bus
    scope: {bus_type: can, bus_id: vehicle_can}
    require:
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: "120"
        count: 2
    severity: error
`,
			wantErr: "terminated.resistance_ohms must be a number",
		},
		{
			name: "unknown terminated key",
			body: `
contracts:
  - id: can_bus
    scope: {bus_type: can, bus_id: vehicle_can}
    require:
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: 120
        count: 2
        impedance: 120
    severity: error
`,
			wantErr: "unknown terminated key",
		},
		{
			name: "chip selects require spi",
			body: `
contracts:
  - id: can_bus
    scope: {bus_type: can, bus_id: vehicle_can}
    require:
      connected:
        nets: [CANH, CANL]
        chip_selects:
          - { ref: U2, net: IMU_CS }
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
    severity: error
`,
			wantErr: "connected.chip_selects requires scope.bus_type spi",
		},
		{
			name: "chip select ref must be a slave",
			body: `
contracts:
  - id: imu_spi
    scope: {bus_type: spi, bus_id: imu_spi}
    require:
      connected:
        nets: [SPI_SCK]
        chip_selects:
          - { ref: U1, net: IMU_CS }
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
    severity: error
`,
			wantErr: "must be a slave participant",
		},
		{
			name: "duplicate chip select ref",
			body: `
contracts:
  - id: imu_spi
    scope: {bus_type: spi, bus_id: imu_spi}
    require:
      connected:
        nets: [SPI_SCK]
        chip_selects:
          - { ref: U2, net: IMU_CS }
          - { ref: U2, net: MAG_CS }
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
    severity: error
`,
			wantErr: "duplicated",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := contracts.ParseYAML([]byte(tt.body), "contracts.yaml")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}

	_, err := contracts.ParseYAML([]byte(spiSharedChipSelectContract), "contracts.yaml")
	if err != nil {
		t.Fatalf("two chip-select entries may name the same net: %v", err)
	}
}

func TestCANTerminationCount(t *testing.T) {
	t.Run("two terminators pass", func(t *testing.T) {
		design := canDesign(
			ir.Part{Value: "120"},
			ir.Part{Value: "120"},
		)
		if findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract)); len(findings) != 0 {
			t.Fatalf("expected two 120 ohm terminators to pass, got %+v", findings)
		}
	})

	t.Run("one terminator is termination_count_low", func(t *testing.T) {
		design := canDesign(ir.Part{Value: "120"})
		findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract))
		finding := requireRuleFinding(t, findings, contracts.RuleTerminationCountLow)
		if finding.Expected == nil || finding.Expected.Text != "2" || finding.Observed == nil || finding.Observed.Text != "1" {
			t.Fatalf("unexpected count evidence: %+v %+v", finding.Expected, finding.Observed)
		}
		if finding.BusID != "vehicle_can" || finding.BusType != "can" || finding.Severity != "ERROR" {
			t.Fatalf("unexpected termination finding: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RuleTerminationCountHigh) || hasRuleFinding(findings, contracts.RuleInterfaceNotConnected) {
			t.Fatalf("a low count should stay a count finding, got %+v", findings)
		}
	})

	t.Run("three terminators is termination_count_high", func(t *testing.T) {
		design := canDesign(
			ir.Part{Value: "120"},
			ir.Part{Value: "120"},
			ir.Part{Value: "120"},
		)
		findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract))
		finding := requireRuleFinding(t, findings, contracts.RuleTerminationCountHigh)
		if finding.Expected == nil || finding.Expected.Text != "2" || finding.Observed == nil || finding.Observed.Text != "3" {
			t.Fatalf("unexpected count evidence: %+v %+v", finding.Expected, finding.Observed)
		}
		if len(findings) != 1 {
			t.Fatalf("expected only the high count, got %+v", findings)
		}
	})

	t.Run("connected parts stay connected when the count is wrong", func(t *testing.T) {
		design := canDesign(ir.Part{Value: "120"})
		findings := contracts.Evaluate(design, userContractIR(t, design, canConnectedAndTerminated))
		finding := requireRuleFinding(t, findings, contracts.RuleTerminationCountLow)
		if finding.Expected == nil || finding.Expected.Text != "2" || finding.Observed == nil || finding.Observed.Text != "1" {
			t.Fatalf("unexpected count evidence: %+v %+v", finding.Expected, finding.Observed)
		}
		if hasRuleFinding(findings, contracts.RuleInterfaceNotConnected) || hasRuleFinding(findings, contracts.RuleInterfaceNetMissing) {
			t.Fatalf("bus connectivity should pass while the terminator count fails, got %+v", findings)
		}
	})

	t.Run("missing net is interface_net_missing", func(t *testing.T) {
		design := canDesign(ir.Part{Value: "120"}, ir.Part{Value: "120"})
		design.Nets = design.Nets[:1]
		findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract))
		finding := requireRuleFinding(t, findings, contracts.RuleInterfaceNetMissing)
		if finding.Net != "CANL" || finding.Observed == nil || finding.Observed.Text != "missing" {
			t.Fatalf("unexpected missing net finding: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RuleTerminationCountLow) || hasRuleFinding(findings, contracts.RuleTerminationCountHigh) {
			t.Fatalf("a missing net should not also be a count finding, got %+v", findings)
		}
	})

	t.Run("connected and terminated report one missing net", func(t *testing.T) {
		design := canDesign(ir.Part{Value: "120"}, ir.Part{Value: "120"})
		design.Nets = design.Nets[:1]
		findings := contracts.Evaluate(design, userContractIR(t, design, canConnectedAndTerminated))
		count := 0
		for _, finding := range findings {
			if finding.RuleID == contracts.RuleInterfaceNetMissing && finding.Net == "CANL" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected one missing CANL finding, got %+v", findings)
		}
	})

	t.Run("slash net names still match", func(t *testing.T) {
		design := canDesign(ir.Part{Value: "120"}, ir.Part{Value: "120"})
		design.Nets[0].Name = "/CANH"
		design.Nets[1].Name = "/CANL"
		if findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract)); len(findings) != 0 {
			t.Fatalf("expected /CANH and /CANL to satisfy CANH and CANL, got %+v", findings)
		}
	})

	t.Run("only a two-pin part across both nets counts", func(t *testing.T) {
		design := canDesign(ir.Part{Value: "120"})
		design.Parts = append(design.Parts,
			ir.Part{Ref: "R10", Value: "10k"},
			ir.Part{Ref: "R11", Value: "120"},
			ir.Part{Ref: "R12", Value: "120"},
		)
		design.Nets[0].Pins = append(design.Nets[0].Pins,
			ir.PinRef{Ref: "R10", Pin: "1"},
			ir.PinRef{Ref: "R11", Pin: "1"},
			ir.PinRef{Ref: "R12", Pin: "1"},
		)
		design.Nets[1].Pins = append(design.Nets[1].Pins,
			ir.PinRef{Ref: "R10", Pin: "2"},
			ir.PinRef{Ref: "R12", Pin: "2"},
		)
		design.Nets = append(design.Nets, ir.Net{Name: "GND", Pins: []ir.PinRef{
			{Ref: "R11", Pin: "2"},
			{Ref: "R12", Pin: "3"},
		}})
		findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract))
		finding := requireRuleFinding(t, findings, contracts.RuleTerminationCountLow)
		if finding.Observed == nil || finding.Observed.Text != "1" {
			t.Fatalf("10k, CANH-to-GND, and a three-pin part should not count, got %+v", findings)
		}
	})

	t.Run("resistance field or value can match", func(t *testing.T) {
		design := canDesign(
			ir.Part{Value: "can-term", Fields: map[string]string{"resistance_ohms": "120"}},
			ir.Part{Value: "120R"},
		)
		if findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract)); len(findings) != 0 {
			t.Fatalf("expected resistance_ohms and 120R to count, got %+v", findings)
		}
	})

	t.Run("pull-up field is not a terminator resistance", func(t *testing.T) {
		design := canDesign(
			ir.Part{Value: "10k", Fields: map[string]string{"pullup_ohms": "120"}},
			ir.Part{Value: "10k", Fields: map[string]string{"pullup_ohms": "120"}},
		)
		findings := contracts.Evaluate(design, userContractIR(t, design, canTerminationContract))
		finding := requireRuleFinding(t, findings, contracts.RuleTerminationCountLow)
		if finding.Observed == nil || finding.Observed.Text != "0" {
			t.Fatalf("pullup_ohms should not count as a terminator, got %+v", findings)
		}
	})
}

func TestSPIChipSelectExclusivity(t *testing.T) {
	t.Run("different chip-select nets pass", func(t *testing.T) {
		design := spiDesign([]string{"IMU_CS"}, []string{"MAG_CS"})
		if findings := contracts.Evaluate(design, userContractIR(t, design, spiChipSelectContract)); len(findings) != 0 {
			t.Fatalf("expected separate chip-select nets to pass, got %+v", findings)
		}
	})

	t.Run("two slaves on one chip-select net is spi_cs_shared", func(t *testing.T) {
		design := spiDesign([]string{"IMU_CS"}, []string{"IMU_CS"})
		findings := contracts.Evaluate(design, userContractIR(t, design, spiSharedChipSelectContract))
		finding := requireRuleFinding(t, findings, contracts.RuleSPICSShared)
		if finding.Net != "IMU_CS" || finding.Expected == nil || finding.Expected.Text != "IMU_CS" || finding.Observed == nil || finding.Observed.Text != "U2, U3" {
			t.Fatalf("unexpected shared chip-select evidence: %+v", finding)
		}
		if finding.BusID != "imu_spi" || finding.BusType != "spi" || finding.Severity != "ERROR" {
			t.Fatalf("unexpected shared chip-select finding: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RuleInterfaceNotConnected) || hasRuleFinding(findings, contracts.RuleInterfaceNetMissing) {
			t.Fatalf("shared chip-select should still be electrically connected, got %+v", findings)
		}
		if len(findings) != 1 {
			t.Fatalf("expected only spi_cs_shared, got %+v", findings)
		}
	})

	t.Run("slaves that land on one chip-select net are spi_cs_shared", func(t *testing.T) {
		design := spiDesign([]string{"IMU_CS"}, []string{"MAG_CS", "IMU_CS"})
		findings := contracts.Evaluate(design, userContractIR(t, design, spiChipSelectContract))
		finding := requireRuleFinding(t, findings, contracts.RuleSPICSShared)
		if finding.Expected == nil || finding.Expected.Text != "IMU_CS" || finding.Observed == nil || finding.Observed.Text != "U2, U3" {
			t.Fatalf("unexpected physical share evidence: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RuleInterfaceNotConnected) {
			t.Fatalf("each slave is on its own chip-select net, got %+v", findings)
		}
	})

	t.Run("slave missing its chip-select pin is interface_not_connected", func(t *testing.T) {
		design := spiDesign([]string{"IMU_CS"}, nil)
		design.Nets = append(design.Nets, ir.Net{Name: "MAG_CS", Pins: []ir.PinRef{{Ref: "U1", Pin: "10"}}})
		findings := contracts.Evaluate(design, userContractIR(t, design, spiChipSelectContract))
		finding := requireRuleFinding(t, findings, contracts.RuleInterfaceNotConnected)
		if finding.ComponentRef != "U3" || finding.Net != "MAG_CS" {
			t.Fatalf("unexpected missing chip-select finding: %+v", finding)
		}
		if finding.Expected == nil || finding.Expected.Text != "U3 connected to MAG_CS" || finding.Observed == nil || finding.Observed.Text != "U3 has no pin on MAG_CS" {
			t.Fatalf("unexpected chip-select evidence: %+v %+v", finding.Expected, finding.Observed)
		}
		if hasRuleFinding(findings, contracts.RuleSPICSShared) {
			t.Fatalf("one slave on MAG_CS is not shared, got %+v", findings)
		}
	})

	t.Run("missing chip-select net is interface_net_missing", func(t *testing.T) {
		design := spiDesign([]string{"IMU_CS"}, nil)
		findings := contracts.Evaluate(design, userContractIR(t, design, spiChipSelectContract))
		finding := requireRuleFinding(t, findings, contracts.RuleInterfaceNetMissing)
		if finding.Net != "MAG_CS" || finding.Observed == nil || finding.Observed.Text != "missing" {
			t.Fatalf("unexpected missing chip-select net: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RuleInterfaceNotConnected) {
			t.Fatalf("a missing chip-select net should not also be not-connected, got %+v", findings)
		}
	})
}

func TestEnabledRuleIDsIncludeTopologyRules(t *testing.T) {
	enabled := map[string]struct{}{}
	for _, id := range contracts.EnabledRuleIDs() {
		enabled[id] = struct{}{}
	}
	for _, id := range []string{
		contracts.RuleTerminationCountLow,
		contracts.RuleTerminationCountHigh,
		contracts.RuleSPICSShared,
	} {
		if _, ok := enabled[id]; !ok {
			t.Fatalf("enabled rules missing %s", id)
		}
	}
}

func canDesign(terminators ...ir.Part) *ir.DesignIR {
	canH := []ir.PinRef{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}}
	canL := []ir.PinRef{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "2"}}
	parts := []ir.Part{{Ref: "U1", Value: "MCU"}, {Ref: "U2", Value: "TRANSCEIVER"}}
	for i, term := range terminators {
		if term.Ref == "" {
			term.Ref = fmt.Sprintf("R%d", i+1)
		}
		parts = append(parts, term)
		canH = append(canH, ir.PinRef{Ref: term.Ref, Pin: "1"})
		canL = append(canL, ir.PinRef{Ref: term.Ref, Pin: "2"})
	}
	return &ir.DesignIR{
		Parts: parts,
		Nets: []ir.Net{
			{Name: "CANH", Pins: canH},
			{Name: "CANL", Pins: canL},
		},
	}
}

func spiDesign(u2CS []string, u3CS []string) *ir.DesignIR {
	nets := []ir.Net{
		{Name: "SPI_SCK", Pins: []ir.PinRef{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}, {Ref: "U3", Pin: "1"}}},
		{Name: "SPI_MOSI", Pins: []ir.PinRef{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "2"}, {Ref: "U3", Pin: "2"}}},
		{Name: "SPI_MISO", Pins: []ir.PinRef{{Ref: "U1", Pin: "3"}, {Ref: "U2", Pin: "3"}, {Ref: "U3", Pin: "3"}}},
	}
	csPins := map[string][]ir.PinRef{}
	add := func(ref string, pin string, names []string) {
		for i, name := range names {
			csPins[name] = append(csPins[name], ir.PinRef{Ref: ref, Pin: pin + strconv.Itoa(i)})
		}
	}
	seen := map[string]struct{}{}
	for _, name := range append(append([]string{}, u2CS...), u3CS...) {
		seen[name] = struct{}{}
	}
	masterNets := make([]string, 0, len(seen))
	for name := range seen {
		masterNets = append(masterNets, name)
	}
	sort.Strings(masterNets)
	add("U1", "10", masterNets)
	add("U2", "4", u2CS)
	add("U3", "5", u3CS)
	names := make([]string, 0, len(csPins))
	for name := range csPins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		nets = append(nets, ir.Net{Name: name, Pins: csPins[name]})
	}
	return &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "U1", Value: "MCU"},
			{Ref: "U2", Value: "IMU"},
			{Ref: "U3", Value: "MAG"},
		},
		Nets: nets,
	}
}
