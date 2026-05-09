package contracts_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestContractsYAMLValidParses(t *testing.T) {
	loaded, err := contracts.ParseYAML([]byte(`
contracts:
  - id: i2c_policy
    description: I2C team policy
    scope:
      bus_type: i2c
      bus_id: i2c_main
      nets:
        sda: I2C_SDA
        scl: I2C_SCL
      rail: +3V3
    require:
      common_ground: true
      pullup_ohms:
        min: 2200
        max: 10000
      voltage_compatible: true
      current_budget:
        max_utilization_pct: 80
      no_i2c_address_conflict: true
    severity: error
`), "contracts.yaml")
	if err != nil {
		t.Fatalf("parse valid yaml: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected one contract, got %+v", loaded)
	}
	if loaded[0].ID != "i2c_policy" || loaded[0].SourceKind != contracts.ContractSourceUserYAML {
		t.Fatalf("unexpected loaded contract: %+v", loaded[0])
	}
	if len(loaded[0].Requirements) != 5 {
		t.Fatalf("expected five normalized requirements, got %+v", loaded[0].Requirements)
	}
	if loaded[0].Scope.BusID != "i2c_main" || loaded[0].Scope.Nets == nil || loaded[0].Scope.Nets.SDA != "I2C_SDA" || loaded[0].Scope.Nets.SCL != "I2C_SCL" {
		t.Fatalf("expected explicit bus scope, got %+v", loaded[0].Scope)
	}
}

func TestContractsYAMLInvalidSchemaFails(t *testing.T) {
	_, err := contracts.ParseYAML([]byte(`
contracts:
  - id: bad
    unknown: true
    require:
      common_ground: true
    severity: severe
`), "contracts.yaml")
	if err == nil {
		t.Fatal("expected invalid schema error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected strict schema error, got %v", err)
	}
}

func TestContractsYAMLStrictValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "missing id fails",
			body: `
contracts:
  - scope:
      bus_type: i2c
    require:
      no_i2c_address_conflict: true
    severity: error
`,
			wantErr: "id is required",
		},
		{
			name: "duplicate id fails",
			body: `
contracts:
  - id: dup
    scope: {bus_type: i2c}
    require: {no_i2c_address_conflict: true}
    severity: error
  - id: dup
    scope: {bus_type: i2c}
    require: {no_i2c_address_conflict: true}
    severity: warn
`,
			wantErr: "duplicated",
		},
		{
			name: "missing severity fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require: {no_i2c_address_conflict: true}
`,
			wantErr: "severity is required",
		},
		{
			name: "invalid severity fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require: {no_i2c_address_conflict: true}
    severity: severe
`,
			wantErr: "severity must be one of",
		},
		{
			name: "empty scope fails",
			body: `
contracts:
  - id: bad
    scope: {}
    require: {no_i2c_address_conflict: true}
    severity: error
`,
			wantErr: "scope must set at least one selector",
		},
		{
			name: "unknown scope key fails",
			body: `
contracts:
  - id: bad
    scope:
      bus_type: i2c
      clock: fast
    require: {no_i2c_address_conflict: true}
    severity: error
`,
			wantErr: "unknown scope key",
		},
		{
			name: "invalid bus type fails",
			body: `
contracts:
  - id: bad
    scope:
      bus_type: spi
    require: {no_i2c_address_conflict: true}
    severity: error
`,
			wantErr: "scope.bus_type must be i2c",
		},
		{
			name: "unknown explicit nets key fails",
			body: `
contracts:
  - id: bad
    scope:
      bus_type: i2c
      nets:
        sda: I2C_SDA
        scl: I2C_SCL
        alert: I2C_ALERT
    require: {no_i2c_address_conflict: true}
    severity: error
`,
			wantErr: "unknown scope.nets key",
		},
		{
			name: "empty require fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require: {}
    severity: error
`,
			wantErr: "require must set at least one enabled requirement",
		},
		{
			name: "unknown requirement key fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require:
      i2c_rise_time:
        max_ns: 1000
    severity: error
`,
			wantErr: "unknown requirement key",
		},
		{
			name: "pullup no min max fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require:
      pullup_ohms: {}
    severity: error
`,
			wantErr: "pullup_ohms must set min or max",
		},
		{
			name: "pullup min greater than max fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require:
      pullup_ohms:
        min: 10000
        max: 2200
    severity: error
`,
			wantErr: "pullup_ohms.min must be <= pullup_ohms.max",
		},
		{
			name: "pullup min nonpositive fails",
			body: `
contracts:
  - id: bad
    scope: {bus_type: i2c}
    require:
      pullup_ohms:
        min: 0
    severity: error
`,
			wantErr: "pullup_ohms.min must be > 0",
		},
		{
			name: "current budget missing utilization fails",
			body: `
contracts:
  - id: bad
    scope: {rail: +3V3}
    require:
      current_budget: {}
    severity: error
`,
			wantErr: "current_budget.max_utilization_pct is required",
		},
		{
			name: "current budget utilization over 100 fails",
			body: `
contracts:
  - id: bad
    scope: {rail: +3V3}
    require:
      current_budget:
        max_utilization_pct: 120
    severity: error
`,
			wantErr: "current_budget.max_utilization_pct must be > 0 and <= 100",
		},
		{
			name: "explicit nets missing scl fails",
			body: `
contracts:
  - id: bad
    scope:
      bus_type: i2c
      nets:
        sda: I2C_SDA
    require: {no_i2c_address_conflict: true}
    severity: error
`,
			wantErr: "scope.nets.sda and scope.nets.scl are required",
		},
		{
			name: "invalid id fails",
			body: `
contracts:
  - id: i2c policy
    scope: {bus_type: i2c}
    require: {no_i2c_address_conflict: true}
    severity: error
`,
			wantErr: "id must match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := contracts.ParseYAML([]byte(tt.body), "contracts.yaml")
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestContractsYAMLExplicitNetsPass(t *testing.T) {
	_, err := contracts.ParseYAML([]byte(`
contracts:
  - id: i2c_policy
    scope:
      bus_type: i2c
      bus_id: i2c_main
      nets:
        sda: I2C_SDA
        scl: I2C_SCL
    require:
      no_i2c_address_conflict: true
    severity: info
`), "contracts.yaml")
	if err != nil {
		t.Fatalf("expected explicit nets to pass, got %v", err)
	}
}

func TestUserYAMLPullupMissingTriggersFinding(t *testing.T) {
	design := i2cDesign(nil)
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_pullups
    scope:
      bus_type: i2c
      rail: +3V3
    require:
      pullup_ohms:
        min: 2200
        max: 10000
    severity: error
`)

	findings := contracts.Evaluate(design, contractIR)
	finding := requireContractFinding(t, findings, contracts.ContractPullupOhms)
	if finding.ContractID != "i2c_pullups" || finding.ContractSource != contracts.ContractSourceUserYAML {
		t.Fatalf("expected user yaml provenance, got %+v", finding)
	}
	if finding.ContractFile != "contracts.yaml" || finding.Requirement != string(contracts.ContractPullupOhms) {
		t.Fatalf("expected contract file and requirement, got %+v", finding)
	}
}

func TestUserYAMLPullupBelowMinTriggersFinding(t *testing.T) {
	design := i2cDesign(map[string]float64{"R1": 1000, "R2": 1000})
	contractIR := userContractIR(t, design, pullupPolicyYAML(2200, 10000))

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractPullupOhms)
	if !strings.Contains(finding.Message, "Observed: effective pull-up on I2C_SDA is 1k. Expected: 2.2k to 10k.") {
		t.Fatalf("expected below-min pullup finding, got %+v", finding)
	}
}

func TestUserYAMLPullupAboveMaxTriggersFinding(t *testing.T) {
	design := i2cDesign(map[string]float64{"R1": 20000, "R2": 20000})
	contractIR := userContractIR(t, design, pullupPolicyYAML(2200, 10000))

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractPullupOhms)
	if !strings.Contains(finding.Message, "Observed: effective pull-up on I2C_SDA is 20k. Expected: 2.2k to 10k.") {
		t.Fatalf("expected above-max pullup finding, got %+v", finding)
	}
}

func TestUserYAMLCommonGroundMissingTriggersFinding(t *testing.T) {
	design := i2cDesign(map[string]float64{"R1": 4700, "R2": 4700})
	design.Nets = append(design.Nets, ir.Net{
		Name: "GND",
		Pins: []ir.PinRef{{Ref: "U1", Pin: "GND", Name: "GND"}},
	})
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_ground
    scope:
      bus_type: i2c
    require:
      common_ground: true
    severity: error
`)

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractCommonGround)
	if finding.ComponentRef != "U2" {
		t.Fatalf("expected U2 missing ground finding, got %+v", finding)
	}
}

func TestUserYAMLI2CAddressConflictTriggersFinding(t *testing.T) {
	design := i2cDesign(map[string]float64{"R1": 4700, "R2": 4700})
	design.Parts[0].Fields["i2c_address"] = "0x68"
	design.Parts[1].Fields["i2c_address"] = "104"
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_addresses
    scope:
      bus_type: i2c
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractNoI2CAddressConflict)
	if !strings.Contains(finding.Message, "0x68") {
		t.Fatalf("expected duplicate address in finding, got %+v", finding)
	}
}

func TestUserYAMLI2CAddressNormalization(t *testing.T) {
	tests := []struct {
		name        string
		addressA    string
		addressB    string
		wantFinding bool
	}{
		{name: "hex and decimal collide", addressA: "0x68", addressB: "104", wantFinding: true},
		{name: "uppercase hex and decimal collide", addressA: "0X68", addressB: "104", wantFinding: true},
		{name: "h suffix and decimal collide", addressA: "68h", addressB: "104", wantFinding: true},
		{name: "different addresses do not collide", addressA: "0x68", addressB: "0x69", wantFinding: false},
		{name: "invalid address does not crash", addressA: "not-an-address", addressB: "0x68", wantFinding: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			design := i2cDesign(map[string]float64{"R1": 4700, "R2": 4700})
			design.Parts[0].Fields["i2c_address"] = tt.addressA
			design.Parts[1].Fields["i2c_address"] = tt.addressB
			contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_addresses
    scope:
      bus_type: i2c
    require:
      no_i2c_address_conflict: true
    severity: error
`)

			findings := contracts.Evaluate(design, contractIR)
			hasFinding := hasContractFinding(findings, contracts.ContractNoI2CAddressConflict)
			if hasFinding != tt.wantFinding {
				t.Fatalf("expected finding=%v, got %+v", tt.wantFinding, findings)
			}
			if tt.wantFinding {
				finding := requireContractFinding(t, findings, contracts.ContractNoI2CAddressConflict)
				if !strings.Contains(finding.Message, "0x68") || strings.Contains(finding.Message, "0X68") {
					t.Fatalf("expected canonical lowercase hex address, got %+v", finding)
				}
			}
		})
	}
}

func TestUserYAMLI2CExplicitBusScoping(t *testing.T) {
	design := twoBusI2CDesign(map[string]string{
		"U1": "0x68",
		"U2": "0x69",
		"U3": "0x68",
	})

	separateBusIR := userContractIR(t, design, `
contracts:
  - id: i2c_main
    scope:
      bus_type: i2c
      bus_id: i2c_main
      nets:
        sda: I2C_MAIN_SDA
        scl: I2C_MAIN_SCL
    require:
      no_i2c_address_conflict: true
    severity: error
  - id: i2c_aux
    scope:
      bus_type: i2c
      bus_id: i2c_aux
      nets:
        sda: I2C_AUX_SDA
        scl: I2C_AUX_SCL
    require:
      no_i2c_address_conflict: true
    severity: error
`)
	if findings := contracts.Evaluate(design, separateBusIR); len(findings) != 0 {
		t.Fatalf("expected same address on different explicit buses to pass, got %+v", findings)
	}

	sameBusDesign := twoBusI2CDesign(map[string]string{
		"U1": "0x68",
		"U2": "0x68",
		"U3": "0x68",
	})
	sameBusIR := userContractIR(t, sameBusDesign, `
contracts:
  - id: i2c_main
    scope:
      bus_type: i2c
      bus_id: i2c_main
      nets:
        sda: I2C_MAIN_SDA
        scl: I2C_MAIN_SCL
    require:
      no_i2c_address_conflict: true
    severity: error
`)
	finding := requireContractFinding(t, contracts.Evaluate(sameBusDesign, sameBusIR), contracts.ContractNoI2CAddressConflict)
	if finding.BusID != "i2c_main" || finding.BusType != "i2c" {
		t.Fatalf("expected explicit bus fields, got %+v", finding)
	}
	if finding.BusNets == nil || finding.BusNets.SDA != "I2C_MAIN_SDA" || finding.BusNets.SCL != "I2C_MAIN_SCL" {
		t.Fatalf("expected explicit bus nets, got %+v", finding)
	}
	if !strings.Contains(finding.Message, "on bus i2c_main") {
		t.Fatalf("expected bus id in finding message, got %+v", finding)
	}
}

func TestUserYAMLI2CExplicitMissingNetsTriggerFinding(t *testing.T) {
	design := i2cDesign(nil)
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_pullup_policy
    scope:
      bus_type: i2c
      nets:
        sda: DOES_NOT_EXIST
        scl: ALSO_MISSING
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	findings := contracts.Evaluate(design, contractIR)
	if len(findings) != 2 {
		t.Fatalf("expected missing SDA/SCL findings, got %+v", findings)
	}
	for _, want := range []string{"DOES_NOT_EXIST", "ALSO_MISSING"} {
		found := false
		for _, finding := range findings {
			if finding.Net == want && strings.Contains(finding.Message, "Contract i2c_pullup_policy references missing net "+want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected missing net finding for %s, got %+v", want, findings)
		}
	}
}

func TestUserYAMLI2CExplicitNetsMatchKiCadLeadingSlash(t *testing.T) {
	design := i2cDesign(nil)
	design.Nets[0].Name = "/I2C_SDA"
	design.Nets[1].Name = "/I2C_SCL"
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_pullup_policy
    scope:
      bus_type: i2c
      nets:
        sda: I2C_SDA
        scl: I2C_SCL
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	if findings := contracts.Evaluate(design, contractIR); len(findings) != 0 {
		t.Fatalf("expected slash-prefixed IR nets to match explicit scope, got %+v", findings)
	}
}

func TestUserYAMLI2CExplicitNetsMatchContractLeadingSlash(t *testing.T) {
	design := i2cDesign(nil)
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_pullup_policy
    scope:
      bus_type: i2c
      nets:
        sda: /I2C_SDA
        scl: /I2C_SCL
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	if findings := contracts.Evaluate(design, contractIR); len(findings) != 0 {
		t.Fatalf("expected slash-prefixed contract nets to match IR scope, got %+v", findings)
	}
}

func TestUserYAMLVoltageCompatibleTriggersFinding(t *testing.T) {
	design := i2cDesign(map[string]float64{"R1": 4700, "R2": 4700})
	design.Parts[1].Fields["voltage_max_v"] = "3.6"
	design.Nets = append(design.Nets, ir.Net{
		Name: "+5V",
		Pins: []ir.PinRef{{Ref: "R1", Pin: "2"}, {Ref: "R2", Pin: "2"}},
	})
	contractIR := userContractIR(t, design, `
contracts:
  - id: i2c_voltage
    scope:
      bus_type: i2c
      rail: +5V
    require:
      voltage_compatible: true
    severity: error
`)
	contractIR.PutNet("+5V", contracts.NetContract{VoltageNominal: contracts.Float64(5.0)})

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractVoltageCompatible)
	if finding.ComponentRef != "U2" {
		t.Fatalf("expected U2 voltage compatibility finding, got %+v", finding)
	}
}

func TestUserYAMLPullupDetection(t *testing.T) {
	tests := []struct {
		name        string
		minOhms     float64
		maxOhms     float64
		resistors   []pullupFixture
		wantFinding bool
		wantMessage string
	}{
		{
			name:      "4.7k pull-up passes",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "R1", value: "4.7k", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:      "4700 parser works",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "R1", value: "4700", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:      "4k7 parser works",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "R1", value: "4k7", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:      "4700R parser works",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "R1", value: "4700R", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:      "resistance_ohms field parser works",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "RPULL", field: "4700", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:      "2200 ohm symbol parser works",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "R1", value: "2200Ω", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:      "2200 ohm suffix parser works",
			minOhms:   2200,
			maxOhms:   10000,
			resistors: []pullupFixture{{ref: "R1", value: "2200 ohm", netA: "I2C_SDA", netB: "+3V3"}},
		},
		{
			name:        "1k below min fails",
			minOhms:     2200,
			maxOhms:     10000,
			resistors:   []pullupFixture{{ref: "R1", value: "1k", netA: "I2C_SDA", netB: "+3V3"}},
			wantFinding: true,
			wantMessage: "Observed: effective pull-up on I2C_SDA is 1k. Expected: 2.2k to 10k.",
		},
		{
			name:        "20k above max fails",
			minOhms:     2200,
			maxOhms:     10000,
			resistors:   []pullupFixture{{ref: "R1", value: "20k", netA: "I2C_SDA", netB: "+3V3"}},
			wantFinding: true,
			wantMessage: "Observed: effective pull-up on I2C_SDA is 20k. Expected: 2.2k to 10k.",
		},
		{
			name:    "two 10k pull-ups compute effective 5k",
			minOhms: 4000,
			maxOhms: 6000,
			resistors: []pullupFixture{
				{ref: "R1", value: "10k", netA: "I2C_SDA", netB: "+3V3"},
				{ref: "R2", value: "10k", netA: "I2C_SDA", netB: "+3V3"},
			},
		},
		{
			name:        "pull-down to GND ignored",
			minOhms:     2200,
			maxOhms:     10000,
			resistors:   []pullupFixture{{ref: "R1", value: "4.7k", netA: "I2C_SDA", netB: "GND"}},
			wantFinding: true,
			wantMessage: "Observed: R1 = 4.7k connects I2C_SDA to GND. Expected: pull-up resistor between 2.2k and 10k to a compatible positive rail.",
		},
		{
			name:        "resistor between SDA and SCL ignored",
			minOhms:     2200,
			maxOhms:     10000,
			resistors:   []pullupFixture{{ref: "R1", value: "4.7k", netA: "I2C_SDA", netB: "I2C_SCL"}},
			wantFinding: true,
			wantMessage: "Observed: R1 = 4.7k connects I2C_SDA to I2C_SCL. Expected: pull-up resistor between 2.2k and 10k to a compatible positive rail.",
		},
		{
			name:        "no pull-up fails",
			minOhms:     2200,
			maxOhms:     10000,
			wantFinding: true,
			wantMessage: "Observed: no pull-up resistor found on net I2C_SDA. Expected: pull-up resistor between 2.2k and 10k to a compatible positive rail.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			design := pullupTestDesign(tt.resistors)
			contractIR := userContractIR(t, design, pullupNetPolicyYAML(tt.minOhms, tt.maxOhms))
			findings := contracts.Evaluate(design, contractIR)
			hasFinding := hasContractFinding(findings, contracts.ContractPullupOhms)
			if hasFinding != tt.wantFinding {
				t.Fatalf("expected pullup finding=%v, got %+v", tt.wantFinding, findings)
			}
			if tt.wantFinding {
				finding := requireContractFinding(t, findings, contracts.ContractPullupOhms)
				if !strings.Contains(finding.Message, tt.wantMessage) {
					t.Fatalf("expected message containing %q, got %+v", tt.wantMessage, finding)
				}
				if finding.Net != "I2C_SDA" {
					t.Fatalf("expected net field I2C_SDA, got %+v", finding)
				}
				if strings.Contains(tt.wantMessage, "effective") && finding.EffectivePullupOhms == nil {
					t.Fatalf("expected effective pullup field, got %+v", finding)
				}
			}
		})
	}
}

func TestUserYAMLCurrentBudgetTriggersFinding(t *testing.T) {
	design := &ir.DesignIR{
		Parts: []ir.Part{
			{Ref: "REG", Fields: map[string]string{"architon_current_budget_a": "1.0"}},
			{Ref: "U1", Fields: map[string]string{"load_current_a": "0.9"}},
		},
		Nets: []ir.Net{
			{Name: "+3V3", Pins: []ir.PinRef{{Ref: "REG", Pin: "OUT"}, {Ref: "U1", Pin: "VCC"}}},
		},
	}
	contractIR := userContractIR(t, design, `
contracts:
  - id: rail_budget
    scope:
      rail: +3V3
    require:
      current_budget:
        max_utilization_pct: 80
    severity: error
`)

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractCurrentBudget)
	if !strings.Contains(finding.Message, "90.0%") {
		t.Fatalf("expected current utilization finding, got %+v", finding)
	}
}

func pullupPolicyYAML(minOhms float64, maxOhms float64) string {
	return `
contracts:
  - id: i2c_pullups
    scope:
      bus_type: i2c
      rail: +3V3
    require:
      pullup_ohms:
        min: ` + trimFloat(minOhms) + `
        max: ` + trimFloat(maxOhms) + `
    severity: error
`
}

func pullupNetPolicyYAML(minOhms float64, maxOhms float64) string {
	return `
contracts:
  - id: i2c_pullups
    scope:
      bus_type: i2c
      net: I2C_SDA
      rail: +3V3
    require:
      pullup_ohms:
        min: ` + trimFloat(minOhms) + `
        max: ` + trimFloat(maxOhms) + `
    severity: error
`
}

type pullupFixture struct {
	ref   string
	value string
	field string
	netA  string
	netB  string
}

func i2cDesign(pullups map[string]float64) *ir.DesignIR {
	parts := []ir.Part{
		{Ref: "U1", Fields: map[string]string{}},
		{Ref: "U2", Fields: map[string]string{}},
	}
	nets := []ir.Net{
		{Name: "I2C_SDA", Pins: []ir.PinRef{
			{Ref: "U1", Pin: "SDA", Name: "SDA"},
			{Ref: "U2", Pin: "SDA", Name: "SDA"},
		}},
		{Name: "I2C_SCL", Pins: []ir.PinRef{
			{Ref: "U1", Pin: "SCL", Name: "SCL"},
			{Ref: "U2", Pin: "SCL", Name: "SCL"},
		}},
		{Name: "+3V3"},
	}
	if pullups != nil {
		parts = append(parts,
			ir.Part{Ref: "R1", Value: trimFloat(pullups["R1"]), Fields: map[string]string{"resistance_ohms": trimFloat(pullups["R1"])}},
			ir.Part{Ref: "R2", Value: trimFloat(pullups["R2"]), Fields: map[string]string{"resistance_ohms": trimFloat(pullups["R2"])}},
		)
		nets[0].Pins = append(nets[0].Pins, ir.PinRef{Ref: "R1", Pin: "1"})
		nets[1].Pins = append(nets[1].Pins, ir.PinRef{Ref: "R2", Pin: "1"})
		nets[2].Pins = append(nets[2].Pins, ir.PinRef{Ref: "R1", Pin: "2"}, ir.PinRef{Ref: "R2", Pin: "2"})
	}
	return &ir.DesignIR{
		Version: ir.SchemaVersion,
		Parts:   parts,
		Nets:    nets,
	}
}

func pullupTestDesign(resistors []pullupFixture) *ir.DesignIR {
	parts := []ir.Part{
		{Ref: "U1", Fields: map[string]string{}},
		{Ref: "U2", Fields: map[string]string{}},
	}
	nets := []ir.Net{
		{Name: "I2C_SDA", Pins: []ir.PinRef{
			{Ref: "U1", Pin: "SDA", Name: "SDA"},
			{Ref: "U2", Pin: "SDA", Name: "SDA"},
		}},
		{Name: "I2C_SCL", Pins: []ir.PinRef{
			{Ref: "U1", Pin: "SCL", Name: "SCL"},
			{Ref: "U2", Pin: "SCL", Name: "SCL"},
		}},
		{Name: "+3V3"},
		{Name: "GND"},
	}
	netIndex := map[string]int{
		"I2C_SDA": 0,
		"I2C_SCL": 1,
		"+3V3":    2,
		"GND":     3,
	}
	for _, resistor := range resistors {
		fields := map[string]string{}
		if resistor.field != "" {
			fields["resistance_ohms"] = resistor.field
		}
		parts = append(parts, ir.Part{Ref: resistor.ref, Value: resistor.value, Fields: fields})
		for _, netName := range []string{resistor.netA, resistor.netB} {
			idx, ok := netIndex[netName]
			if !ok {
				nets = append(nets, ir.Net{Name: netName})
				idx = len(nets) - 1
				netIndex[netName] = idx
			}
			pin := "1"
			if netName == resistor.netB {
				pin = "2"
			}
			nets[idx].Pins = append(nets[idx].Pins, ir.PinRef{Ref: resistor.ref, Pin: pin})
		}
	}
	return &ir.DesignIR{
		Version: ir.SchemaVersion,
		Parts:   parts,
		Nets:    nets,
	}
}

func twoBusI2CDesign(addresses map[string]string) *ir.DesignIR {
	part := func(ref string) ir.Part {
		fields := map[string]string{}
		if address := strings.TrimSpace(addresses[ref]); address != "" {
			fields["i2c_address"] = address
		}
		return ir.Part{Ref: ref, Fields: fields}
	}
	return &ir.DesignIR{
		Version: ir.SchemaVersion,
		Parts: []ir.Part{
			part("U1"),
			part("U2"),
			part("U3"),
		},
		Nets: []ir.Net{
			{Name: "I2C_MAIN_SDA", Pins: []ir.PinRef{
				{Ref: "U1", Pin: "SDA", Name: "SDA"},
				{Ref: "U2", Pin: "SDA", Name: "SDA"},
			}},
			{Name: "I2C_MAIN_SCL", Pins: []ir.PinRef{
				{Ref: "U1", Pin: "SCL", Name: "SCL"},
				{Ref: "U2", Pin: "SCL", Name: "SCL"},
			}},
			{Name: "I2C_AUX_SDA", Pins: []ir.PinRef{
				{Ref: "U3", Pin: "SDA", Name: "SDA"},
			}},
			{Name: "I2C_AUX_SCL", Pins: []ir.PinRef{
				{Ref: "U3", Pin: "SCL", Name: "SCL"},
			}},
			{Name: "GND", Pins: []ir.PinRef{
				{Ref: "U1", Pin: "GND", Name: "GND"},
				{Ref: "U2", Pin: "GND", Name: "GND"},
				{Ref: "U3", Pin: "GND", Name: "GND"},
			}},
		},
	}
}

func userContractIR(t *testing.T, design *ir.DesignIR, yamlText string) *contracts.ContractIR {
	t.Helper()
	loaded, err := contracts.ParseYAML([]byte(yamlText), "contracts.yaml")
	if err != nil {
		t.Fatalf("parse yaml: %v", err)
	}
	contractIR, err := contracts.NewUserYAMLSource("contracts.yaml", loaded).Enrich(design)
	if err != nil {
		t.Fatalf("enrich user yaml: %v", err)
	}
	contractIR.PutNet("+3V3", contracts.NetContract{VoltageNominal: contracts.Float64(3.3)})
	return contractIR
}

func requireContractFinding(t *testing.T, findings []contracts.Finding, typ contracts.ContractType) contracts.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == string(typ) {
			if finding.ContractID == "" || finding.ContractSource == "" || finding.Requirement == "" || finding.Fix == "" {
				t.Fatalf("expected v0.4 finding fields, got %+v", finding)
			}
			return finding
		}
	}
	t.Fatalf("expected %s finding, got %+v", typ, findings)
	return contracts.Finding{}
}

func hasContractFinding(findings []contracts.Finding, typ contracts.ContractType) bool {
	for _, finding := range findings {
		if finding.RuleID == string(typ) {
			return true
		}
	}
	return false
}

func trimFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconvFormatFloat(value), "0"), ".")
}

func strconvFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
