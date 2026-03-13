package topology

import (
	"cmp"
	"slices"

	"github.com/badimirzai/architon-cli/internal/ir"
)

type PinConn struct {
	Net string
	Pin string
}

func ConnectionsForRef(desgin *ir.DesignIR, ref string) []PinConn {
	var result []PinConn

	for _, net := range desgin.Nets {
		for _, pin := range net.Pins {
			if pin.Ref == ref {
				result = append(result, PinConn{
					Net: net.Name,
					Pin: pin.Pin,
				})
			}
		}
	}
	/* 	sort.Slice(result, func(i, j int) bool {
		// Step A: Check if they are on the same Net
		if result[i].Net == result[j].Net {
			// Step B: If Net is the same, sort by Pin number
			return result[i].Pin < result[j].Pin
		}
		// Step C: If Nets are different, sort alphabetically by Net name
		return result[i].Net < result[j].Net
	}) */

	// Sort the results: Primary sort by Net name, Secondary sort by Pin number.
	slices.SortFunc(result, func(a, b PinConn) int {
		// First, compare the Net names (Primary Sort)
		if n := cmp.Compare(a.Net, b.Net); n != 0 {
			// If the Nets are different, return the comparison result (-1 or 1)
			return n
		}
		// If the Nets are identical (n == 0), break the tie using Pin numbers (Secondary Sort)
		return cmp.Compare(a.Pin, b.Pin)
	})

	return result
}

func NetForRefPin(design *ir.DesignIR, ref string, pin string) (string, bool, bool) {
	var matches []string
	for _, net := range design.Nets {
		for _, pinRef := range net.Pins {

			if pinRef.Ref == ref && pinRef.Pin == pin {
				matches = append(matches, net.Name)
			}
		}
	}
	if len(matches) == 0 {
		return "", false, false
	}
	if len(matches) > 1 {
		return "", false, true
	}
	return matches[0], true, false
}
