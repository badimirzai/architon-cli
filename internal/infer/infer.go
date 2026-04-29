package infer

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ir"
)

const netNameSource = "net_name"
const ambiguousPowerNetReason = "ambiguous power net name"

var (
	decimalVoltagePattern = regexp.MustCompile(`^\+?([0-9]+(?:\.[0-9]+)?)V$`)
	vDecimalPattern       = regexp.MustCompile(`^\+?([0-9]+)V([0-9]+)$`)
	suffixVoltagePattern  = regexp.MustCompile(`^.+_([0-9]+(?:\.[0-9]+)?V|[0-9]+V[0-9]+)$`)
)

var ambiguousPowerNetNames = map[string]struct{}{
	"VBAT":      {},
	"VCC":       {},
	"VIN":       {},
	"POWER":     {},
	"PWR":       {},
	"SUPPLY":    {},
	"MOTOR_PWR": {},
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
	Voltages map[string]InferredVoltage
	Unknowns []UnknownVoltage
}

func InferVoltagesFromNetNames(design *ir.DesignIR) Result {
	result := Result{
		Voltages: map[string]InferredVoltage{},
	}
	if design == nil {
		return result
	}

	netNames := make([]string, 0, len(design.Nets))
	seen := map[string]struct{}{}
	for _, net := range design.Nets {
		name := strings.TrimSpace(net.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)

	for _, netName := range netNames {
		normalized := normalizeNetName(netName)
		if voltage, ok := parseExplicitVoltage(normalized); ok {
			result.Voltages[netName] = InferredVoltage{
				Net:     netName,
				Voltage: voltage,
				Source:  netNameSource,
			}
			continue
		}
		if isAmbiguousPowerNetName(normalized) {
			result.Unknowns = append(result.Unknowns, UnknownVoltage{
				Net:    netName,
				Reason: ambiguousPowerNetReason,
			})
		}
	}

	return result
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

func parseExplicitVoltage(normalized string) (float64, bool) {
	if normalized == "GND" {
		return 0, true
	}

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

	if matches := suffixVoltagePattern.FindStringSubmatch(normalized); len(matches) == 2 {
		if voltage, ok := parseExplicitVoltage(matches[1]); ok {
			return voltage, true
		}
	}

	return 0, false
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
