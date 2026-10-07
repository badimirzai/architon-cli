package contracts

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

const draftComment = "# The user fills pin tokens and power_budget currents.\n"

const (
	draftKindConnected = "connected"
	draftKindI2C       = "i2c"
)

// DraftDocument is a contracts YAML draft built from net names.
// It is not a verification result.
type DraftDocument struct {
	YAML string
	IDs  []string
}

type draftContract struct {
	id           string
	busType      string
	busID        string
	kind         string
	nets         []string
	participants []draftParticipant
	selects      []draftChipSelect
	terminated   bool
	sda          string
	scl          string
}

type draftParticipant struct {
	ref  string
	role string
}

type draftChipSelect struct {
	ref string
	net string
}

// DraftFromDesign builds a contracts draft from net names and pin connectivity.
// Part values, pin names, datasheets, and built-in parts are not read.
// The result is not evaluated.
func DraftFromDesign(design *ir.DesignIR) DraftDocument {
	var built []draftContract
	if design != nil {
		built = buildDraftContracts(design)
	}
	return DraftDocument{
		YAML: renderDraft(built),
		IDs:  draftIDs(built),
	}
}

func buildDraftContracts(design *ir.DesignIR) []draftContract {
	out := make([]draftContract, 0, 4)
	if contract, ok := draftSPI(design); ok {
		out = append(out, contract)
	}
	if contract, ok := draftCAN(design); ok {
		out = append(out, contract)
	}
	out = append(out, draftI2C(design)...)
	return out
}

// draftSPI emits one connected contract when SPI_SCK, SPI_MOSI, and SPI_MISO all exist.
// Participants are refs with a pin on all three nets. The ref with the most pins on
// other nets is master and is listed first. Pin tokens are omitted.
func draftSPI(design *ir.DesignIR) (draftContract, bool) {
	canonical := []string{"SPI_SCK", "SPI_MOSI", "SPI_MISO"}
	nets, names, ok := namedNets(design, canonical)
	if !ok {
		return draftContract{}, false
	}
	refs := refsOnAll(nets)
	if len(refs) == 0 {
		return draftContract{}, false
	}
	ordered := orderByMaster(design, refs, canonicalSet(canonical))
	slaves := []string{}
	if len(ordered) > 1 {
		slaves = ordered[1:]
	}
	return draftContract{
		id:           "spi",
		busType:      "spi",
		busID:        "spi",
		kind:         draftKindConnected,
		nets:         names,
		participants: participantRoles(ordered),
		selects:      chipSelects(design, slaves, canonicalSet(canonical)),
	}, true
}

// draftCAN emits one contract when both CANH and CANL exist.
// Every ref with a pin on either net is a participant, including terminators.
// resistance_ohms and count are fixed. Part values are not read.
func draftCAN(design *ir.DesignIR) (draftContract, bool) {
	canonical := []string{"CANH", "CANL"}
	nets, names, ok := namedNets(design, canonical)
	if !ok {
		return draftContract{}, false
	}
	refs := refsOnAny(nets)
	if len(refs) == 0 {
		return draftContract{}, false
	}
	ordered := orderByMaster(design, refs, canonicalSet(canonical))
	return draftContract{
		id:           "can",
		busType:      "can",
		busID:        "can",
		kind:         draftKindConnected,
		nets:         names,
		participants: participantRoles(ordered),
		terminated:   true,
	}, true
}

// draftI2C emits one contract for SDA and SCL, and one for I2C_SDA and I2C_SCL.
// A pair is emitted only when both nets exist.
func draftI2C(design *ir.DesignIR) []draftContract {
	pairs := []struct {
		id  string
		sda string
		scl string
	}{
		{id: "i2c", sda: "SDA", scl: "SCL"},
		{id: "i2c_bus", sda: "I2C_SDA", scl: "I2C_SCL"},
	}
	out := make([]draftContract, 0, len(pairs))
	for _, pair := range pairs {
		sda, okS := netByCanonical(design, pair.sda)
		scl, okC := netByCanonical(design, pair.scl)
		if !okS || !okC {
			continue
		}
		out = append(out, draftContract{
			id:      pair.id,
			busType: "i2c",
			busID:   pair.id,
			kind:    draftKindI2C,
			sda:     sda.Name,
			scl:     scl.Name,
		})
	}
	return out
}

func namedNets(design *ir.DesignIR, canonical []string) ([]ir.Net, []string, bool) {
	nets := make([]ir.Net, 0, len(canonical))
	names := make([]string, 0, len(canonical))
	for _, name := range canonical {
		net, ok := netByCanonical(design, name)
		if !ok {
			return nil, nil, false
		}
		nets = append(nets, net)
		names = append(names, net.Name)
	}
	return nets, names, true
}

// netByCanonical finds nets whose name matches with or without one leading slash.
// Pins on every matching net are combined. The displayed name prefers the form without a slash.
func netByCanonical(design *ir.DesignIR, canonical string) (ir.Net, bool) {
	if design == nil {
		return ir.Net{}, false
	}
	canonical = normalizeNetName(strings.TrimSpace(canonical))
	if canonical == "" {
		return ir.Net{}, false
	}
	var (
		name  string
		pins  []ir.PinRef
		found bool
	)
	for _, net := range design.Nets {
		raw := strings.TrimSpace(net.Name)
		if normalizeNetName(raw) != canonical {
			continue
		}
		if !found || betterDraftNetName(raw, name) {
			name = raw
		}
		found = true
		pins = append(pins, net.Pins...)
	}
	if !found {
		return ir.Net{}, false
	}
	return ir.Net{Name: name, Pins: pins}, true
}

// betterDraftNetName reports whether candidate should replace current as the displayed net name.
func betterDraftNetName(candidate, current string) bool {
	candidate = strings.TrimSpace(candidate)
	current = strings.TrimSpace(current)
	candidateSlash := strings.HasPrefix(candidate, "/")
	currentSlash := strings.HasPrefix(current, "/")
	if candidateSlash != currentSlash {
		return !candidateSlash
	}
	return candidate < current
}

func canonicalSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[normalizeNetName(strings.TrimSpace(name))] = struct{}{}
	}
	return out
}

func refsOnAll(nets []ir.Net) []string {
	if len(nets) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, net := range nets {
		for _, ref := range refsOnNet(net) {
			counts[ref]++
		}
	}
	out := make([]string, 0, len(counts))
	for ref, count := range counts {
		if count == len(nets) {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

func refsOnAny(nets []ir.Net) []string {
	seen := map[string]struct{}{}
	for _, net := range nets {
		for _, ref := range refsOnNet(net) {
			seen[ref] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for ref := range seen {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func refsOnNet(net ir.Net) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(net.Pins))
	for _, pin := range net.Pins {
		ref := strings.TrimSpace(pin.Ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

// orderByMaster lists the ref with the most pins outside bus first.
// Equal pin counts keep the lexicographically smaller ref as master.
// The remaining refs stay in ref order.
func orderByMaster(design *ir.DesignIR, refs []string, bus map[string]struct{}) []string {
	refs = uniqueSorted(refs)
	if len(refs) == 0 {
		return nil
	}
	master := refs[0]
	best := otherPinCount(design, master, bus)
	for _, ref := range refs[1:] {
		count := otherPinCount(design, ref, bus)
		if count > best {
			master = ref
			best = count
		}
	}
	ordered := make([]string, 0, len(refs))
	ordered = append(ordered, master)
	for _, ref := range refs {
		if ref != master {
			ordered = append(ordered, ref)
		}
	}
	return ordered
}

func otherPinCount(design *ir.DesignIR, ref string, bus map[string]struct{}) int {
	if design == nil {
		return 0
	}
	seen := map[string]struct{}{}
	for _, net := range design.Nets {
		norm := normalizeNetName(strings.TrimSpace(net.Name))
		if _, ok := bus[norm]; ok {
			continue
		}
		for _, pin := range net.Pins {
			if strings.TrimSpace(pin.Ref) != ref {
				continue
			}
			id := strings.TrimSpace(pin.Pin)
			if id == "" {
				continue
			}
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func participantRoles(ordered []string) []draftParticipant {
	out := make([]draftParticipant, len(ordered))
	for i, ref := range ordered {
		role := "slave"
		if i == 0 {
			role = "master"
		}
		out[i] = draftParticipant{ref: ref, role: role}
	}
	return out
}

// chipSelects adds an entry when a slave has a pin on exactly one non-bus net
// whose name ends in _CS or CS. Two slaves on that net are both listed.
func chipSelects(design *ir.DesignIR, slaves []string, bus map[string]struct{}) []draftChipSelect {
	slaves = uniqueSorted(slaves)
	out := make([]draftChipSelect, 0, len(slaves))
	for _, ref := range slaves {
		name, ok := singleChipSelect(design, ref, bus)
		if !ok {
			continue
		}
		out = append(out, draftChipSelect{ref: ref, net: name})
	}
	return out
}

func singleChipSelect(design *ir.DesignIR, ref string, bus map[string]struct{}) (string, bool) {
	if design == nil {
		return "", false
	}
	names := map[string]string{}
	for _, net := range design.Nets {
		raw := strings.TrimSpace(net.Name)
		norm := normalizeNetName(raw)
		if norm == "" {
			continue
		}
		if _, ok := bus[norm]; ok {
			continue
		}
		if !isChipSelectNet(raw) {
			continue
		}
		if !refHasPin(net, ref) {
			continue
		}
		if current, ok := names[norm]; !ok || betterDraftNetName(raw, current) {
			names[norm] = raw
		}
	}
	if len(names) != 1 {
		return "", false
	}
	for _, name := range names {
		return name, true
	}
	return "", false
}

// isChipSelectNet reports whether a net name ends in _CS or CS.
// One leading slash is ignored, so IMU_CS and /IMU_CS both match.
// A name ending in _CS also ends in CS.
func isChipSelectNet(name string) bool {
	base := normalizeNetName(strings.TrimSpace(name))
	return base != "" && strings.HasSuffix(base, "CS")
}

func refHasPin(net ir.Net, ref string) bool {
	for _, pin := range net.Pins {
		if strings.TrimSpace(pin.Ref) == ref {
			return true
		}
	}
	return false
}

func uniqueSorted(refs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func draftIDs(list []draftContract) []string {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, len(list))
	for i, contract := range list {
		ids[i] = contract.id
	}
	return ids
}

func renderDraft(list []draftContract) string {
	var b strings.Builder
	b.WriteString(draftComment)
	if len(list) == 0 {
		b.WriteString("contracts: []\n")
		return b.String()
	}
	b.WriteString("contracts:\n")
	for _, contract := range list {
		renderContract(&b, contract)
	}
	return b.String()
}

func renderContract(b *strings.Builder, contract draftContract) {
	fmt.Fprintf(b, "  - id: %s\n", yamlScalar(contract.id))
	b.WriteString("    scope:\n")
	fmt.Fprintf(b, "      bus_type: %s\n", yamlScalar(contract.busType))
	fmt.Fprintf(b, "      bus_id: %s\n", yamlScalar(contract.busID))
	if contract.sda != "" || contract.scl != "" {
		b.WriteString("      nets:\n")
		fmt.Fprintf(b, "        sda: %s\n", yamlScalar(contract.sda))
		fmt.Fprintf(b, "        scl: %s\n", yamlScalar(contract.scl))
	}
	b.WriteString("    require:\n")
	switch contract.kind {
	case draftKindConnected:
		renderConnected(b, contract)
		if contract.terminated {
			renderTerminated(b, contract.nets)
		}
	case draftKindI2C:
		b.WriteString("      common_ground: true\n")
		b.WriteString("      pullup_ohms:\n")
		b.WriteString("        min: 2200\n")
		b.WriteString("        max: 10000\n")
	}
	b.WriteString("    severity: error\n")
}

func renderConnected(b *strings.Builder, contract draftContract) {
	b.WriteString("      connected:\n")
	fmt.Fprintf(b, "        nets: %s\n", yamlFlow(contract.nets))
	if len(contract.selects) > 0 {
		b.WriteString("        chip_selects:\n")
		for _, sel := range contract.selects {
			fmt.Fprintf(b, "          - ref: %s\n", yamlScalar(sel.ref))
			fmt.Fprintf(b, "            net: %s\n", yamlScalar(sel.net))
		}
	}
	b.WriteString("        participants:\n")
	for _, participant := range contract.participants {
		fmt.Fprintf(b, "          - ref: %s\n", yamlScalar(participant.ref))
		fmt.Fprintf(b, "            role: %s\n", yamlScalar(participant.role))
	}
}

func renderTerminated(b *strings.Builder, nets []string) {
	b.WriteString("      terminated:\n")
	fmt.Fprintf(b, "        nets: %s\n", yamlFlow(nets))
	b.WriteString("        resistance_ohms: 120\n")
	b.WriteString("        count: 2\n")
}

func yamlFlow(values []string) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = yamlScalar(value)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func yamlScalar(value string) string {
	if isYAMLPlain(value) {
		return value
	}
	return strconv.Quote(value)
}

func isYAMLPlain(value string) bool {
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "true", "false", "null", "yes", "no", "~", "on", "off":
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
