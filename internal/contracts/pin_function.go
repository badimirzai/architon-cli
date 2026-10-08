package contracts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// PinFunctionKind is the datasheet role of one cited pin.
// gpio_candidate means the datasheet allows I2C through pinmux.
// It is not a dedicated SDA or SCL pin.
type PinFunctionKind string

const (
	PinFunctionPower         PinFunctionKind = "power"
	PinFunctionGround        PinFunctionKind = "ground"
	PinFunctionBus           PinFunctionKind = "bus"
	PinFunctionGPIOCandidate PinFunctionKind = "gpio_candidate"
)

const (
	uncheckedUnmatchedPart = "unmatched_part"
	uncheckedNoPinFunction = "no_pin_function"
	uncheckedGPIOCandidate = "gpio_candidate"
)

// Citation names the datasheet location a pin function was copied from.
// A function without a title, revision, and table or section is ignored.
type Citation struct {
	Datasheet string `json:"datasheet"`
	Revision  string `json:"revision"`
	Table     string `json:"table,omitempty"`
	Section   string `json:"section,omitempty"`
}

// String renders a citation for findings and rv parts show.
func (c Citation) String() string {
	parts := []string{
		strings.TrimSpace(c.Datasheet),
		"revision " + strings.TrimSpace(c.Revision),
	}
	if table := strings.TrimSpace(c.Table); table != "" {
		parts = append(parts, "table "+table)
	}
	if section := strings.TrimSpace(c.Section); section != "" {
		parts = append(parts, "section "+section)
	}
	return strings.Join(parts, ", ")
}

// cited reports whether this citation has the required datasheet fields.
func (c Citation) cited() bool {
	if strings.TrimSpace(c.Datasheet) == "" || strings.TrimSpace(c.Revision) == "" {
		return false
	}
	return strings.TrimSpace(c.Table) != "" || strings.TrimSpace(c.Section) != ""
}

// PinFunction is one cited pin on a built-in part.
// Number is optional. A missing number is not invented.
// Signal is set for dedicated functions such as SDA, SCL, VIN, or VOUT.
type PinFunction struct {
	Name     string          `json:"name"`
	Number   string          `json:"number,omitempty"`
	Kind     PinFunctionKind `json:"kind"`
	Signal   string          `json:"signal,omitempty"`
	Citation Citation        `json:"citation"`
}

// ProvedCoverage counts dedicated bus-pin checks that passed.
// RuleIDs lists the pin-function rules that passed at least once.
type ProvedCoverage struct {
	Count   int      `json:"count"`
	RuleIDs []string `json:"rule_ids"`
}

// RefusedFinding is an existing ERROR or WARN finding.
// Unchecked pins are not copied here and do not change the exit code.
type RefusedFinding struct {
	RuleID       string     `json:"rule_id"`
	Severity     string     `json:"severity"`
	ComponentRef string     `json:"component_ref,omitempty"`
	Net          string     `json:"net,omitempty"`
	Pin          string     `json:"pin,omitempty"`
	Message      string     `json:"message,omitempty"`
	Expected     *Evidence  `json:"expected,omitempty"`
	Observed     *Evidence  `json:"observed,omitempty"`
	Citations    []Citation `json:"citations,omitempty"`
}

// UncheckedItem is a part or pin the pin-function rules did not check.
type UncheckedItem struct {
	Ref    string `json:"ref"`
	Pin    string `json:"pin,omitempty"`
	Net    string `json:"net,omitempty"`
	Reason string `json:"reason"`
}

// CheckCoverage separates checks that passed, findings that failed, and pins that were not checked.
type CheckCoverage struct {
	Proved     ProvedCoverage   `json:"proved"`
	Refused    []RefusedFinding `json:"refused"`
	NotChecked []UncheckedItem  `json:"not_checked"`
}

// PinFunctionAnalysis is the pin-function result for one design.
type PinFunctionAnalysis struct {
	Findings   []Finding
	Proved     ProvedCoverage
	NotChecked []UncheckedItem
}

type busPinHit struct {
	fn  PinFunction
	pin ir.PinRef
	net string
}

// AnalyzePinFunctions compares cited dedicated bus pins with net names.
// A gpio_candidate never produces a finding. A missing citation never produces a finding.
func AnalyzePinFunctions(design *ir.DesignIR, contractIR *ContractIR) PinFunctionAnalysis {
	analysis := PinFunctionAnalysis{
		Proved:     ProvedCoverage{RuleIDs: []string{}},
		NotChecked: []UncheckedItem{},
	}
	if design == nil || contractIR == nil {
		return analysis
	}

	matched := uniquePartMatches(contractIR.PartMatches)
	parts := append([]ir.Part(nil), design.Parts...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].Ref < parts[j].Ref })
	seenUnmatched := map[string]struct{}{}
	for _, part := range parts {
		ref := strings.TrimSpace(part.Ref)
		if ref == "" {
			continue
		}
		if _, ok := matched[ref]; ok {
			continue
		}
		if _, ok := seenUnmatched[ref]; ok {
			continue
		}
		seenUnmatched[ref] = struct{}{}
		analysis.NotChecked = append(analysis.NotChecked, UncheckedItem{
			Ref:    ref,
			Reason: uncheckedUnmatchedPart,
		})
	}

	type connection struct {
		pin ir.PinRef
		net string
	}
	byRef := map[string][]connection{}
	nets := append([]ir.Net(nil), design.Nets...)
	sort.Slice(nets, func(i, j int) bool { return nets[i].Name < nets[j].Name })
	for _, net := range nets {
		pins := append([]ir.PinRef(nil), net.Pins...)
		sort.Slice(pins, func(i, j int) bool {
			if pins[i].Ref != pins[j].Ref {
				return pins[i].Ref < pins[j].Ref
			}
			if pins[i].Pin != pins[j].Pin {
				return pins[i].Pin < pins[j].Pin
			}
			return pins[i].Name < pins[j].Name
		})
		for _, pin := range pins {
			ref := strings.TrimSpace(pin.Ref)
			if ref == "" {
				continue
			}
			if _, ok := matched[ref]; !ok {
				continue
			}
			byRef[ref] = append(byRef[ref], connection{pin: pin, net: net.Name})
		}
	}

	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	passed := map[string]int{}
	for _, ref := range refs {
		component := contractIR.Components[ref]
		var sda []busPinHit
		var scl []busPinHit
		for _, conn := range byRef[ref] {
			fn, ok := matchPinFunction(component.PinFunctions, conn.pin)
			if !ok {
				analysis.NotChecked = append(analysis.NotChecked, UncheckedItem{
					Ref:    ref,
					Pin:    conn.pin.Pin,
					Net:    conn.net,
					Reason: uncheckedNoPinFunction,
				})
				continue
			}
			if fn.Kind == PinFunctionGPIOCandidate && isNamedI2CNet(conn.net) {
				analysis.NotChecked = append(analysis.NotChecked, UncheckedItem{
					Ref:    ref,
					Pin:    conn.pin.Pin,
					Net:    conn.net,
					Reason: uncheckedGPIOCandidate,
				})
				continue
			}
			if fn.Kind != PinFunctionBus {
				continue
			}
			hit := busPinHit{fn: fn, pin: conn.pin, net: conn.net}
			switch strings.ToUpper(strings.TrimSpace(fn.Signal)) {
			case "SDA":
				sda = append(sda, hit)
				if dedicatedBusNetMismatch(fn.Signal, conn.net) {
					analysis.Findings = append(analysis.Findings, pinFunctionMismatchFinding(ref, component.MPN, hit))
				} else {
					passed[RulePinFunctionMismatch]++
				}
			case "SCL":
				scl = append(scl, hit)
				if dedicatedBusNetMismatch(fn.Signal, conn.net) {
					analysis.Findings = append(analysis.Findings, pinFunctionMismatchFinding(ref, component.MPN, hit))
				} else {
					passed[RulePinFunctionMismatch]++
				}
			}
		}
		if len(sda) == 0 || len(scl) == 0 {
			continue
		}
		shared := sharedBusNets(sda, scl)
		if len(shared) == 0 {
			passed[RulePinBusShort]++
			continue
		}
		for _, netName := range shared {
			analysis.Findings = append(analysis.Findings, pinBusShortFinding(ref, component.MPN, netName, sda, scl))
		}
	}

	analysis.Proved.Count = passed[RulePinFunctionMismatch] + passed[RulePinBusShort]
	for _, ruleID := range []string{RulePinBusShort, RulePinFunctionMismatch} {
		if passed[ruleID] > 0 {
			analysis.Proved.RuleIDs = append(analysis.Proved.RuleIDs, ruleID)
		}
	}
	sort.Strings(analysis.Proved.RuleIDs)
	sort.Slice(analysis.NotChecked, func(i, j int) bool {
		if analysis.NotChecked[i].Ref != analysis.NotChecked[j].Ref {
			return analysis.NotChecked[i].Ref < analysis.NotChecked[j].Ref
		}
		if analysis.NotChecked[i].Pin != analysis.NotChecked[j].Pin {
			return analysis.NotChecked[i].Pin < analysis.NotChecked[j].Pin
		}
		if analysis.NotChecked[i].Net != analysis.NotChecked[j].Net {
			return analysis.NotChecked[i].Net < analysis.NotChecked[j].Net
		}
		return analysis.NotChecked[i].Reason < analysis.NotChecked[j].Reason
	})
	return analysis
}

func pinFunctionMismatchFinding(ref string, mpn string, hit busPinHit) Finding {
	signal := strings.ToUpper(strings.TrimSpace(hit.fn.Signal))
	citation := hit.fn.Citation
	return Finding{
		RuleID:       RulePinFunctionMismatch,
		Severity:     "ERROR",
		ComponentRef: ref,
		Net:          hit.net,
		Pin:          hit.pin.Pin,
		Expected:     evidenceText(signal),
		Observed:     evidenceText(hit.net),
		Source:       builtInContractSourceName,
		ContractID:   mpn,
		Citations:    []Citation{citation},
		Provenance: Provenance{
			Source:   builtInContractSourceName,
			SourceID: mpn,
			Detail:   citation.String(),
		},
		Fix:            fmt.Sprintf("Move the dedicated %s pin onto a net named %s or I2C_%s.", signal, signal, signal),
		WhyThisMatters: "This pin's datasheet function is fixed. A swapped SDA or SCL net does not connect that signal.",
		Message: fmt.Sprintf(
			"%s pin %s is dedicated %s but is on net %s (%s)",
			ref,
			hit.pin.Pin,
			signal,
			hit.net,
			citation.String(),
		),
	}
}

func pinBusShortFinding(ref string, mpn string, netName string, sda []busPinHit, scl []busPinHit) Finding {
	sdaHit := firstHitOnNet(sda, netName)
	sclHit := firstHitOnNet(scl, netName)
	citations := uniqueCitations([]Citation{sdaHit.fn.Citation, sclHit.fn.Citation})
	return Finding{
		RuleID:       RulePinBusShort,
		Severity:     "ERROR",
		ComponentRef: ref,
		Net:          netName,
		Pin:          sdaHit.pin.Pin,
		Expected:     evidenceText("SDA and SCL on different nets"),
		Observed:     evidenceText(netName),
		Source:       builtInContractSourceName,
		ContractID:   mpn,
		Citations:    citations,
		Provenance: Provenance{
			Source:   builtInContractSourceName,
			SourceID: mpn,
			Detail:   joinCitationText(citations),
		},
		Fix:            "Connect the dedicated SDA and SCL pins to different nets.",
		WhyThisMatters: "Dedicated SDA and SCL are two signals. One net cannot carry both.",
		Message: fmt.Sprintf(
			"%s dedicated SDA pin %s and SCL pin %s are both on net %s (%s)",
			ref,
			sdaHit.pin.Pin,
			sclHit.pin.Pin,
			netName,
			joinCitationText(citations),
		),
	}
}

func firstHitOnNet(hits []busPinHit, netName string) busPinHit {
	for _, hit := range hits {
		if hit.net == netName {
			return hit
		}
	}
	if len(hits) == 0 {
		return busPinHit{}
	}
	return hits[0]
}

func sharedBusNets(sda []busPinHit, scl []busPinHit) []string {
	seen := map[string]struct{}{}
	for _, left := range sda {
		for _, right := range scl {
			if left.net == right.net {
				seen[left.net] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for netName := range seen {
		out = append(out, netName)
	}
	sort.Strings(out)
	return out
}

// matchPinFunction matches a netlist pin name or pin number to a cited function.
// Name matches win over number matches. An uncited function does not match.
func matchPinFunction(functions []PinFunction, pin ir.PinRef) (PinFunction, bool) {
	for _, fn := range functions {
		if !fn.Citation.cited() || strings.TrimSpace(fn.Name) == "" {
			continue
		}
		if pinFunctionTokenEqual(fn.Name, pin.Pin) || pinFunctionTokenEqual(fn.Name, pin.Name) {
			return fn, true
		}
	}
	for _, fn := range functions {
		if !fn.Citation.cited() || strings.TrimSpace(fn.Number) == "" {
			continue
		}
		if pinFunctionTokenEqual(fn.Number, pin.Pin) || pinFunctionTokenEqual(fn.Number, pin.Name) {
			return fn, true
		}
	}
	return PinFunction{}, false
}

func pinFunctionTokenEqual(left string, right string) bool {
	left = normalizePin(left)
	right = normalizePin(right)
	return left != "" && left == right
}

// dedicatedBusNetMismatch reports the swapped-name case for a dedicated bus pin.
// SDA on SCL or I2C_SCL, and SCL on SDA or I2C_SDA, are mismatches.
func dedicatedBusNetMismatch(signal string, net string) bool {
	name := canonicalBusNet(net)
	switch strings.ToUpper(strings.TrimSpace(signal)) {
	case "SDA":
		return name == "SCL" || name == "I2C_SCL"
	case "SCL":
		return name == "SDA" || name == "I2C_SDA"
	default:
		return false
	}
}

func isNamedI2CNet(net string) bool {
	switch canonicalBusNet(net) {
	case "SDA", "SCL", "I2C_SDA", "I2C_SCL":
		return true
	default:
		return false
	}
}

func canonicalBusNet(net string) string {
	net = strings.TrimSpace(net)
	for strings.HasPrefix(net, "/") {
		net = strings.TrimPrefix(net, "/")
	}
	return strings.ToUpper(net)
}

func uniqueCitations(in []Citation) []Citation {
	out := make([]Citation, 0, len(in))
	seen := map[string]struct{}{}
	for _, citation := range in {
		if !citation.cited() {
			continue
		}
		key := citation.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, citation)
	}
	return out
}

func joinCitationText(citations []Citation) string {
	parts := make([]string, 0, len(citations))
	for _, citation := range citations {
		parts = append(parts, citation.String())
	}
	return strings.Join(parts, "; ")
}

func clonePinFunctions(in []PinFunction) []PinFunction {
	if len(in) == 0 {
		return nil
	}
	out := make([]PinFunction, len(in))
	copy(out, in)
	return out
}
