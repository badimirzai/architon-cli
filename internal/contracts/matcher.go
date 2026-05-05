package contracts

import (
	"sort"
	"strings"
	"unicode"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// MatchResult describes a deterministic part-to-contract match. Ambiguous
// results deliberately have Matched=false so callers cannot accidentally apply
// a contract guessed from more than one candidate.
type MatchResult struct {
	Matched    bool
	Ambiguous  bool
	Query      string
	Kind       string
	Contract   SystemContract
	Candidates []string
}

// MatchPart matches a DesignIR part to a SystemContract by exact MPN or alias.
// It rejects ambiguity and does not do fuzzy substring search.
func MatchPart(part ir.Part, catalog []SystemContract) MatchResult {
	if len(catalog) == 0 {
		catalog = BuiltinContracts()
	}
	queries := partMatchQueries(part)
	for _, query := range queries {
		result := matchPartQuery(query.value, query.kind, catalog)
		if result.Matched || result.Ambiguous {
			result.Query = query.value
			return result
		}
	}
	return MatchResult{}
}

type partMatchQuery struct {
	value string
	kind  string
}

func partMatchQueries(part ir.Part) []partMatchQuery {
	out := make([]partMatchQuery, 0, 6)
	add := func(value string, kind string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range out {
			if normalizeMPN(existing.value) == normalizeMPN(value) && existing.kind == kind {
				return
			}
		}
		out = append(out, partMatchQuery{value: value, kind: kind})
	}

	add(part.MPN, "exact_mpn")
	for _, key := range sortedFieldKeys(part.Fields) {
		if isMPNFieldKey(key) {
			add(part.Fields[key], "exact_mpn")
		}
	}
	add(part.Value, "value")
	return out
}

func matchPartQuery(value string, kind string, catalog []SystemContract) MatchResult {
	target := normalizeMPN(value)
	if target == "" {
		return MatchResult{}
	}

	exact := make([]SystemContract, 0, 1)
	alias := make([]SystemContract, 0, 1)
	for _, contract := range catalog {
		// Exact MPN matches win over aliases. Both comparisons use normalized
		// tokens so common punctuation differences do not change the result.
		if normalizeMPN(contract.MPN) == target {
			exact = append(exact, contract)
			continue
		}
		for _, candidate := range contract.Aliases {
			if normalizeMPN(candidate) == target {
				alias = append(alias, contract)
				break
			}
		}
	}

	if len(exact) > 0 {
		return buildMatchResult(exact, kind)
	}
	if len(alias) > 0 {
		return buildMatchResult(alias, "alias")
	}
	return MatchResult{}
}

func buildMatchResult(matches []SystemContract, kind string) MatchResult {
	matches = uniqueContracts(matches)
	if len(matches) == 1 {
		return MatchResult{
			Matched:  true,
			Kind:     kind,
			Contract: cloneSystemContracts(matches)[0],
		}
	}
	candidates := make([]string, 0, len(matches))
	for _, match := range matches {
		candidates = append(candidates, match.MPN)
	}
	sort.Strings(candidates)
	return MatchResult{
		Ambiguous:  true,
		Kind:       kind,
		Candidates: candidates,
	}
}

func uniqueContracts(in []SystemContract) []SystemContract {
	out := make([]SystemContract, 0, len(in))
	seen := map[string]struct{}{}
	for _, contract := range in {
		key := normalizeMPN(contract.MPN)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, contract)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MPN < out[j].MPN })
	return out
}

func normalizeMPN(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range strings.ToUpper(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func contractPartMPN(part ir.Part) string {
	if strings.TrimSpace(part.MPN) != "" {
		return strings.TrimSpace(part.MPN)
	}
	for _, key := range sortedFieldKeys(part.Fields) {
		if isMPNFieldKey(key) && strings.TrimSpace(part.Fields[key]) != "" {
			return strings.TrimSpace(part.Fields[key])
		}
	}
	return strings.TrimSpace(part.Value)
}

func sortedFieldKeys(fields map[string]string) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isMPNFieldKey(key string) bool {
	normalized := normalizeFieldKey(key)
	switch normalized {
	case "mpn", "manufacturer_part_number", "part_number", "mfr_part_number", "mfr_pn", "pn":
		return true
	default:
		return false
	}
}
