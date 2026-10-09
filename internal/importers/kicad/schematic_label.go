package kicad

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// NetLabelPin is an existing schematic pin, named by reference and pin name or pin number.
type NetLabelPin struct {
	Ref string
	Pin string
}

// NetLabelEntry is one accepted join. Each named pin already exists on a schematic.
// A blank pin is ignored, so a candidate choice can name one pin.
type NetLabelEntry struct {
	ID   string
	Net  string
	Pins [2]NetLabelPin
}

// AddNetLabels places one KiCad 9 local label at each unconnected pin.
// The label form matches a schematic saved as (version 20250114):
//
//	(label "I2C_SDA"
//	  (at x y 0)
//	  (effects (font (size 1.27 1.27)) (justify left bottom))
//	  (uuid "..."))
//
// A pin already on the proposal net is left in place. A missing pin, a pin on
// any other net, or a no-connect exits with an error and no rewritten bytes.
// Symbols are not moved. Wires, no-connects, and new symbols are not added.
func AddNetLabels(files map[string][]byte, entries []NetLabelEntry) (map[string][]byte, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	sheets := make([]*parsedSheet, 0, len(paths))
	for _, path := range paths {
		sheet, err := parseSchematic(path, files[path])
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, sheet)
	}

	planned := map[*parsedSheet][]string{}
	for _, entry := range entries {
		if err := planNetLabelEntry(sheets, entry, planned); err != nil {
			return nil, err
		}
	}

	changed := map[string][]byte{}
	for _, sheet := range sheets {
		blocks := planned[sheet]
		if len(blocks) == 0 {
			continue
		}
		next, err := sheet.insertLabels(blocks)
		if err != nil {
			return nil, fmt.Errorf("update schematic %s: %w", sheet.path, err)
		}
		if next != string(files[sheet.path]) {
			changed[sheet.path] = []byte(next)
		}
	}
	return changed, nil
}

func planNetLabelEntry(sheets []*parsedSheet, entry NetLabelEntry, planned map[*parsedSheet][]string) error {
	net := strings.TrimSpace(entry.Net)
	if net == "" || canonicalSchematicNet(net) == "" {
		return fmt.Errorf("accepted entry %s has no net", strings.TrimSpace(entry.ID))
	}
	if strings.ContainsAny(net, "\r\n") {
		return fmt.Errorf("accepted entry %s has no net", strings.TrimSpace(entry.ID))
	}
	seen := map[*placedPin]struct{}{}
	named := 0
	for _, end := range entry.Pins {
		if strings.TrimSpace(end.Ref) == "" && strings.TrimSpace(end.Pin) == "" {
			continue
		}
		named++
		pin, err := findPlacedPin(sheets, end.Ref, end.Pin)
		if err != nil {
			return err
		}
		if _, ok := seen[pin]; ok {
			return fmt.Errorf("accepted entry %s names one pin twice", strings.TrimSpace(entry.ID))
		}
		seen[pin] = struct{}{}
		switch state, detail := pinNetState(pin, net); state {
		case netOnProposal:
		case netUnconnected:
			block, err := formatNetLabel(pin.sheet.indent, pin.sheet.newline, net, pin.at)
			if err != nil {
				return err
			}
			pin.sheet.labels = append(pin.sheet.labels, netAt{name: net, at: pin.at})
			planned[pin.sheet] = append(planned[pin.sheet], block)
		case netOther:
			if detail == "" {
				return fmt.Errorf("pin %s %s is already on a net", pin.ref, pin.token)
			}
			return fmt.Errorf("pin %s %s is already on net %s", pin.ref, pin.token, detail)
		case netNoConnect:
			return fmt.Errorf("pin %s %s has a no-connect", pin.ref, pin.token)
		default:
			return fmt.Errorf("pin %s %s not found", end.Ref, end.Pin)
		}
	}
	if named == 0 {
		return fmt.Errorf("accepted entry %s needs a pin", strings.TrimSpace(entry.ID))
	}
	return nil
}

type netState int

const (
	netUnconnected netState = iota
	netOnProposal
	netOther
	netNoConnect
)

func pinNetState(pin *placedPin, proposalNet string) (netState, string) {
	root := pin.sheet.connected.find(pin.at)
	names := pin.sheet.netNames(root)
	want := canonicalSchematicNet(proposalNet)
	var others []string
	seen := map[string]struct{}{}
	matched := false
	for _, name := range names {
		canon := canonicalSchematicNet(name)
		if canon == "" {
			continue
		}
		if canon == want {
			matched = true
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		others = append(others, name)
	}
	sort.Strings(others)
	if len(others) > 0 {
		return netOther, strings.Join(others, ", ")
	}
	if matched {
		return netOnProposal, ""
	}
	if pin.sheet.wired(root) || pin.sheet.otherPins(root, pin.index) > 0 {
		return netOther, ""
	}
	if pin.sheet.noConnectAt(root) {
		return netNoConnect, ""
	}
	return netUnconnected, ""
}

type point struct {
	x int64
	y int64
}

type netAt struct {
	name string
	at   point
}

type placedPin struct {
	sheet  *parsedSheet
	refs   []string
	ref    string
	token  string
	name   string
	number string
	at     point
	index  int
}

type parsedSheet struct {
	path       string
	src        string
	indent     string
	newline    string
	pins       []placedPin
	wires      [][2]point
	labels     []netAt
	noConnects []point
	connected  *disjoint
}

func parseSchematic(path string, data []byte) (*parsedSheet, error) {
	src := string(data)
	parser := newSExprParser(src)
	roots, err := parser.parseAll()
	if err != nil {
		return nil, fmt.Errorf("parse schematic %s: %w", path, err)
	}
	if len(roots) != 1 || roots[0].head() != "kicad_sch" {
		return nil, fmt.Errorf("parse schematic %s: expected one kicad_sch", path)
	}
	root := roots[0]
	sheet := &parsedSheet{
		path:    path,
		src:     src,
		indent:  detectIndent(src),
		newline: detectNewline(src),
	}
	libs := parseLibPins(childByHead(root, "lib_symbols"))
	for _, child := range root.children[1:] {
		switch child.head() {
		case "symbol":
			if _, ok := child.fieldValue("lib_id"); !ok {
				continue
			}
			pins, power, err := parseSymbolInstance(child, libs)
			if err != nil {
				return nil, fmt.Errorf("parse schematic %s: %w", path, err)
			}
			for _, pin := range pins {
				pin.sheet = sheet
				pin.index = len(sheet.pins)
				sheet.pins = append(sheet.pins, pin)
			}
			sheet.labels = append(sheet.labels, power...)
		case "wire", "bus":
			segments, err := parseSegments(child)
			if err != nil {
				return nil, fmt.Errorf("parse schematic %s: %w", path, err)
			}
			sheet.wires = append(sheet.wires, segments...)
		case "label", "global_label", "hierarchical_label":
			label, err := parseNetLabel(child)
			if err != nil {
				return nil, fmt.Errorf("parse schematic %s: %w", path, err)
			}
			sheet.labels = append(sheet.labels, label)
		case "no_connect":
			at, err := parsePointAt(child)
			if err != nil {
				return nil, fmt.Errorf("parse schematic %s: %w", path, err)
			}
			sheet.noConnects = append(sheet.noConnects, at)
		}
	}
	sheet.connected = sheet.buildConnected()
	return sheet, nil
}

func (sheet *parsedSheet) buildConnected() *disjoint {
	sets := newDisjoint()
	pts := make([]point, 0, len(sheet.pins)+len(sheet.labels)+len(sheet.wires)*2)
	add := func(p point) {
		sets.add(p)
		pts = append(pts, p)
	}
	for _, pin := range sheet.pins {
		add(pin.at)
	}
	for _, label := range sheet.labels {
		add(label.at)
	}
	for _, nc := range sheet.noConnects {
		add(nc)
	}
	for _, wire := range sheet.wires {
		add(wire[0])
		add(wire[1])
	}
	for _, wire := range sheet.wires {
		var on []point
		seen := map[point]bool{}
		for _, p := range pts {
			if seen[p] || !onSegment(wire[0], wire[1], p) {
				continue
			}
			seen[p] = true
			on = append(on, p)
		}
		for i := 1; i < len(on); i++ {
			sets.union(on[0], on[i])
		}
	}
	return sets
}

func (sheet *parsedSheet) netNames(root point) []string {
	var names []string
	seen := map[string]bool{}
	for _, label := range sheet.labels {
		if sheet.connected.find(label.at) != root || seen[label.name] {
			continue
		}
		seen[label.name] = true
		names = append(names, label.name)
	}
	return names
}

func (sheet *parsedSheet) wired(root point) bool {
	for _, wire := range sheet.wires {
		if sheet.connected.find(wire[0]) == root || sheet.connected.find(wire[1]) == root {
			return true
		}
	}
	return false
}

func (sheet *parsedSheet) otherPins(root point, self int) int {
	count := 0
	for i := range sheet.pins {
		if i == self {
			continue
		}
		if sheet.connected.find(sheet.pins[i].at) == root {
			count++
		}
	}
	return count
}

func (sheet *parsedSheet) noConnectAt(root point) bool {
	for _, nc := range sheet.noConnects {
		if sheet.connected.find(nc) == root {
			return true
		}
	}
	return false
}

func findPlacedPin(sheets []*parsedSheet, ref string, token string) (*placedPin, error) {
	ref = strings.TrimSpace(ref)
	token = strings.TrimSpace(token)
	if ref == "" || normalizePinToken(token) == "" {
		return nil, fmt.Errorf("pin %s %s not found", ref, token)
	}
	// A pin name wins over a pin number, matching the netlist pin check.
	found, err := onePlacedPin(sheets, ref, token, true)
	if err != nil || found != nil {
		return found, err
	}
	return onePlacedPin(sheets, ref, token, false)
}

func onePlacedPin(sheets []*parsedSheet, ref string, token string, byName bool) (*placedPin, error) {
	var found *placedPin
	for _, sheet := range sheets {
		for i := range sheet.pins {
			pin := &sheet.pins[i]
			if !pin.hasRef(ref) {
				continue
			}
			matched := pinTokenEqual(pin.number, token)
			if byName {
				matched = pinNameMatches(pin.name, token)
			}
			if !matched {
				continue
			}
			if found != nil {
				return nil, fmt.Errorf("pin %s %s matches more than one pin", ref, token)
			}
			found = pin
		}
	}
	if found == nil {
		if byName {
			return nil, nil
		}
		return nil, fmt.Errorf("pin %s %s not found", ref, token)
	}
	found.ref = ref
	found.token = token
	return found, nil
}

func (pin *placedPin) hasRef(ref string) bool {
	for _, candidate := range pin.refs {
		if candidate == ref {
			return true
		}
	}
	return false
}

type libPin struct {
	name   string
	number string
	unit   int
	style  int
	x      int64
	y      int64
}

func parseLibPins(section *sExpr) map[string][]libPin {
	libs := map[string][]libPin{}
	if section == nil {
		return libs
	}
	for _, child := range section.children[1:] {
		if child.head() != "symbol" {
			continue
		}
		name, ok := atomAt(child, 1)
		if !ok || name == "" {
			continue
		}
		libs[name] = libraryPins(child)
	}
	return libs
}

func libraryPins(lib *sExpr) []libPin {
	pins := directPins(lib, 1, 1)
	for _, child := range lib.children[1:] {
		if child.head() != "symbol" {
			continue
		}
		name, ok := atomAt(child, 1)
		if !ok {
			continue
		}
		unit, style, parsed := splitUnitStyle(name)
		if !parsed {
			unit, style = 1, 1
		}
		pins = append(pins, directPins(child, unit, style)...)
	}
	return pins
}

func directPins(expr *sExpr, unit int, style int) []libPin {
	var pins []libPin
	for _, child := range expr.children[1:] {
		if child.head() != "pin" {
			continue
		}
		name, _ := textField(child, "name")
		number, _ := textField(child, "number")
		x, y, _, err := parseAt(child)
		if err != nil {
			continue
		}
		pins = append(pins, libPin{name: name, number: number, unit: unit, style: style, x: x, y: y})
	}
	return pins
}

func parseSymbolInstance(expr *sExpr, libs map[string][]libPin) ([]placedPin, []netAt, error) {
	libID, ok := expr.fieldValue("lib_id")
	if !ok || strings.TrimSpace(libID) == "" {
		return nil, nil, fmt.Errorf("symbol is missing lib_id")
	}
	origin, rot, err := parsePlacement(expr)
	if err != nil {
		return nil, nil, err
	}
	unit := 1
	if n, ok := intField(expr, "unit"); ok {
		unit = n
	}
	style := 1
	if n, ok := intField(expr, "body_style"); ok {
		style = n
	} else if n, ok := intField(expr, "convert"); ok {
		style = n
	}
	mirrorX, mirrorY := mirrorAxes(expr)
	refs := symbolRefs(expr)
	var pins []placedPin
	for _, lib := range pinsForUnit(libs[libID], unit, style) {
		at, err := worldPin(origin, rot, mirrorX, mirrorY, lib.x, lib.y)
		if err != nil {
			ref := ""
			if len(refs) > 0 {
				ref = refs[0]
			}
			return nil, nil, fmt.Errorf("symbol %s rotation is not supported", ref)
		}
		pins = append(pins, placedPin{
			refs:   refs,
			name:   lib.name,
			number: lib.number,
			at:     at,
		})
	}
	var power []netAt
	if strings.HasPrefix(strings.ToLower(libID), "power:") {
		if value, ok := propertyValue(expr, "Value"); ok && strings.TrimSpace(value) != "" {
			for _, pin := range pins {
				power = append(power, netAt{name: strings.TrimSpace(value), at: pin.at})
			}
		}
	}
	return pins, power, nil
}

func pinsForUnit(pins []libPin, unit int, style int) []libPin {
	out := make([]libPin, 0, len(pins))
	for _, pin := range pins {
		if pin.style != style {
			continue
		}
		if pin.unit != unit && pin.unit != 0 {
			continue
		}
		out = append(out, pin)
	}
	return out
}

func symbolRefs(expr *sExpr) []string {
	var refs []string
	seen := map[string]bool{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	if ref, ok := propertyValue(expr, "Reference"); ok {
		add(ref)
	}
	for _, ref := range nestedAtoms(expr, "reference") {
		add(ref)
	}
	return refs
}

func nestedAtoms(expr *sExpr, head string) []string {
	if expr == nil || expr.kind != sExprList {
		return nil
	}
	var out []string
	if expr.head() == head {
		if value, ok := atomAt(expr, 1); ok {
			out = append(out, value)
		}
	}
	for _, child := range expr.children[1:] {
		out = append(out, nestedAtoms(child, head)...)
	}
	return out
}

func parsePlacement(expr *sExpr) (point, int, error) {
	x, y, rot, err := parseAt(expr)
	if err != nil {
		return point{}, 0, fmt.Errorf("symbol is missing a position")
	}
	return point{x: x, y: y}, rot, nil
}

// worldPin converts a library pin into the schematic point KiCad connects.
// Library Y is up. The .kicad_sch file stores Y down, so an unrotated symbol
// places the pin at (origin.x + pinX, origin.y - pinY). Rotation is counterclockwise
// on the page, in 90 degree steps. This matches version 20250114 schematics:
// a resistor pin at library (0, 3.81) on a symbol at (120.65, 91.44) lands on (120.65, 87.63).
func worldPin(origin point, rot int, mirrorX bool, mirrorY bool, pinX int64, pinY int64) (point, error) {
	switch rot {
	case 0, 90, 180, 270:
	default:
		return point{}, fmt.Errorf("rotation %d", rot)
	}
	if mirrorX {
		pinY = -pinY
	}
	if mirrorY {
		pinX = -pinX
	}
	x, y := pinX, -pinY
	switch rot {
	case 90:
		x, y = y, -x
	case 180:
		x, y = -x, -y
	case 270:
		x, y = -y, x
	}
	return point{x: origin.x + x, y: origin.y + y}, nil
}

func parseNetLabel(expr *sExpr) (netAt, error) {
	name, ok := atomAt(expr, 1)
	if !ok || strings.TrimSpace(name) == "" {
		return netAt{}, fmt.Errorf("label is missing a name")
	}
	at, err := parsePointAt(expr)
	if err != nil {
		return netAt{}, fmt.Errorf("label %s is missing a position", name)
	}
	return netAt{name: name, at: at}, nil
}

func parseSegments(expr *sExpr) ([][2]point, error) {
	pts := childByHead(expr, "pts")
	if pts == nil {
		return nil, fmt.Errorf("%s is missing points", expr.head())
	}
	points := make([]point, 0, 2)
	for _, child := range pts.children[1:] {
		if child.head() != "xy" || len(child.children) < 3 || child.children[1].kind != sExprAtom || child.children[2].kind != sExprAtom {
			continue
		}
		x, err := parseNM(child.children[1].atom)
		if err != nil {
			return nil, err
		}
		y, err := parseNM(child.children[2].atom)
		if err != nil {
			return nil, err
		}
		points = append(points, point{x: x, y: y})
	}
	if len(points) < 2 {
		return nil, fmt.Errorf("%s has fewer than two points", expr.head())
	}
	segments := make([][2]point, 0, len(points)-1)
	for i := 1; i < len(points); i++ {
		segments = append(segments, [2]point{points[i-1], points[i]})
	}
	return segments, nil
}

func parsePointAt(expr *sExpr) (point, error) {
	x, y, _, err := parseAt(expr)
	if err != nil {
		return point{}, err
	}
	return point{x: x, y: y}, nil
}

func parseAt(expr *sExpr) (int64, int64, int, error) {
	at := childByHead(expr, "at")
	if at == nil || len(at.children) < 3 || at.children[1].kind != sExprAtom || at.children[2].kind != sExprAtom {
		return 0, 0, 0, fmt.Errorf("missing at")
	}
	x, err := parseNM(at.children[1].atom)
	if err != nil {
		return 0, 0, 0, err
	}
	y, err := parseNM(at.children[2].atom)
	if err != nil {
		return 0, 0, 0, err
	}
	rot := 0
	if len(at.children) >= 4 && at.children[3].kind == sExprAtom {
		rot, err = parseRotation(at.children[3].atom)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	return x, y, rot, nil
}

func parseRotation(raw string) (int, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("rotation %q", raw)
	}
	rounded := math.Round(value)
	if math.Abs(value-rounded) > 0.001 {
		return 0, fmt.Errorf("rotation %q", raw)
	}
	rot := int(rounded) % 360
	if rot < 0 {
		rot += 360
	}
	return rot, nil
}

func mirrorAxes(expr *sExpr) (bool, bool) {
	mirrorX := false
	mirrorY := false
	for _, child := range expr.children {
		if child.head() != "mirror" || len(child.children) < 2 || child.children[1].kind != sExprAtom {
			continue
		}
		switch child.children[1].atom {
		case "x":
			mirrorX = true
		case "y":
			mirrorY = true
		}
	}
	return mirrorX, mirrorY
}

func propertyValue(expr *sExpr, name string) (string, bool) {
	for _, child := range expr.children {
		if child.head() != "property" || len(child.children) < 3 {
			continue
		}
		if child.children[1].kind != sExprAtom || child.children[2].kind != sExprAtom {
			continue
		}
		if child.children[1].atom == name {
			return child.children[2].atom, true
		}
	}
	return "", false
}

func textField(expr *sExpr, name string) (string, bool) {
	node := childByHead(expr, name)
	if node == nil {
		return "", false
	}
	return atomAt(node, 1)
}

func intField(expr *sExpr, name string) (int, bool) {
	node := childByHead(expr, name)
	if node == nil || len(node.children) < 2 || node.children[1].kind != sExprAtom {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(node.children[1].atom))
	if err != nil {
		return 0, false
	}
	return value, true
}

func childByHead(expr *sExpr, head string) *sExpr {
	if expr == nil {
		return nil
	}
	for _, child := range expr.children {
		if child.head() == head {
			return child
		}
	}
	return nil
}

func atomAt(expr *sExpr, index int) (string, bool) {
	if expr == nil || index < 0 || index >= len(expr.children) || expr.children[index].kind != sExprAtom {
		return "", false
	}
	return expr.children[index].atom, true
}

func splitUnitStyle(name string) (int, int, bool) {
	i := strings.LastIndex(name, "_")
	if i < 0 {
		return 0, 0, false
	}
	j := strings.LastIndex(name[:i], "_")
	if j < 0 {
		return 0, 0, false
	}
	unit, err1 := strconv.Atoi(name[j+1 : i])
	style, err2 := strconv.Atoi(name[i+1:])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return unit, style, true
}

func (sheet *parsedSheet) insertLabels(blocks []string) (string, error) {
	idx := strings.LastIndex(sheet.src, ")")
	if idx < 0 {
		return "", fmt.Errorf("schematic has no closing parenthesis")
	}
	var b strings.Builder
	b.Grow(len(sheet.src) + len(blocks)*160)
	b.WriteString(sheet.src[:idx])
	if idx == 0 || (sheet.src[idx-1] != '\n' && sheet.src[idx-1] != '\r') {
		b.WriteString(sheet.newline)
	}
	for _, block := range blocks {
		b.WriteString(block)
	}
	b.WriteString(sheet.src[idx:])
	return b.String(), nil
}

func formatNetLabel(indent string, newline string, name string, at point) (string, error) {
	id, err := newLabelUUID()
	if err != nil {
		return "", err
	}
	i1 := indent
	i2 := indent + indent
	i3 := i2 + indent
	i4 := i3 + indent
	return i1 + "(label " + quoteSExpr(name) + newline +
		i2 + "(at " + formatNM(at.x) + " " + formatNM(at.y) + " 0)" + newline +
		i2 + "(effects" + newline +
		i3 + "(font" + newline +
		i4 + "(size 1.27 1.27)" + newline +
		i3 + ")" + newline +
		i3 + "(justify left bottom)" + newline +
		i2 + ")" + newline +
		i2 + "(uuid " + quoteSExpr(id) + ")" + newline +
		i1 + ")" + newline, nil
}

func newLabelUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("new label uuid: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	text := hex.EncodeToString(raw[:])
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32], nil
}

func quoteSExpr(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		if r == '\\' || r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func detectIndent(src string) string {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || len(trimmed) == len(line) {
			continue
		}
		return line[:len(line)-len(trimmed)]
	}
	return "\t"
}

func detectNewline(src string) string {
	if strings.Contains(src, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func canonicalSchematicNet(name string) string {
	name = strings.TrimSpace(name)
	for strings.HasPrefix(name, "/") {
		name = strings.TrimPrefix(name, "/")
	}
	return name
}

func pinNameMatches(name string, token string) bool {
	if pinTokenEqual(name, token) {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if pinTokenEqual(part, token) {
			return true
		}
	}
	return false
}

func pinTokenEqual(left string, right string) bool {
	left = normalizePinToken(left)
	right = normalizePinToken(right)
	return left != "" && left == right
}

func normalizePinToken(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '*' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parseNM(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty coordinate")
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, fmt.Errorf("coordinate %q", raw)
	}
	whole, frac, _ := strings.Cut(s, ".")
	if strings.Contains(frac, ".") || whole == "" && frac == "" {
		return 0, fmt.Errorf("coordinate %q", raw)
	}
	if whole == "" {
		whole = "0"
	}
	if !digitsOnly(whole) || (frac != "" && !digitsOnly(frac)) {
		return 0, fmt.Errorf("coordinate %q", raw)
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("coordinate %q", raw)
	}
	var f int64
	if frac != "" {
		roundUp := false
		digits := frac
		if len(digits) > 6 {
			roundUp = digits[6] >= '5'
			digits = digits[:6]
		}
		for len(digits) < 6 {
			digits += "0"
		}
		f, err = strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("coordinate %q", raw)
		}
		if roundUp {
			f++
			if f >= 1_000_000 {
				w++
				f = 0
			}
		}
	}
	v := w*1_000_000 + f
	if neg {
		v = -v
	}
	return v, nil
}

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatNM(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := v / 1_000_000
	frac := v % 1_000_000
	text := strconv.FormatInt(whole, 10)
	if frac != 0 {
		text += "." + strings.TrimRight(fmt.Sprintf("%06d", frac), "0")
	}
	if neg {
		return "-" + text
	}
	return text
}

func onSegment(a point, b point, p point) bool {
	cross := (b.x-a.x)*(p.y-a.y) - (b.y-a.y)*(p.x-a.x)
	if cross != 0 {
		return false
	}
	return p.x >= minInt(a.x, b.x) && p.x <= maxInt(a.x, b.x) && p.y >= minInt(a.y, b.y) && p.y <= maxInt(a.y, b.y)
}

func minInt(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type disjoint struct {
	parent map[point]point
}

func newDisjoint() *disjoint {
	return &disjoint{parent: map[point]point{}}
}

func (d *disjoint) add(p point) {
	if _, ok := d.parent[p]; ok {
		return
	}
	d.parent[p] = p
}

func (d *disjoint) find(p point) point {
	d.add(p)
	for d.parent[p] != p {
		d.parent[p] = d.parent[d.parent[p]]
		p = d.parent[p]
	}
	return p
}

func (d *disjoint) union(a point, b point) {
	ra := d.find(a)
	rb := d.find(b)
	if ra != rb {
		d.parent[ra] = rb
	}
}
