package propagate

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
	"github.com/badimirzai/architon-cli/internal/topology"
)

// NetVoltage tracks which component (Source or Regulator)
// provided a specific voltage to an electrical Net.
type NetVoltage struct {
	Net     string
	Voltage float64
	Source  string
}

// Result holds the final state of the circuit's power distribution
// and any illegal wiring found during analysis.
type Result struct {
	NetVoltages map[string]NetVoltage
	Conflicts   []string
}

// Propagate simulates the flow of electricity through the design.
// It starts with known Sources (batteries) and "pushes" voltage through
// Regulators until every connected Net has a calculated voltage.
func Propagate(desgin *ir.DesignIR, m *meta.Meta) Result {

	// Initialize the map to track voltages.
	// result.Conflicts starts as a nil slice (idiomatic Go).
	result := Result{
		NetVoltages: map[string]NetVoltage{},
	}

	// PHASE 1: Process Primary Power Sources
	// We start here because these are the "origins" of all voltage in the system.
	for _, s := range m.Sources {
		existing, ok := result.NetVoltages[s.Net]

		// If this Net already has a voltage from another source, check for a mismatch.
		if ok && existing.Voltage != s.Voltage {
			result.Conflicts = append(result.Conflicts,
				fmt.Sprintf("Voltage conflict on net %s: %.2f vs %.2f", s.Net, existing.Voltage, s.Voltage))
			continue
		}
		// Mark this net as "powered" by a source.
		result.NetVoltages[s.Net] = NetVoltage{
			Net:     s.Net,
			Voltage: s.Voltage,
			Source:  "source",
		}
	}

	// PHASE 2: Process Regulators
	regs := m.Regulators

	// Sort regulators by their Reference (U1, U2...) to ensure the
	// simulation results are deterministic and consistent every run.
	slices.SortFunc(regs, func(a, b meta.Regulator) int {
		return cmp.Compare(a.Ref, b.Ref)
	})

	// maxIter defines the maximum "distance" a voltage can travel.
	// We loop enough times to allow voltage to flow through a long chain
	// of regulators (e.g., Battery -> Reg1 -> Reg2 -> Reg3).
	maxIter := len(regs) + len(m.Sources) + 10

	for i := 0; i < maxIter; i++ {
		// 'changed' tracks if we discovered a NEW voltage in this pass.
		changed := false

		for _, r := range regs {
			// Find which electrical Nets are physically connected to the regulator's pins.
			inNet, ok, amb := topology.NetForRefPin(desgin, r.Ref, r.InPin)
			if !ok || amb {
				continue //skip if pin isn't connected or wiring is ambigous
			}
			outNet, ok, amb := topology.NetForRefPin(desgin, r.Ref, r.OutPin)
			if !ok || amb {
				continue
			}

			// check if input side of regulator actually has power yet
			_, known := result.NetVoltages[inNet]

			if !known {
				continue // can't regulate power if the input is dead/unkown
			}

			//check if something else is trying to power the output net
			existing, exists := result.NetVoltages[outNet]

			//if the output net already has a different voltage, we have a short circuit/conflic
			if exists && existing.Voltage != r.OutVoltage {
				result.Conflicts = append(result.Conflicts, fmt.Sprintf("Voltage conflict on net %s,", outNet))
				continue
			}

			// If the output net is currently unpoweredm "propagate" the regulator's voltage to it.
			if !exists {
				result.NetVoltages[outNet] = NetVoltage{
					Net:     outNet,
					Voltage: r.OutVoltage,
					Source:  "regulator: " + r.Ref,
				}
				// Since we powered a new net, other regulators might now have
				// what they need. We must run another pass of the outer loop.
				changed = true
			}

		}
		// OPTIMIZATION: If no new voltages were found in this entire pass,
		// the circuit has reached a steady state and we can stop early.
		if !changed {
			break
		}
	}

	// Sort conflicts alphabetically so the error report is easy to read.
	slices.Sort(result.Conflicts)
	return result
}
