package contracts_test

import (
	"math"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestPowerBudgetContractParses(t *testing.T) {
	loaded, err := contracts.ParseYAML([]byte(powerBudgetExample), "contracts.yaml")
	if err != nil {
		t.Fatalf("parse power budget: %v", err)
	}
	if len(loaded) != 1 || len(loaded[0].Requirements) != 1 {
		t.Fatalf("expected one power budget requirement, got %+v", loaded)
	}
	req := loaded[0].Requirements[0]
	if req.Type != contracts.ContractPowerBudget || loaded[0].Scope.BusType != "power" || loaded[0].Scope.BusID != "3V3" || loaded[0].Scope.Rail != "+3V3" {
		t.Fatalf("unexpected power budget scope: %+v scope=%+v", req, loaded[0].Scope)
	}
	if req.PowerSource == nil || req.PowerSource.Ref != "U2" || req.PowerSource.MaxCurrentA != 1 {
		t.Fatalf("unexpected source: %+v", req.PowerSource)
	}
	if len(req.PowerConsumers) != 2 || req.PowerConsumers[0].Ref != "U3" || req.PowerConsumers[0].CurrentA != 0.3 || req.PowerConsumers[1].Ref != "U4" || req.PowerConsumers[1].CurrentA != 0.4 {
		t.Fatalf("unexpected consumers: %+v", req.PowerConsumers)
	}
	if req.MinimumMarginPct == nil || *req.MinimumMarginPct != 20 {
		t.Fatalf("unexpected margin: %+v", req.MinimumMarginPct)
	}
}

func TestPowerBudgetEvaluation(t *testing.T) {
	t.Run("margin below minimum", func(t *testing.T) {
		// 0.5A + 0.25A = 0.75A. Remaining margin is 25%, below 40%.
		design := powerBudgetDesign("U2", "U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 40, `
          - { ref: U3, current_a: 0.5 }
          - { ref: U4, current_a: 0.25 }
`)))
		finding := requireRuleFinding(t, findings, contracts.RulePowerMarginLow)
		if finding.ComponentRef != "U2" || finding.BusType != "power" || finding.BusID != "3V3" {
			t.Fatalf("unexpected margin finding: %+v", finding)
		}
		requireEvidenceMin(t, finding.Expected, 40, "percent")
		requireEvidenceMin(t, finding.Observed, 25, "percent")
		if hasRuleFinding(findings, contracts.RulePowerBudgetExceeded) || hasRuleFinding(findings, contracts.RuleInterfaceNotConnected) {
			t.Fatalf("load is inside the source limit and nets are not checked, got %+v", findings)
		}
	})

	t.Run("declared example currents pass", func(t *testing.T) {
		// 0.3A + 0.4A = 0.7A. Remaining margin is 30%, which meets 20%.
		design := powerBudgetDesign("U2", "U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetExample))
		if len(findings) != 0 {
			t.Fatalf("expected declared example to pass, got %+v", findings)
		}
	})

	t.Run("margin exactly on the limit passes", func(t *testing.T) {
		// 0.8A of 1.0A leaves 20%. Part fields and the GND net must not change that.
		design := powerBudgetDesign("U2", "U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 20, `
          - { ref: U3, current_a: 0.8 }
`)))
		if len(findings) != 0 {
			t.Fatalf("expected exact margin to pass, got %+v", findings)
		}
	})

	t.Run("margin just below the limit fails", func(t *testing.T) {
		design := powerBudgetDesign("U2", "U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 20, `
          - { ref: U3, current_a: 0.801 }
`)))
		finding := requireRuleFinding(t, findings, contracts.RulePowerMarginLow)
		requireEvidenceMin(t, finding.Expected, 20, "percent")
		if finding.Observed == nil || finding.Observed.Min == nil || finding.Observed.Unit != "percent" {
			t.Fatalf("expected observed margin percent, got %+v", finding.Observed)
		}
		if *finding.Observed.Min >= 20 || *finding.Observed.Min <= 19 {
			t.Fatalf("expected margin just below 20, got %v", *finding.Observed.Min)
		}
		if hasRuleFinding(findings, contracts.RulePowerBudgetExceeded) {
			t.Fatalf("0.801A is still within 1A, got %+v", findings)
		}
	})

	t.Run("load above source limit", func(t *testing.T) {
		// 0.5A + 0.75A = 1.25A, above 1.0A.
		design := powerBudgetDesign("U2", "U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 20, `
          - { ref: U3, current_a: 0.5 }
          - { ref: U4, current_a: 0.75 }
`)))
		finding := requireRuleFinding(t, findings, contracts.RulePowerBudgetExceeded)
		if finding.ComponentRef != "U2" {
			t.Fatalf("unexpected exceeded finding: %+v", finding)
		}
		requireEvidenceMax(t, finding.Expected, 1, "A")
		requireEvidenceMax(t, finding.Observed, 1.25, "A")
		if !strings.Contains(finding.Message, "1.25A") || !strings.Contains(finding.Message, "1A") {
			t.Fatalf("message should carry both currents, got %q", finding.Message)
		}
		if hasRuleFinding(findings, contracts.RulePowerMarginLow) {
			t.Fatalf("over-limit load is not also a margin finding, got %+v", findings)
		}
	})

	t.Run("load equal to source limit is low margin", func(t *testing.T) {
		design := powerBudgetDesign("U2", "U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 20, `
          - { ref: U3, current_a: 0.5 }
          - { ref: U4, current_a: 0.5 }
`)))
		finding := requireRuleFinding(t, findings, contracts.RulePowerMarginLow)
		requireEvidenceMin(t, finding.Expected, 20, "percent")
		requireEvidenceMin(t, finding.Observed, 0, "percent")
		if hasRuleFinding(findings, contracts.RulePowerBudgetExceeded) {
			t.Fatalf("load equal to the source limit is not exceeded, got %+v", findings)
		}
	})

	t.Run("missing source ref", func(t *testing.T) {
		design := powerBudgetDesign("U3", "U4")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 20, `
          - { ref: U3, current_a: 0.8 }
          - { ref: U4, current_a: 0.8 }
`)))
		finding := requireRuleFinding(t, findings, contracts.RuleInterfaceComponentMissing)
		if finding.ComponentRef != "U2" || finding.Expected == nil || finding.Expected.Text != "component U2" || finding.Observed == nil || finding.Observed.Text != "missing" {
			t.Fatalf("unexpected missing source finding: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RulePowerBudgetExceeded) || hasRuleFinding(findings, contracts.RulePowerMarginLow) {
			t.Fatalf("missing source should not also compare current, got %+v", findings)
		}
	})

	t.Run("missing consumer ref", func(t *testing.T) {
		design := powerBudgetDesign("U2", "U3")
		findings := contracts.Evaluate(design, userContractIR(t, design, powerBudgetContract(1, 20, `
          - { ref: U3, current_a: 0.9 }
          - { ref: U4, current_a: 0.9 }
`)))
		finding := requireRuleFinding(t, findings, contracts.RuleInterfaceComponentMissing)
		if finding.ComponentRef != "U4" || finding.Observed == nil || finding.Observed.Text != "missing" {
			t.Fatalf("unexpected missing consumer finding: %+v", finding)
		}
		if hasRuleFinding(findings, contracts.RulePowerBudgetExceeded) || hasRuleFinding(findings, contracts.RulePowerMarginLow) {
			t.Fatalf("missing consumer should not also compare current, got %+v", findings)
		}
	})
}

func TestPowerBudgetLeavesCurrentBudgetInPlace(t *testing.T) {
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
      power_budget:
        source:
          ref: REG
          max_current_a: 1.0
        consumers:
          - { ref: U1, current_a: 0.1 }
        minimum_margin_pct: 20
    severity: error
`)
	findings := contracts.Evaluate(design, contractIR)
	finding := requireContractFinding(t, findings, contracts.ContractCurrentBudget)
	if !strings.Contains(finding.Message, "90.0%") {
		t.Fatalf("expected current_budget to keep using part fields, got %+v", finding)
	}
	if hasRuleFinding(findings, contracts.RulePowerBudgetExceeded) || hasRuleFinding(findings, contracts.RulePowerMarginLow) {
		t.Fatalf("power_budget must use the declared 0.1A, got %+v", findings)
	}
}

func TestPowerBudgetSchemaRejectsBadShape(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "current budget still requires i2c bus type",
			body: `
contracts:
  - id: rail_3v3
    scope: {bus_type: power, bus_id: "3V3", rail: "+3V3"}
    require:
      current_budget:
        max_utilization_pct: 80
    severity: error
`,
			wantErr: "scope.bus_type must be i2c",
		},
		{
			name: "unknown power budget key",
			body: `
contracts:
  - id: rail_3v3
    scope: {bus_type: power, bus_id: "3V3"}
    require:
      power_budget:
        source: {ref: U2, max_current_a: 1}
        consumers:
          - {ref: U3, current_a: 0.2}
        minimum_margin_pct: 20
        datasheet: true
    severity: error
`,
			wantErr: `unknown power_budget key "datasheet"`,
		},
		{
			name: "max current must be positive",
			body: `
contracts:
  - id: rail_3v3
    scope: {bus_type: power, bus_id: "3V3"}
    require:
      power_budget:
        source: {ref: U2, max_current_a: 0}
        consumers:
          - {ref: U3, current_a: 0.2}
        minimum_margin_pct: 20
    severity: error
`,
			wantErr: "power_budget.source.max_current_a must be > 0",
		},
		{
			name: "current must be a number",
			body: `
contracts:
  - id: rail_3v3
    scope: {bus_type: power, bus_id: "3V3"}
    require:
      power_budget:
        source: {ref: U2, max_current_a: 1}
        consumers:
          - {ref: U3, current_a: "0.2"}
        minimum_margin_pct: 20
    severity: error
`,
			wantErr: "power_budget.consumers.current_a must be a number",
		},
		{
			name: "margin above 100",
			body: `
contracts:
  - id: rail_3v3
    scope: {bus_type: power, bus_id: "3V3"}
    require:
      power_budget:
        source: {ref: U2, max_current_a: 1}
        consumers:
          - {ref: U3, current_a: 0.2}
        minimum_margin_pct: 120
    severity: error
`,
			wantErr: "power_budget.minimum_margin_pct must be >= 0 and <= 100",
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
}

func TestEnabledRuleIDsIncludePowerBudgetRules(t *testing.T) {
	enabled := map[string]struct{}{}
	for _, id := range contracts.EnabledRuleIDs() {
		enabled[id] = struct{}{}
	}
	for _, id := range []string{
		contracts.RulePowerBudgetExceeded,
		contracts.RulePowerMarginLow,
		contracts.RuleInterfaceComponentMissing,
	} {
		if _, ok := enabled[id]; !ok {
			t.Fatalf("enabled rules missing %s", id)
		}
	}
}

const powerBudgetExample = `
contracts:
  - id: rail_3v3
    scope:
      bus_type: power
      bus_id: "3V3"
      rail: "+3V3"
    require:
      power_budget:
        source:
          ref: U2
          max_current_a: 1.0
        consumers:
          - { ref: U3, current_a: 0.3 }
          - { ref: U4, current_a: 0.4 }
        minimum_margin_pct: 20
    severity: error
`

func powerBudgetContract(maxA float64, margin float64, consumers string) string {
	return `
contracts:
  - id: rail_3v3
    scope:
      bus_type: power
      bus_id: "3V3"
      rail: "+3V3"
    require:
      power_budget:
        source:
          ref: U2
          max_current_a: ` + trimFloat(maxA) + `
        consumers:
` + consumers + `
        minimum_margin_pct: ` + trimFloat(margin) + `
    severity: error
`
}

// powerBudgetDesign puts every ref on GND and stores large current fields.
// power_budget must ignore both. A wrong rail stays a connected contract.
func powerBudgetDesign(refs ...string) *ir.DesignIR {
	parts := make([]ir.Part, 0, len(refs))
	pins := make([]ir.PinRef, 0, len(refs))
	for i, ref := range refs {
		parts = append(parts, ir.Part{
			Ref: ref,
			Fields: map[string]string{
				"architon_current_budget_a": "9",
				"load_current_a":            "9",
				"max_current_a":             "9",
			},
		})
		pins = append(pins, ir.PinRef{Ref: ref, Pin: trimFloat(float64(i + 1))})
	}
	return &ir.DesignIR{
		Parts: parts,
		Nets:  []ir.Net{{Name: "GND", Pins: pins}},
	}
}

func requireEvidenceMax(t *testing.T, ev *contracts.Evidence, want float64, unit string) {
	t.Helper()
	if ev == nil || ev.Max == nil || ev.Min != nil || ev.Text != "" || ev.Unit != unit || math.Abs(*ev.Max-want) > 1e-9 {
		t.Fatalf("expected max %v %s, got %+v", want, unit, ev)
	}
}

func requireEvidenceMin(t *testing.T, ev *contracts.Evidence, want float64, unit string) {
	t.Helper()
	if ev == nil || ev.Min == nil || ev.Max != nil || ev.Text != "" || ev.Unit != unit || math.Abs(*ev.Min-want) > 1e-9 {
		t.Fatalf("expected min %v %s, got %+v", want, unit, ev)
	}
}
