package infer

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/badimirzai/architon-cli/internal/ir"
)

const ambiguousPowerNetReason = "ambiguous power net name"
const multipleCandidateVoltagesReason = "multiple candidate voltages detected"

const (
	SourceUserOverride          = "USER_OVERRIDE"
	SourceNetNameExact          = "NET_NAME_EXACT"
	SourceRegulatorOutput       = "REGULATOR_OUTPUT"
	SourceSemanticAlias         = "SEMANTIC_ALIAS"
	SourceWeakSemanticAlias     = "WEAK_SEMANTIC_ALIAS"
	SourceAmbiguousNumericToken = "AMBIGUOUS_NUMERIC_TOKEN"
	SourceUnknown               = "UNKNOWN"
)

const (
	ConfidenceHigh    = "HIGH"
	ConfidenceMedium  = "MEDIUM"
	ConfidenceLow     = "LOW"
	ConfidenceUnknown = "UNKNOWN"
)

const (
	scoreUserOverride          = 1.00
	scoreNetNameExact          = 0.95
	scoreRegulatorOutput       = 0.90
	scoreSemanticAlias         = 0.80
	scoreWeakSemanticAlias     = 0.65
	scoreAmbiguousNumericToken = 0.40
	scoreUnknown               = 0.00
)

var (
	decimalVoltagePattern = regexp.MustCompile(`^\+?([0-9]+(?:\.[0-9]+)?)V$`)
	vDecimalPattern       = regexp.MustCompile(`^\+?([0-9]+)V([0-9]+)$`)
)

var ambiguousPowerNetNames = map[string]struct{}{
	"VBAT":      {},
	"VCC":       {},
	"VDD":       {},
	"VIN":       {},
	"POWER":     {},
	"PWR":       {},
	"SUPPLY":    {},
	"MOTOR_PWR": {},
}

var genericRailNameTokens = map[string]struct{}{
	"VCC":    {},
	"VDD":    {},
	"VIN":    {},
	"POWER":  {},
	"PWR":    {},
	"SUPPLY": {},
}

var knownSemanticAliases = map[string]float64{
	"VBUS": 5.0,
}

var weakSemanticAliases = map[string]struct{}{
	"VCC": {},
	"VDD": {},
}

type InferredVoltage struct {
	Net     string
	Voltage float64
	Source  string
}

type UnknownVoltage struct {
	Net    string
	Reason string
}

type Result struct {
	Voltages   map[string]InferredVoltage
	Unknowns   []UnknownVoltage
	Inferences []VoltageInference
}

// VoltageInference is the stable provenance record for a rail voltage decision.
// Voltage is nil when Architon found rail-like evidence but could not choose a
// deterministic single value.
type VoltageInference struct {
	NetName         string   `json:"net_name"`
	Voltage         *float64 `json:"voltage"`
	Source          string   `json:"source"`
	ConfidenceScore float64  `json:"confidence_score"`
	ConfidenceLevel string   `json:"confidence_level"`
	Evidence        []string `json:"evidence"`
	Warnings        []string `json:"warnings"`
}

// VoltageEvidence carries non-name evidence into inference, such as explicit
// metadata overrides or regulator outputs discovered by propagation.
type VoltageEvidence struct {
	NetName   string
	Voltage   float64
	Source    string
	BaseScore float64
	Evidence  string
}

// VoltageInferenceOptions keeps name parsing deterministic while allowing scan
// to enrich the result with metadata/regulator evidence and propagation conflicts.
type VoltageInferenceOptions struct {
	Evidence  []VoltageEvidence
	Conflicts []string
}

func InferVoltagesFromNetNames(design *ir.DesignIR) Result {
	return InferVoltages(design, VoltageInferenceOptions{})
}

// InferVoltages produces both the legacy voltage maps used by rules and the
// richer confidence/provenance records used by reports and --explain-rails.
func InferVoltages(design *ir.DesignIR, opts VoltageInferenceOptions) Result {
	result := Result{
		Voltages: map[string]InferredVoltage{},
	}

	seen := map[string]struct{}{}
	addNetName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
	}

	if design != nil {
		for _, net := range design.Nets {
			addNetName(net.Name)
		}
	}

	for _, evidence := range opts.Evidence {
		addNetName(evidence.NetName)
	}

	conflictNets := conflictMessagesByNet(opts.Conflicts)
	for netName := range conflictNets {
		addNetName(netName)
	}

	netNames := make([]string, 0, len(seen))
	for netName := range seen {
		netNames = append(netNames, netName)
	}
	sort.Strings(netNames)

	states := make(map[string]*inferenceState, len(netNames))
	for _, netName := range netNames {
		state := newInferenceState(netName)
		state.addNetNameEvidence()
		for _, conflict := range conflictNets[netName] {
			state.hasConflict = true
			state.addWarning(fmt.Sprintf("conflicting voltage evidence: %s", conflict))
		}
		states[netName] = state
	}

	externalEvidence := append([]VoltageEvidence(nil), opts.Evidence...)
	sort.Slice(externalEvidence, func(i, j int) bool {
		if externalEvidence[i].NetName != externalEvidence[j].NetName {
			return externalEvidence[i].NetName < externalEvidence[j].NetName
		}
		if externalEvidence[i].Source != externalEvidence[j].Source {
			return externalEvidence[i].Source < externalEvidence[j].Source
		}
		if externalEvidence[i].Voltage != externalEvidence[j].Voltage {
			return externalEvidence[i].Voltage < externalEvidence[j].Voltage
		}
		return externalEvidence[i].Evidence < externalEvidence[j].Evidence
	})
	for _, evidence := range externalEvidence {
		netName := strings.TrimSpace(evidence.NetName)
		if netName == "" {
			continue
		}
		state := states[netName]
		if state == nil {
			state = newInferenceState(netName)
			state.addNetNameEvidence()
			states[netName] = state
		}
		state.addExternalEvidence(evidence)
	}

	for _, netName := range netNames {
		inference := states[netName].finalize()
		result.Inferences = append(result.Inferences, inference)
		if inference.Voltage != nil {
			result.Voltages[netName] = InferredVoltage{
				Net:     netName,
				Voltage: *inference.Voltage,
				Source:  inference.Source,
			}
			continue
		}
		if reason, ok := unknownVoltageReason(states[netName], inference); ok {
			result.Unknowns = append(result.Unknowns, UnknownVoltage{
				Net:    netName,
				Reason: reason,
			})
		}
	}

	return result
}

// InferVoltageFromNetName is a small test/helper entry point for scoring a
// single net name without building a DesignIR.
func InferVoltageFromNetName(netName string) VoltageInference {
	result := InferVoltages(&ir.DesignIR{
		Nets: []ir.Net{{Name: netName}},
	}, VoltageInferenceOptions{})
	if len(result.Inferences) == 0 {
		return newInferenceState(netName).finalize()
	}
	return result.Inferences[0]
}

type voltageCandidate struct {
	voltage float64
	source  string
	score   float64
}

type inferenceState struct {
	netName                   string
	normalized                string
	candidates                []voltageCandidate
	evidence                  []string
	warnings                  []string
	hasGenericRailName        bool
	hasHeuristicExtraction    bool
	hasMultipleVoltageOptions bool
	hasConflict               bool
	noVoltageSource           string
	noVoltageScore            float64
}

func newInferenceState(netName string) *inferenceState {
	normalized := normalizeNetName(netName)
	return &inferenceState{
		netName:            strings.TrimSpace(netName),
		normalized:         normalized,
		hasGenericRailName: hasGenericRailName(normalized),
		noVoltageSource:    SourceUnknown,
		noVoltageScore:     scoreUnknown,
	}
}

// addNetNameEvidence records deterministic evidence available from the net name
// alone. Exact forms get high confidence; suffix tokens such as VCC_3V3 are
// treated as heuristic-only because the surrounding name can change meaning.
func (s *inferenceState) addNetNameEvidence() {
	if s.normalized == "" {
		s.addWarning("net name is empty")
		return
	}

	if s.normalized == "GND" {
		s.addCandidate(0, SourceNetNameExact, scoreNetNameExact)
		s.addEvidence(fmt.Sprintf("matched ground net %q", s.netName))
		return
	}

	if voltage, ok := parseExactVoltage(s.normalized); ok {
		s.addCandidate(voltage, SourceNetNameExact, scoreNetNameExact)
		s.addEvidence(fmt.Sprintf("matched exact voltage pattern %q", s.netName))
		return
	}

	if voltage, ok := knownSemanticAliases[s.normalized]; ok {
		s.addCandidate(voltage, SourceSemanticAlias, scoreSemanticAlias)
		s.addEvidence(fmt.Sprintf("matched known semantic alias %q = %s", s.normalized, formatVoltage(voltage)))
		return
	}

	if _, ok := weakSemanticAliases[s.normalized]; ok {
		s.noVoltageSource = SourceWeakSemanticAlias
		s.noVoltageScore = scoreWeakSemanticAlias
		s.addEvidence(fmt.Sprintf("matched weak semantic alias %q", s.normalized))
		s.addWarning(fmt.Sprintf("weak semantic alias %q does not identify a unique voltage", s.normalized))
		return
	}

	voltageTokens := voltageTokens(s.normalized)
	if len(voltageTokens) > 0 {
		s.hasHeuristicExtraction = true
		for _, token := range voltageTokens {
			voltage, ok := parseVoltageToken(token)
			if !ok {
				continue
			}
			s.addCandidate(voltage, SourceAmbiguousNumericToken, scoreAmbiguousNumericToken)
			s.addEvidence(fmt.Sprintf("matched ambiguous voltage token %q in net name %q", token, s.netName))
		}
	}
}

// addExternalEvidence records evidence produced elsewhere in the scan. These
// sources intentionally outrank weak/heuristic name matches when they disagree.
func (s *inferenceState) addExternalEvidence(evidence VoltageEvidence) {
	source := strings.TrimSpace(evidence.Source)
	if source == "" {
		source = SourceUnknown
	}
	score := evidence.BaseScore
	if score == 0 && source != SourceUnknown {
		score = defaultScoreForSource(source)
	}
	s.addCandidate(evidence.Voltage, source, score)

	evidenceText := strings.TrimSpace(evidence.Evidence)
	if evidenceText == "" {
		evidenceText = fmt.Sprintf("matched %s evidence for %q at %s", source, evidence.NetName, formatVoltage(evidence.Voltage))
	}
	s.addEvidence(evidenceText)
}

func (s *inferenceState) addCandidate(voltage float64, source string, score float64) {
	s.candidates = append(s.candidates, voltageCandidate{
		voltage: voltage,
		source:  source,
		score:   score,
	})
}

func (s *inferenceState) addEvidence(evidence string) {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return
	}
	for _, existing := range s.evidence {
		if existing == evidence {
			return
		}
	}
	s.evidence = append(s.evidence, evidence)
}

func (s *inferenceState) addWarning(warning string) {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return
	}
	for _, existing := range s.warnings {
		if existing == warning {
			return
		}
	}
	s.warnings = append(s.warnings, warning)
}

// finalize applies downgrade rules after all evidence has been gathered. This
// keeps scoring order-independent and makes conflict handling deterministic.
func (s *inferenceState) finalize() VoltageInference {
	source := s.noVoltageSource
	baseScore := s.noVoltageScore
	var voltage *float64

	distinctVoltages := distinctCandidateVoltages(s.candidates)
	if len(s.candidates) > 0 {
		best, hasBest := bestCandidate(s.candidates)
		if hasBest {
			source = best.source
			baseScore = best.score
			if len(distinctVoltages) == 1 || hasAuthoritativeSource(best.source) {
				v := best.voltage
				voltage = &v
			}
		}
	}

	if len(distinctVoltages) > 1 {
		s.hasMultipleVoltageOptions = true
		s.hasConflict = true
	}

	score := baseScore
	if shouldApplyGenericDowngrade(s) {
		score -= 0.20
		s.addWarning(fmt.Sprintf("generic rail name %q reduces confidence", s.netName))
	}
	if s.hasMultipleVoltageOptions {
		score -= 0.20
		s.addWarning("multiple candidate voltages detected")
	}
	if s.hasHeuristicExtraction {
		score -= 0.30
		s.addWarning("heuristic-only voltage extraction used")
	}
	if s.hasConflict {
		score -= 0.50
		s.addWarning("conflicting voltage evidence detected")
	}

	if source == "" {
		source = SourceUnknown
	}
	score = normalizeScore(score)
	if source == SourceUnknown && len(s.evidence) == 0 && len(s.warnings) == 0 {
		s.addWarning(fmt.Sprintf("no voltage evidence found in net name %q", s.netName))
	}

	sort.Strings(s.evidence)
	sort.Strings(s.warnings)

	return VoltageInference{
		NetName:         s.netName,
		Voltage:         voltage,
		Source:          source,
		ConfidenceScore: score,
		ConfidenceLevel: confidenceLevel(score),
		Evidence:        stableStrings(s.evidence),
		Warnings:        stableStrings(s.warnings),
	}
}

func normalizeNetName(netName string) string {
	trimmed := strings.TrimSpace(netName)
	trimmed = strings.TrimPrefix(trimmed, "/")
	return strings.ToUpper(trimmed)
}

func isAmbiguousPowerNetName(normalized string) bool {
	_, ok := ambiguousPowerNetNames[normalized]
	return ok
}

func parseExactVoltage(normalized string) (float64, bool) {
	if matches := decimalVoltagePattern.FindStringSubmatch(normalized); len(matches) == 2 {
		voltage, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return 0, false
		}
		return voltage, true
	}

	if voltage, ok := parseVDecimal(normalized); ok {
		return voltage, true
	}

	return 0, false
}

func parseExplicitVoltage(normalized string) (float64, bool) {
	if normalized == "GND" {
		return 0, true
	}
	return parseExactVoltage(normalized)
}

func parseVDecimal(value string) (float64, bool) {
	matches := vDecimalPattern.FindStringSubmatch(value)
	if len(matches) != 3 {
		return 0, false
	}
	voltage, err := strconv.ParseFloat(matches[1]+"."+matches[2], 64)
	if err != nil {
		return 0, false
	}
	return voltage, true
}

func parseVoltageToken(token string) (float64, bool) {
	return parseExactVoltage(strings.ToUpper(strings.TrimSpace(token)))
}

func voltageTokens(normalized string) []string {
	tokens := splitNetNameTokens(normalized)
	out := make([]string, 0, len(tokens))
	seen := map[string]struct{}{}
	for _, token := range tokens {
		if _, ok := parseVoltageToken(token); !ok {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}

func splitNetNameTokens(normalized string) []string {
	fields := strings.FieldsFunc(normalized, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '.')
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func hasGenericRailName(normalized string) bool {
	if _, ok := genericRailNameTokens[normalized]; ok {
		return true
	}
	for _, token := range splitNetNameTokens(normalized) {
		if _, ok := genericRailNameTokens[token]; ok {
			return true
		}
	}
	return false
}

func shouldApplyGenericDowngrade(s *inferenceState) bool {
	if !s.hasGenericRailName {
		return false
	}
	// Plain unknown signal names should not be penalized just because a token
	// happens to match a generic rail word; only rail-like evidence is downgraded.
	return len(s.candidates) > 0 ||
		s.noVoltageSource == SourceWeakSemanticAlias ||
		isAmbiguousPowerNetName(s.normalized)
}

func distinctCandidateVoltages(candidates []voltageCandidate) []float64 {
	out := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		found := false
		for _, existing := range out {
			if sameVoltage(existing, candidate.voltage) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, candidate.voltage)
		}
	}
	sort.Float64s(out)
	return out
}

func bestCandidate(candidates []voltageCandidate) (voltageCandidate, bool) {
	if len(candidates) == 0 {
		return voltageCandidate{}, false
	}
	sorted := append([]voltageCandidate(nil), candidates...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].score != sorted[j].score {
			return sorted[i].score > sorted[j].score
		}
		if sourcePriority(sorted[i].source) != sourcePriority(sorted[j].source) {
			return sourcePriority(sorted[i].source) > sourcePriority(sorted[j].source)
		}
		if sorted[i].source != sorted[j].source {
			return sorted[i].source < sorted[j].source
		}
		return sorted[i].voltage < sorted[j].voltage
	})
	return sorted[0], true
}

func hasAuthoritativeSource(source string) bool {
	// Authoritative sources may still provide a value when weaker evidence
	// disagrees; the conflict downgrade preserves the reduced confidence.
	switch source {
	case SourceUserOverride, SourceNetNameExact, SourceRegulatorOutput, SourceSemanticAlias:
		return true
	default:
		return false
	}
}

func sourcePriority(source string) int {
	switch source {
	case SourceUserOverride:
		return 100
	case SourceNetNameExact:
		return 90
	case SourceRegulatorOutput:
		return 80
	case SourceSemanticAlias:
		return 70
	case SourceWeakSemanticAlias:
		return 60
	case SourceAmbiguousNumericToken:
		return 50
	default:
		return 0
	}
}

func defaultScoreForSource(source string) float64 {
	switch source {
	case SourceUserOverride:
		return scoreUserOverride
	case SourceNetNameExact:
		return scoreNetNameExact
	case SourceRegulatorOutput:
		return scoreRegulatorOutput
	case SourceSemanticAlias:
		return scoreSemanticAlias
	case SourceWeakSemanticAlias:
		return scoreWeakSemanticAlias
	case SourceAmbiguousNumericToken:
		return scoreAmbiguousNumericToken
	default:
		return scoreUnknown
	}
}

func unknownVoltageReason(s *inferenceState, inference VoltageInference) (string, bool) {
	if inference.Voltage != nil {
		return "", false
	}
	if s.hasMultipleVoltageOptions {
		return multipleCandidateVoltagesReason, true
	}
	if isAmbiguousPowerNetName(s.normalized) || s.noVoltageSource == SourceWeakSemanticAlias {
		return ambiguousPowerNetReason, true
	}
	return "", false
}

func conflictMessagesByNet(messages []string) map[string][]string {
	out := map[string][]string{}
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		netName := netNameFromConflictMessage(message)
		if netName == "" {
			continue
		}
		out[netName] = append(out[netName], message)
	}
	for netName := range out {
		sort.Strings(out[netName])
	}
	return out
}

func netNameFromConflictMessage(message string) string {
	const prefix = "Voltage conflict on net "
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(message, prefix)
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:idx])
}

func confidenceLevel(score float64) string {
	switch {
	case score >= 0.90:
		return ConfidenceHigh
	case score >= 0.70:
		return ConfidenceMedium
	case score >= 0.40:
		return ConfidenceLow
	default:
		return ConfidenceUnknown
	}
}

func normalizeScore(score float64) float64 {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return math.Round(score*100) / 100
}

func sameVoltage(left float64, right float64) bool {
	return math.Abs(left-right) < 1e-9
}

func formatVoltage(voltage float64) string {
	return fmt.Sprintf("%.2fV", voltage)
}

func stableStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
