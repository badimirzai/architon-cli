package rules

import (
	"fmt"

	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
	"github.com/badimirzai/architon-cli/internal/propagate"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/topology"
)

// Overvoltage scans the design to find components connected to a higher
// voltage than their specified maximum rating.
func Overvoltage(
	desin *ir.DesignIR,
	meta *meta.Meta,
	netVoltages map[string]propagate.NetVoltage, // The results from the Propagate function
) []report.RuleResult {
	return OvervoltageWithInferences(desin, meta, netVoltages, nil)
}

func OvervoltageWithInferences(
	desin *ir.DesignIR,
	meta *meta.Meta,
	netVoltages map[string]propagate.NetVoltage,
	inferencesByNet map[string]infer.VoltageInference,
) []report.RuleResult {
	var results []report.RuleResult

	// Step 1: Loop through every component defined in the metadata (e.g., R1, C1, U1)
	for _, comp := range meta.Components {

		// Step 2: Query the design topology to find all physical connections for this component.
		// This returns a list of which pin is on which net (e.g., Pin 1 is on "GND", Pin 2 is on "VCC")
		conns := topology.ConnectionsForRef(desin, comp.Ref)
		for _, c := range conns {
			// Step 3: Check if the net this pin is touching actually has a known voltage.
			netV, ok := netVoltages[c.Net]
			if !ok {
				// skip if the net isn't powered (e.g., an unconnected pin or ground)
				continue
			}

			// Step 4: Compare the actual net voltage against the component's safety limit.
			if netV.Voltage > comp.MaxVoltage {
				result := report.RuleResult{
					ID:       "RULE_OVERVOLTAGE",
					Severity: "error", // critical failure that could damage hardware.
					Message: fmt.Sprintf(
						"%s pin %s on net %s is %.2fV (max %.2fV)",
						comp.Ref, // The component ID (e.g., "LED1")
						c.Pin,    // The specific pin (e.g., "A")
						c.Net,    // The wire name (e.g., "12V_RAIL")
						netV.Voltage,
						comp.MaxVoltage,
					),
				}
				if inference, ok := inferencesByNet[c.Net]; ok {
					result.Inference = &report.InferenceProvenance{
						NetName:         inference.NetName,
						Source:          inference.Source,
						ConfidenceScore: inference.ConfidenceScore,
						ConfidenceLevel: inference.ConfidenceLevel,
					}
				}

				// Step 5: If it's too high, record a rule violation.
				results = append(results, result)
			}
		}
	}

	return results
}
