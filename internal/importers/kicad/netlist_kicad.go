package kicad

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// ImportKiCadNetlist imports a KiCad 9+ Eeschema S-expression netlist into DesignIR.
func ImportKiCadNetlist(path string) (*ir.DesignIR, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read KiCad netlist: %w", err)
	}

	parser := newSExprParser(string(content))
	rootExprs, err := parser.parseAll()
	if err != nil {
		return nil, fmt.Errorf("parse KiCad netlist: %w", err)
	}

	design := &ir.DesignIR{
		Version: ir.SchemaVersion,
		Source:  "kicad_netlist_sexpr",
		Metadata: ir.IRMetadata{
			InputFile: path,
			ParsedAt:  time.Now().UTC().Format(time.RFC3339),
		},
	}

	componentsSection := findSection(rootExprs, "components")
	if componentsSection != nil {
		design.Parts = parseNetlistComponents(componentsSection)
	}

	netsSection := findSection(rootExprs, "nets")
	if netsSection != nil {
		design.Nets, err = parseNetlistNets(netsSection)
		if err != nil {
			return nil, err
		}
	}

	if design.Version == "" {
		design.Version = ir.SchemaVersion
	}

	sortPartsByRef(design.Parts)
	sortNetsByName(design.Nets)
	return design, nil
}

type sExprKind int

const (
	sExprAtom sExprKind = iota
	sExprList
)

type sExpr struct {
	kind     sExprKind
	atom     string
	children []*sExpr
}

func (expr *sExpr) head() string {
	if expr == nil || expr.kind != sExprList || len(expr.children) == 0 || expr.children[0].kind != sExprAtom {
		return ""
	}
	return expr.children[0].atom
}

func (expr *sExpr) fieldValue(name string) (string, bool) {
	if expr == nil || expr.kind != sExprList {
		return "", false
	}
	for _, child := range expr.children[1:] {
		if child == nil || child.kind != sExprList || child.head() != name || len(child.children) < 2 {
			continue
		}
		if child.children[1].kind != sExprAtom {
			continue
		}
		return child.children[1].atom, true
	}
	return "", false
}

type sExprTokenKind int

const (
	tokenEOF sExprTokenKind = iota
	tokenLParen
	tokenRParen
	tokenAtom
)

type sExprToken struct {
	kind  sExprTokenKind
	value string
}

type sExprTokenizer struct {
	input []rune
	pos   int
}

func newSExprTokenizer(input string) *sExprTokenizer {
	return &sExprTokenizer{input: []rune(input)}
}

func (t *sExprTokenizer) nextToken() (sExprToken, error) {
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		switch {
		case unicode.IsSpace(ch):
			t.pos++
		case ch == ';':
			for t.pos < len(t.input) && t.input[t.pos] != '\n' {
				t.pos++
			}
		case ch == '(':
			t.pos++
			return sExprToken{kind: tokenLParen}, nil
		case ch == ')':
			t.pos++
			return sExprToken{kind: tokenRParen}, nil
		case ch == '"':
			return t.readQuotedAtom()
		default:
			return t.readBareAtom(), nil
		}
	}
	return sExprToken{kind: tokenEOF}, nil
}

func (t *sExprTokenizer) readQuotedAtom() (sExprToken, error) {
	t.pos++

	var builder strings.Builder
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if ch == '"' {
			t.pos++
			return sExprToken{kind: tokenAtom, value: builder.String()}, nil
		}
		if ch == '\\' {
			t.pos++
			if t.pos >= len(t.input) {
				return sExprToken{}, fmt.Errorf("unterminated escape sequence")
			}
			switch escaped := t.input[t.pos]; escaped {
			case 'n':
				builder.WriteRune('\n')
			case 'r':
				builder.WriteRune('\r')
			case 't':
				builder.WriteRune('\t')
			case '"', '\\':
				builder.WriteRune(escaped)
			default:
				builder.WriteRune(escaped)
			}
			t.pos++
			continue
		}
		builder.WriteRune(ch)
		t.pos++
	}

	return sExprToken{}, fmt.Errorf("unterminated quoted string")
}

func (t *sExprTokenizer) readBareAtom() sExprToken {
	start := t.pos
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if unicode.IsSpace(ch) || ch == '(' || ch == ')' || ch == ';' {
			break
		}
		t.pos++
	}
	return sExprToken{kind: tokenAtom, value: string(t.input[start:t.pos])}
}

type sExprParser struct {
	tokenizer *sExprTokenizer
	peeked    sExprToken
	hasPeeked bool
}

func newSExprParser(input string) *sExprParser {
	return &sExprParser{tokenizer: newSExprTokenizer(input)}
}

func (p *sExprParser) parseAll() ([]*sExpr, error) {
	exprs := make([]*sExpr, 0, 1)
	for {
		token, err := p.peek()
		if err != nil {
			return nil, err
		}
		if token.kind == tokenEOF {
			return exprs, nil
		}
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
	}
}

func (p *sExprParser) parseExpr() (*sExpr, error) {
	token, err := p.next()
	if err != nil {
		return nil, err
	}

	switch token.kind {
	case tokenAtom:
		return &sExpr{kind: sExprAtom, atom: token.value}, nil
	case tokenLParen:
		children := make([]*sExpr, 0, 4)
		for {
			nextToken, err := p.peek()
			if err != nil {
				return nil, err
			}
			if nextToken.kind == tokenEOF {
				return nil, fmt.Errorf("unterminated s-expression")
			}
			if nextToken.kind == tokenRParen {
				_, _ = p.next()
				return &sExpr{kind: sExprList, children: children}, nil
			}
			child, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			children = append(children, child)
		}
	case tokenRParen:
		return nil, fmt.Errorf("unexpected closing parenthesis")
	case tokenEOF:
		return nil, fmt.Errorf("unexpected end of input")
	default:
		return nil, fmt.Errorf("unexpected token")
	}
}

func (p *sExprParser) peek() (sExprToken, error) {
	if p.hasPeeked {
		return p.peeked, nil
	}
	token, err := p.tokenizer.nextToken()
	if err != nil {
		return sExprToken{}, err
	}
	p.peeked = token
	p.hasPeeked = true
	return token, nil
}

func (p *sExprParser) next() (sExprToken, error) {
	if p.hasPeeked {
		token := p.peeked
		p.hasPeeked = false
		return token, nil
	}
	return p.tokenizer.nextToken()
}

func findSection(exprs []*sExpr, sectionName string) *sExpr {
	for _, expr := range exprs {
		if expr == nil {
			continue
		}
		if expr.head() == sectionName {
			return expr
		}
		if expr.kind == sExprList {
			if nested := findSection(expr.children[1:], sectionName); nested != nil {
				return nested
			}
		}
	}
	return nil
}

func parseNetlistComponents(section *sExpr) []ir.Part {
	parts := make([]ir.Part, 0)
	for _, child := range section.children[1:] {
		if child == nil || child.kind != sExprList || child.head() != "comp" {
			continue
		}

		ref, ok := child.fieldValue("ref")
		if !ok || strings.TrimSpace(ref) == "" {
			continue
		}

		part := ir.Part{Ref: ref}
		if value, ok := child.fieldValue("value"); ok {
			part.Value = value
		}
		if footprint, ok := child.fieldValue("footprint"); ok {
			part.Footprint = footprint
		}
		parts = append(parts, part)
	}
	return parts
}

func parseNetlistNets(section *sExpr) ([]ir.Net, error) {
	nets := make([]ir.Net, 0)
	for _, child := range section.children[1:] {
		if child == nil || child.kind != sExprList || child.head() != "net" {
			continue
		}

		name, ok := child.fieldValue("name")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("net missing required name")
		}

		net := ir.Net{Name: name}
		for _, netChild := range child.children[1:] {
			if netChild == nil || netChild.kind != sExprList || netChild.head() != "node" {
				continue
			}

			ref, hasRef := netChild.fieldValue("ref")
			if !hasRef || strings.TrimSpace(ref) == "" {
				return nil, fmt.Errorf("net %q node missing required ref", name)
			}
			pin, hasPin := netChild.fieldValue("pin")
			if !hasPin || strings.TrimSpace(pin) == "" {
				return nil, fmt.Errorf("net %q node missing required pin", name)
			}
			net.Pins = append(net.Pins, ir.PinRef{Ref: ref, Pin: pin})
		}

		sortPinsByRef(net.Pins)
		nets = append(nets, net)
	}
	return nets, nil
}

func sortPartsByRef(parts []ir.Part) {
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].Ref != parts[j].Ref {
			return parts[i].Ref < parts[j].Ref
		}
		if parts[i].Value != parts[j].Value {
			return parts[i].Value < parts[j].Value
		}
		return parts[i].Footprint < parts[j].Footprint
	})
}

func sortNetsByName(nets []ir.Net) {
	sort.Slice(nets, func(i, j int) bool {
		if nets[i].Name != nets[j].Name {
			return nets[i].Name < nets[j].Name
		}
		return len(nets[i].Pins) < len(nets[j].Pins)
	})
	for i := range nets {
		sortPinsByRef(nets[i].Pins)
	}
}

func sortPinsByRef(pins []ir.PinRef) {
	sort.Slice(pins, func(i, j int) bool {
		if pins[i].Ref != pins[j].Ref {
			return pins[i].Ref < pins[j].Ref
		}
		return pins[i].Pin < pins[j].Pin
	})
}
