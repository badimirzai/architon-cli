package contracts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

const connectionProposalComment = "# A decided entry is not a scan result, and a needs_choice entry is not accepted.\n"

const (
	proposalStatusDecided     = "decided"
	proposalStatusNeedsChoice = "needs_choice"
	proposalStatusConflict    = "conflict"
)

// ConnectionProposal is a reviewable join list.
// It is built from built-in pin functions and netlist connectivity.
// It is not a scan result, and the rule engine does not evaluate it.
type ConnectionProposal struct {
	YAML        string
	Decided     int
	NeedsChoice int
	Conflict    int
}

type proposalEntry struct {
	id         string
	status     string
	signal     string
	net        string
	ref        string
	mpn        string
	parts      []proposalPart
	candidates []proposalCandidate
}

type proposalPart struct {
	ref      string
	mpn      string
	pin      string
	nets     []proposalNet
	citation Citation
}

type proposalCandidate struct {
	number   string
	pins     []string
	citation Citation
}

type proposalNet struct {
	canonical string
	display   string
}

type proposalSide struct {
	ref      string
	mpn      string
	pin      string
	citation Citation
	nets     []proposalNet
}

type matchedProposalPart struct {
	ref       string
	mpn       string
	functions []PinFunction
}

type signalNetState int

const (
	signalNetNone signalNetState = iota
	signalNetOne
	signalNetConflict
)

// ProposeConnections builds a reviewable join list from built-in pin functions and netlist connectivity.
// Dedicated SDA and SCL pins can decide a join. A gpio_candidate cannot.
// The function does not call the rule engine.
func ProposeConnections(design *ir.DesignIR) ConnectionProposal {
	entries := buildConnectionEntries(design)
	decided := 0
	needsChoice := 0
	conflict := 0
	for _, entry := range entries {
		switch entry.status {
		case proposalStatusDecided:
			decided++
		case proposalStatusNeedsChoice:
			needsChoice++
		case proposalStatusConflict:
			conflict++
		}
	}
	return ConnectionProposal{
		YAML:        renderConnectionProposal(entries),
		Decided:     decided,
		NeedsChoice: needsChoice,
		Conflict:    conflict,
	}
}

func buildConnectionEntries(design *ir.DesignIR) []proposalEntry {
	parts := matchedProposalParts(design)
	if len(parts) == 0 {
		return nil
	}
	entries := make([]proposalEntry, 0)
	for _, signal := range []string{"SDA", "SCL"} {
		entries = append(entries, entriesForSignal(design, parts, signal)...)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].signal != entries[j].signal {
			return signalOrder(entries[i].signal) < signalOrder(entries[j].signal)
		}
		if entries[i].status != entries[j].status {
			return statusOrder(entries[i].status) < statusOrder(entries[j].status)
		}
		return entries[i].id < entries[j].id
	})
	return entries
}

func signalOrder(signal string) int {
	switch strings.ToUpper(strings.TrimSpace(signal)) {
	case "SDA":
		return 0
	case "SCL":
		return 1
	default:
		return 2
	}
}

func statusOrder(status string) int {
	switch status {
	case proposalStatusDecided:
		return 0
	case proposalStatusNeedsChoice:
		return 1
	case proposalStatusConflict:
		return 2
	default:
		return 3
	}
}

func matchedProposalParts(design *ir.DesignIR) []matchedProposalPart {
	if design == nil {
		return nil
	}
	parts := append([]ir.Part(nil), design.Parts...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].Ref < parts[j].Ref })
	seen := map[string]struct{}{}
	out := make([]matchedProposalPart, 0, len(parts))
	for _, part := range parts {
		ref := strings.TrimSpace(part.Ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		// Ambiguous matches are skipped. A guessed MPN must not decide a join.
		match := MatchPart(part, nil)
		if !match.Matched {
			continue
		}
		out = append(out, matchedProposalPart{
			ref:       ref,
			mpn:       match.Contract.MPN,
			functions: match.Contract.PinFunctions,
		})
	}
	return out
}

func entriesForSignal(design *ir.DesignIR, parts []matchedProposalPart, signal string) []proposalEntry {
	sides := make([]proposalSide, 0)
	for _, part := range parts {
		side, ok := dedicatedSide(design, part, signal)
		if !ok {
			continue
		}
		sides = append(sides, side)
	}
	entries := make([]proposalEntry, 0)
	if len(sides) >= 2 {
		entries = append(entries, pairEntries(sides, signal)...)
	}
	if len(sides) == 0 {
		return entries
	}
	net, state := uniqueSignalNet(sides)
	for _, part := range parts {
		if !gpioOnlyForSignal(part.functions, signal) {
			continue
		}
		entry := proposalEntry{
			id:         proposalID(signal, part.ref),
			status:     proposalStatusNeedsChoice,
			signal:     signal,
			ref:        part.ref,
			mpn:        part.mpn,
			candidates: gpioCandidateGroups(part.functions),
		}
		// The net is named only when every dedicated pin agrees on one net.
		// A gpio_candidate that already sits on a net does not choose that net.
		if state == signalNetOne {
			entry.net = net.display
		}
		entries = append(entries, entry)
	}
	return entries
}

func pairEntries(sides []proposalSide, signal string) []proposalEntry {
	net, state := uniqueSignalNet(sides)
	entries := make([]proposalEntry, 0)
	for i := 0; i < len(sides); i++ {
		for j := i + 1; j < len(sides); j++ {
			left := sides[i]
			right := sides[j]
			id := proposalID(signal, left.ref, right.ref)
			switch state {
			case signalNetOne:
				if len(left.nets) == 0 && len(right.nets) == 0 {
					continue
				}
				entries = append(entries, proposalEntry{
					id:     id,
					status: proposalStatusDecided,
					signal: signal,
					net:    net.display,
					parts:  proposalPartsFromSides(left, right),
				})
			case signalNetConflict:
				if !sidesConflict(left, right) {
					continue
				}
				entries = append(entries, proposalEntry{
					id:     id,
					status: proposalStatusConflict,
					signal: signal,
					parts:  proposalPartsFromSides(left, right),
				})
			}
		}
	}
	return entries
}

// uniqueSignalNet is one when every connected dedicated pin shares that net.
// Pins on no net do not create a second choice. Two different nets conflict.
func uniqueSignalNet(sides []proposalSide) (proposalNet, signalNetState) {
	var net proposalNet
	state := signalNetNone
	for _, side := range sides {
		if len(side.nets) == 0 {
			continue
		}
		if len(side.nets) != 1 {
			return proposalNet{}, signalNetConflict
		}
		switch state {
		case signalNetNone:
			net = side.nets[0]
			state = signalNetOne
		case signalNetOne:
			if side.nets[0].canonical != net.canonical {
				return proposalNet{}, signalNetConflict
			}
			if betterDraftNetName(side.nets[0].display, net.display) {
				net.display = side.nets[0].display
			}
		}
	}
	return net, state
}

func sidesConflict(left proposalSide, right proposalSide) bool {
	if len(left.nets) == 0 || len(right.nets) == 0 {
		return false
	}
	return len(left.nets) != 1 || len(right.nets) != 1 || left.nets[0].canonical != right.nets[0].canonical
}

func proposalPartsFromSides(sides ...proposalSide) []proposalPart {
	out := make([]proposalPart, 0, len(sides))
	for _, side := range sides {
		out = append(out, proposalPart{
			ref:      side.ref,
			mpn:      side.mpn,
			pin:      side.pin,
			nets:     append([]proposalNet(nil), side.nets...),
			citation: side.citation,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ref < out[j].ref })
	return out
}

func proposalID(signal string, refs ...string) string {
	ordered := append([]string(nil), refs...)
	sort.Strings(ordered)
	return strings.ToLower(strings.TrimSpace(signal)) + "-" + strings.Join(ordered, "-")
}

func dedicatedSide(design *ir.DesignIR, part matchedProposalPart, signal string) (proposalSide, bool) {
	functions := dedicatedBusFunctions(part.functions, signal)
	if len(functions) == 0 {
		return proposalSide{}, false
	}
	display := displayBusFunction(functions, signal)
	return proposalSide{
		ref:      part.ref,
		mpn:      part.mpn,
		pin:      strings.TrimSpace(display.Name),
		citation: display.Citation,
		nets:     connectedNets(design, part.ref, functions),
	}, true
}

func dedicatedBusFunctions(functions []PinFunction, signal string) []PinFunction {
	out := make([]PinFunction, 0, 2)
	for _, fn := range functions {
		if fn.Kind != PinFunctionBus {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(fn.Signal), signal) {
			continue
		}
		if !fn.Citation.cited() || strings.TrimSpace(fn.Name) == "" {
			continue
		}
		out = append(out, fn)
	}
	return out
}

// displayBusFunction prefers the pin whose name is the signal, such as SDA
// over an alias on the same pad. That choice is the datasheet name, not a
// ranking of gpio_candidate pins.
func displayBusFunction(functions []PinFunction, signal string) PinFunction {
	for _, fn := range functions {
		if strings.EqualFold(strings.TrimSpace(fn.Name), signal) {
			return fn
		}
	}
	return functions[0]
}

// gpioOnlyForSignal is an MCU side for this signal: cited gpio_candidate pins,
// and no dedicated bus pin for the signal. Power and ground pins can still exist.
func gpioOnlyForSignal(functions []PinFunction, signal string) bool {
	if len(dedicatedBusFunctions(functions, signal)) > 0 {
		return false
	}
	return len(gpioCandidateGroups(functions)) > 0
}

// gpioCandidateGroups lists every cited gpio_candidate. Pins that share a
// number stay together so IO21 and GPIO21 are one pin with two names.
// Catalog order is kept. It is not a ranking, and none of the pins is accepted.
func gpioCandidateGroups(functions []PinFunction) []proposalCandidate {
	order := make([]string, 0)
	groups := map[string]*proposalCandidate{}
	for _, fn := range functions {
		if fn.Kind != PinFunctionGPIOCandidate || !fn.Citation.cited() {
			continue
		}
		name := strings.TrimSpace(fn.Name)
		if name == "" {
			continue
		}
		number := strings.TrimSpace(fn.Number)
		key := number
		if key == "" {
			key = "name:" + name
		}
		key += "\x00" + fn.Citation.String()
		group, ok := groups[key]
		if !ok {
			group = &proposalCandidate{number: number, citation: fn.Citation}
			groups[key] = group
			order = append(order, key)
		}
		if !containsString(group.pins, name) {
			group.pins = append(group.pins, name)
		}
	}
	out := make([]proposalCandidate, 0, len(order))
	for _, key := range order {
		out = append(out, *groups[key])
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// connectedNets finds nets whose pin name or pin number matches a dedicated function.
// A KiCad unconnected-* net is not a join target. A leading slash does not make a second net.
func connectedNets(design *ir.DesignIR, ref string, functions []PinFunction) []proposalNet {
	if design == nil {
		return nil
	}
	display := map[string]string{}
	for _, net := range design.Nets {
		raw := strings.TrimSpace(net.Name)
		if raw == "" || isUnconnectedNet(raw) {
			continue
		}
		canon := canonicalProposalNet(raw)
		if canon == "" {
			continue
		}
		if !refHasMatchingPin(net, ref, functions) {
			continue
		}
		if current, ok := display[canon]; !ok || betterDraftNetName(raw, current) {
			display[canon] = raw
		}
	}
	canons := make([]string, 0, len(display))
	for canon := range display {
		canons = append(canons, canon)
	}
	sort.Strings(canons)
	out := make([]proposalNet, 0, len(canons))
	for _, canon := range canons {
		out = append(out, proposalNet{canonical: canon, display: display[canon]})
	}
	return out
}

func refHasMatchingPin(net ir.Net, ref string, functions []PinFunction) bool {
	for _, pin := range net.Pins {
		if strings.TrimSpace(pin.Ref) != ref {
			continue
		}
		for _, fn := range functions {
			if functionMatchesPin(fn, pin) {
				return true
			}
		}
	}
	return false
}

func functionMatchesPin(fn PinFunction, pin ir.PinRef) bool {
	if pinFunctionTokenEqual(fn.Name, pin.Pin) || pinFunctionTokenEqual(fn.Name, pin.Name) {
		return true
	}
	if strings.TrimSpace(fn.Number) == "" {
		return false
	}
	return pinFunctionTokenEqual(fn.Number, pin.Pin) || pinFunctionTokenEqual(fn.Number, pin.Name)
}

func canonicalProposalNet(name string) string {
	name = strings.TrimSpace(name)
	for strings.HasPrefix(name, "/") {
		name = strings.TrimPrefix(name, "/")
	}
	return name
}

func isUnconnectedNet(name string) bool {
	return strings.HasPrefix(strings.ToLower(canonicalProposalNet(name)), "unconnected")
}

func renderConnectionProposal(entries []proposalEntry) string {
	var b strings.Builder
	b.WriteString(connectionProposalComment)
	if len(entries) == 0 {
		b.WriteString("connections: []\n")
		return b.String()
	}
	b.WriteString("connections:\n")
	for _, entry := range entries {
		renderProposalEntry(&b, entry)
	}
	return b.String()
}

func renderProposalEntry(b *strings.Builder, entry proposalEntry) {
	fmt.Fprintf(b, "  - id: %s\n", yamlScalar(entry.id))
	fmt.Fprintf(b, "    status: %s\n", yamlScalar(entry.status))
	fmt.Fprintf(b, "    signal: %s\n", yamlScalar(entry.signal))
	switch entry.status {
	case proposalStatusDecided:
		fmt.Fprintf(b, "    net: %s\n", yamlScalar(entry.net))
		renderProposalParts(b, entry.parts, false)
	case proposalStatusConflict:
		// Conflict records the nets the pins are already on. It does not name a join.
		renderProposalParts(b, entry.parts, true)
	case proposalStatusNeedsChoice:
		if entry.net != "" {
			fmt.Fprintf(b, "    net: %s\n", yamlScalar(entry.net))
		}
		fmt.Fprintf(b, "    ref: %s\n", yamlScalar(entry.ref))
		fmt.Fprintf(b, "    mpn: %s\n", yamlScalar(entry.mpn))
		renderProposalCandidates(b, entry.candidates)
	}
}

func renderProposalParts(b *strings.Builder, parts []proposalPart, includeNets bool) {
	b.WriteString("    parts:\n")
	for _, part := range parts {
		fmt.Fprintf(b, "      - ref: %s\n", yamlScalar(part.ref))
		fmt.Fprintf(b, "        mpn: %s\n", yamlScalar(part.mpn))
		fmt.Fprintf(b, "        pin: %s\n", yamlScalar(part.pin))
		if includeNets {
			renderPartNets(b, part.nets)
		}
		renderProposalCitation(b, "        ", part.citation)
	}
}

func renderPartNets(b *strings.Builder, nets []proposalNet) {
	switch len(nets) {
	case 0:
	case 1:
		fmt.Fprintf(b, "        net: %s\n", yamlScalar(nets[0].display))
	default:
		names := make([]string, len(nets))
		for i, net := range nets {
			names[i] = net.display
		}
		fmt.Fprintf(b, "        nets: %s\n", yamlFlow(names))
	}
}

func renderProposalCandidates(b *strings.Builder, candidates []proposalCandidate) {
	b.WriteString("    candidates:\n")
	for _, candidate := range candidates {
		fmt.Fprintf(b, "      - pins: %s\n", yamlFlow(candidate.pins))
		if candidate.number != "" {
			fmt.Fprintf(b, "        number: %s\n", yamlScalar(candidate.number))
		}
		renderProposalCitation(b, "        ", candidate.citation)
	}
}

func renderProposalCitation(b *strings.Builder, indent string, citation Citation) {
	fmt.Fprintf(b, "%scitation:\n", indent)
	fmt.Fprintf(b, "%s  datasheet: %s\n", indent, yamlScalar(citation.Datasheet))
	fmt.Fprintf(b, "%s  revision: %s\n", indent, yamlScalar(citation.Revision))
	if table := strings.TrimSpace(citation.Table); table != "" {
		fmt.Fprintf(b, "%s  table: %s\n", indent, yamlScalar(table))
	}
	if section := strings.TrimSpace(citation.Section); section != "" {
		fmt.Fprintf(b, "%s  section: %s\n", indent, yamlScalar(section))
	}
}
