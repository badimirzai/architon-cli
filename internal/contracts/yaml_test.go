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
	if !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected strict schema error, got %v", err)
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
	if !strings.Contains(finding.Message, "below minimum") {
		t.Fatalf("expected below-min pullup finding, got %+v", finding)
	}
}

func TestUserYAMLPullupAboveMaxTriggersFinding(t *testing.T) {
	design := i2cDesign(map[string]float64{"R1": 20000, "R2": 20000})
	contractIR := userContractIR(t, design, pullupPolicyYAML(2200, 10000))

	finding := requireContractFinding(t, contracts.Evaluate(design, contractIR), contracts.ContractPullupOhms)
	if !strings.Contains(finding.Message, "above maximum") {
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

func trimFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconvFormatFloat(value), "0"), ".")
}

func strconvFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
