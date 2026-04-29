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
// It starts with known initial voltages and "pushes" voltage through
// Regulators until every connected Net has a calculated voltage.
func Propagate(design ir.DesignIR, m meta.Meta, initial map[string]float64) Result {
	result := Result{
		NetVoltages: map[string]NetVoltage{},
	}

	initialVoltages := make(map[string]float64, len(initial)+len(m.Sources))
	for net, voltage := range initial {
		initialVoltages[net] = voltage
	}

	metaSourceNets := map[string]struct{}{}
	sourceVoltages := map[string]float64{}
	for _, s := range m.Sources {
		metaSourceNets[s.Net] = struct{}{}
		if existing, ok := sourceVoltages[s.Net]; ok && existing != s.Voltage {
			result.Conflicts = append(result.Conflicts,
				fmt.Sprintf("Voltage conflict on net %s: %.2f vs %.2f", s.Net, existing, s.Voltage))
			continue
		}
		sourceVoltages[s.Net] = s.Voltage
		initialVoltages[s.Net] = s.Voltage
	}

	nets := make([]string, 0, len(initialVoltages))
	for net := range initialVoltages {
		nets = append(nets, net)
	}
	slices.Sort(nets)
	for _, net := range nets {
		source := "initial"
		if _, ok := metaSourceNets[net]; ok {
			source = "source"
		}
		result.NetVoltages[net] = NetVoltage{
			Net:     net,
			Voltage: initialVoltages[net],
			Source:  source,
		}
	}

	// PHASE 2: Process Regulators
	regs := append([]meta.Regulator(nil), m.Regulators...)

	// Sort regulators by their Reference (U1, U2...) to ensure the
	// simulation results are deterministic and consistent every run.
	slices.SortFunc(regs, func(a, b meta.Regulator) int {
		return cmp.Compare(a.Ref, b.Ref)
	})

	// maxIter defines the maximum "distance" a voltage can travel.
	// We loop enough times to allow voltage to flow through a long chain
	// of regulators (e.g., Battery -> Reg1 -> Reg2 -> Reg3).
	maxIter := len(regs) + len(initialVoltages) + 10

	for i := 0; i < maxIter; i++ {
		// 'changed' tracks if we discovered a NEW voltage in this pass.
		changed := false

		for _, r := range regs {
			// Find which electrical Nets are physically connected to the regulator's pins.
			inNet, ok, amb := topology.NetForRefPin(&design, r.Ref, r.InPin)
			if !ok || amb {
				continue //skip if pin isn't connected or wiring is ambigous
			}
			outNet, ok, amb := topology.NetForRefPin(&design, r.Ref, r.OutPin)
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
				result.Conflicts = append(result.Conflicts,
					fmt.Sprintf("Voltage conflict on net %s: %.2f vs %.2f", outNet, existing.Voltage, r.OutVoltage))
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
