package cmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/badimirzai/architon-cli/internal/importers/kicad"
	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
	"github.com/badimirzai/architon-cli/internal/propagate"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/rules"
	"github.com/badimirzai/architon-cli/internal/ui"
	"github.com/spf13/cobra"
)

const defaultScanReportPath = "architon-report.json"
const noScanInputsFoundInProjectDirMessage = "no BOM or netlist file found in project directory (expected bom/bom.csv, bom.csv, exports/bom.csv, or *bom*.csv in root/bom/exports/, plus exports/*.net or *.net in root)"

var scanCmd = newScanCmd()

type resolvedScanInput struct {
	DirectPath        string
	Directory         bool
	ProjectPath       string
	BOMPath           string
	NetlistPath       string
	BOMDiscovered     bool
	NetlistDiscovered bool
}

func init() {
	rootCmd.AddCommand(scanCmd)
}

func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Scan an electronics BOM and generate a deterministic verification report",
		Long: `Scan an electronics BOM and generate a deterministic verification report.

Current supported input:
  - KiCad BOM CSV
  - KiCad .net S-expression netlist

Examples:
  rv scan .
  rv scan bom.csv
  rv scan exports/example.net
  rv scan bom.csv --map mapping.yaml
  rv scan bom.csv --out result.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mappingFile, _ := cmd.Flags().GetString("map")
			outputPath, _ := cmd.Flags().GetString("out")
			bomOverride, _ := cmd.Flags().GetString("bom")
			netlistOverride, _ := cmd.Flags().GetString("netlist")
			metaOverride, _ := cmd.Flags().GetString("meta")

			resolvedInput, err := resolveScanInput(args[0], bomOverride, netlistOverride)
			if err != nil {
				return fatalError(err)
			}

			design, err := importResolvedScanInput(resolvedInput, mappingFile)
			if err != nil {
				return userError(err)
			}

			designReport := report.NewVerificationReport(design)
			inferRes := infer.InferVoltagesFromNetNames(design)

			// -------------------------
			// META: auto-discover + skeleton generation (directory scans only)
			// -------------------------
			metaPath := strings.TrimSpace(metaOverride)
			metaExplicit := metaPath != ""
			skeletonCreated := false

			if resolvedInput.Directory {
				defaultMetaPath := filepath.Join(resolvedInput.ProjectPath, ".architon", "meta.yaml")

				if !metaExplicit {
					// Auto-discover existing meta.yaml
					if _, err := os.Stat(defaultMetaPath); err == nil {
						metaPath = defaultMetaPath
					} else if os.IsNotExist(err) {
						// Auto-generate skeleton only if netlist/nets exist
						if designReport.Summary.Nets > 0 {
							if err := meta.WriteSkeleton(defaultMetaPath, scanDetectURefs(design)); err != nil {
								return internalError(fmt.Errorf("create meta skeleton: %w", err))
							}
							skeletonCreated = true
							fmt.Fprintf(cmd.OutOrStdout(), "Created .architon/meta.yaml — fill sources/components to enable voltage rules\n")
							// Do not load the generated placeholder file on the same run.
							metaPath = ""
						}
					} else {
						return internalError(fmt.Errorf("stat default meta: %w", err))
					}
				} else {
					// If user explicitly set --meta, respect it as-is
					metaPath = filepath.Clean(metaPath)
				}
			}

			// -------------------------
			// META: load + decide whether to run rules
			// -------------------------
			metaObj := &meta.Meta{}
			metaLoaded := false

			if metaPath != "" && !skeletonCreated {
				// meta-based rules require nets
				if designReport.Summary.Nets == 0 {
					return &ExitError{
						Code: 3,
						Err:  errors.New("--meta requires a netlist (no nets present in DesignIR)"),
					}
				}

				parsed, err := meta.Parse(metaPath)
				if err != nil {
					return &ExitError{
						Code: 3,
						Err:  fmt.Errorf("meta load failed: %w", err),
					}
				}

				if metaExplicit {
					// Explicit override: must be valid, otherwise tool failure
					if err := meta.ValidateStrict(parsed); err != nil {
						return &ExitError{
							Code: 3,
							Err:  fmt.Errorf("meta invalid: %w", err),
						}
					}
					metaObj = parsed
					metaLoaded = true
				} else {
					// Auto-discovered: use only completed entries, so skeleton placeholders stay informational.
					prepared := scanPrepareAutoMeta(parsed)
					if scanMetaHasEntries(prepared) {
						if err := meta.ValidateStrict(prepared); err != nil {
							return &ExitError{
								Code: 3,
								Err:  fmt.Errorf("meta invalid: %w", err),
							}
						}
						metaObj = prepared
						metaLoaded = true
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "Hint: edit .architon/meta.yaml (sources/components) to enable voltage rules\n")
					}
				}
			}

			// -------------------------
			// VOLTAGE PROPAGATION + RULES
			// -------------------------
			initialVoltages := scanInitialVoltages(inferRes, metaObj)
			propRes := propagate.Result{NetVoltages: map[string]propagate.NetVoltage{}}
			if designReport.Summary.Nets > 0 && len(initialVoltages) > 0 {
				propRes = propagate.Propagate(*design, *metaObj, initialVoltages)
			}

			netVolts := scanReportNetVoltages(propRes.NetVoltages)
			inferredNetVolts := scanReportInferredNetVoltages(inferRes)
			unknownVoltageNets := scanReportUnknownVoltageNets(inferRes)
			if len(netVolts) > 0 || len(inferredNetVolts) > 0 || len(unknownVoltageNets) > 0 || len(propRes.Conflicts) > 0 {
				designReport.Derived = &report.Derived{
					NetVoltages:         netVolts,
					InferredNetVoltages: inferredNetVolts,
					UnknownVoltageNets:  unknownVoltageNets,
					Conflicts:           propRes.Conflicts,
				}
			}

			if len(propRes.Conflicts) > 0 {
				// Conflicts are violations (severity error => exit 2)
				for _, c := range propRes.Conflicts {
					designReport.Rules = append(designReport.Rules, report.RuleResult{
						ID:       "RULE_VOLTAGE_CONFLICT",
						Severity: "error",
						Message:  c,
					})
				}
			}

			if metaLoaded && len(propRes.NetVoltages) > 0 {
				// Overvoltage violations
				designReport.Rules = append(designReport.Rules, rules.Overvoltage(design, metaObj, propRes.NetVoltages)...)
			}

			if len(designReport.Rules) > 0 {
				// Deterministic rule ordering
				sort.SliceStable(designReport.Rules, func(i, j int) bool {
					if designReport.Rules[i].ID == designReport.Rules[j].ID {
						return designReport.Rules[i].Message < designReport.Rules[j].Message
					}
					return designReport.Rules[i].ID < designReport.Rules[j].ID
				})
			}
			// Update summary (because report.NewVerificationReport() computed these before rules existed)
			designReport.Summary.Rules = len(designReport.Rules)
			designReport.Summary.HasFailures = len(design.ParseErrors) > 0 || scanRuleViolationCount(designReport) > 0

			// Write report JSON after derived/rules are attached
			if err := report.WriteVerificationReport(outputPath, designReport); err != nil {
				return internalError(err)
			}

			// -------------------------
			// 0 clean/info only
			// 1 warnings detected
			// 2 violations detected (severity error)
			// 3 tool execution failure
			// -------------------------
			exitCode := scanExitCode(designReport)

			fmt.Fprintf(cmd.OutOrStdout(), "ARCHITON SCAN\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Target: %s\n", args[0])
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"Result: %s — %s\n",
				ui.Colorize(scanExitColorToken(exitCode), scanResultLabel(exitCode)),
				scanResultExplanation(exitCode),
			)
			fmt.Fprintf(cmd.OutOrStdout(), "Parts: %d\n", designReport.Summary.Parts)
			fmt.Fprintf(cmd.OutOrStdout(), "Nets: %d\n", designReport.Summary.Nets)
			fmt.Fprintf(cmd.OutOrStdout(), "Errors: %d\n", designReport.Summary.ParseErrorsCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Warnings: %d\n", designReport.Summary.ParseWarningsCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Rules: %d\n", designReport.Summary.Rules)
			fmt.Fprintf(cmd.OutOrStdout(), "Violations: %d\n", scanRuleViolationCount(designReport))
			fmt.Fprintf(cmd.OutOrStdout(), "Inferred voltages: %d Unknown voltage nets: %d\n", len(inferRes.Voltages), len(inferRes.Unknowns))

			if len(designReport.Rules) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Rule findings:\n")
				for _, rule := range designReport.Rules {
					severity := strings.ToUpper(strings.TrimSpace(rule.Severity))
					if severity == "" {
						severity = "ERROR"
					}
					line := fmt.Sprintf("- %s %s: %s", severity, rule.ID, rule.Message)
					fmt.Fprintln(cmd.OutOrStdout(), ui.Colorize(scanRuleColorToken(severity), line))
				}
			}

			if resolvedInput.BOMDiscovered {
				fmt.Fprintf(cmd.OutOrStdout(), "Detected BOM: %s\n", resolvedInput.BOMPath)
			}
			if resolvedInput.NetlistDiscovered {
				fmt.Fprintf(cmd.OutOrStdout(), "Detected Netlist: %s\n", resolvedInput.NetlistPath)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", outputPath)
			fmt.Fprintln(cmd.OutOrStdout(), ui.Colorize(scanExitColorToken(exitCode), fmt.Sprintf("exit code: %d", exitCode)))

			if exitCode == 0 {
				return nil
			}

			switch exitCode {
			case 1:
				return &ExitError{
					Code: 1,
					Err:  errors.New(ui.Colorize("WARN", fmt.Sprintf("scan completed with %d warning(s); wrote %s", scanRuleWarningCount(designReport), outputPath))),
				}
			case 2:
				return &ExitError{
					Code: 2,
					Err:  errors.New(ui.Colorize("ERROR", fmt.Sprintf("scan completed with %d violation(s); wrote %s", scanRuleViolationCount(designReport), outputPath))),
				}
			case 3:
				return &ExitError{
					Code: 3,
					Err:  errors.New(ui.Colorize("ERROR", fmt.Sprintf("scan completed with %d parse error(s); wrote %s", designReport.Summary.ParseErrorsCount, outputPath))),
				}
			default:
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("scan failed with unexpected exit code %d", exitCode),
				}
			}
		},
	}

	cmd.Flags().String("map", "", "Path to YAML file with explicit BOM header mapping")
	cmd.Flags().String("out", defaultScanReportPath, "Path to write the scan report JSON")
	cmd.Flags().String("bom", "", "Override BOM file path when scanning a project directory")
	cmd.Flags().String("netlist", "", "Override netlist file path when scanning a project directory")
	cmd.Flags().String("meta", "", "Override meta file path (default: .architon/meta.yaml if present; auto-generated if missing)")
	return cmd
}

func importResolvedScanInput(input resolvedScanInput, mappingFile string) (*ir.DesignIR, error) {
	if !input.Directory {
		return importDirectScanPath(input.DirectPath, mappingFile)
	}

	switch {
	case input.BOMPath != "" && input.NetlistPath != "":
		mapping, err := loadScanMapping(mappingFile)
		if err != nil {
			return nil, err
		}
		bomDesign, err := kicad.ImportKiCadBOM(input.BOMPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("import KiCad BOM: %w", err)
		}
		netlistDesign, err := kicad.ImportKiCadNetlist(input.NetlistPath)
		if err != nil {
			return nil, fmt.Errorf("import KiCad netlist: %w", err)
		}
		return ir.MergeProjectIR(bomDesign, netlistDesign, input.ProjectPath, time.Now()), nil
	case input.BOMPath != "":
		mapping, err := loadScanMapping(mappingFile)
		if err != nil {
			return nil, err
		}
		design, err := kicad.ImportKiCadBOM(input.BOMPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("import KiCad BOM: %w", err)
		}
		return design, nil
	case input.NetlistPath != "":
		design, err := kicad.ImportKiCadNetlist(input.NetlistPath)
		if err != nil {
			return nil, fmt.Errorf("import KiCad netlist: %w", err)
		}
		return design, nil
	default:
		return nil, errors.New(noScanInputsFoundInProjectDirMessage)
	}
}

func importDirectScanPath(path string, mappingFile string) (*ir.DesignIR, error) {
	format, err := detectScanInputFormat(path)
	if err != nil {
		return nil, err
	}

	switch format {
	case "csv":
		mapping, err := loadScanMapping(mappingFile)
		if err != nil {
			return nil, err
		}
		design, err := kicad.ImportKiCadBOM(path, mapping)
		if err != nil {
			return nil, fmt.Errorf("import KiCad BOM: %w", err)
		}
		return design, nil
	case "netlist":
		design, err := kicad.ImportKiCadNetlist(path)
		if err != nil {
			return nil, fmt.Errorf("import KiCad netlist: %w", err)
		}
		return design, nil
	default:
		return nil, fmt.Errorf("unsupported input format for %q (currently supported: CSV, .net)", path)
	}
}

func loadScanMapping(mappingFile string) (kicad.ColumnMapping, error) {
	if mappingFile == "" {
		return kicad.ColumnMapping{}, nil
	}

	mapping, err := kicad.LoadColumnMapping(mappingFile)
	if err != nil {
		return kicad.ColumnMapping{}, fmt.Errorf("load mapping: %w", err)
	}
	return mapping, nil
}

func resolveScanInput(inputPath string, bomOverride string, netlistOverride string) (resolvedScanInput, error) {
	cleanInput := filepath.Clean(inputPath)

	info, statErr := os.Stat(cleanInput)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return resolvedScanInput{DirectPath: cleanInput}, nil
		}
		return resolvedScanInput{}, fmt.Errorf("stat input path: %w", statErr)
	}
	if !info.IsDir() {
		return resolvedScanInput{DirectPath: cleanInput}, nil
	}

	absInput, err := filepath.Abs(cleanInput)
	if err != nil {
		return resolvedScanInput{}, fmt.Errorf("resolve project directory: %w", err)
	}
	absInput = filepath.Clean(absInput)

	resolved := resolvedScanInput{
		Directory:   true,
		ProjectPath: absInput,
	}

	if bomOverride != "" {
		resolved.BOMPath = filepath.Clean(bomOverride)
	} else {
		resolved.BOMPath, err = resolveDetectedBOMPath(absInput)
		if err != nil {
			return resolvedScanInput{}, err
		}
		resolved.BOMDiscovered = resolved.BOMPath != ""
	}

	if netlistOverride != "" {
		resolved.NetlistPath = filepath.Clean(netlistOverride)
	} else {
		resolved.NetlistPath, err = resolveDetectedNetlistPath(absInput)
		if err != nil {
			return resolvedScanInput{}, err
		}
		resolved.NetlistDiscovered = resolved.NetlistPath != ""
	}

	if resolved.BOMPath == "" && resolved.NetlistPath == "" {
		return resolvedScanInput{}, errors.New(noScanInputsFoundInProjectDirMessage)
	}

	return resolved, nil
}

func resolveDetectedBOMPath(absInput string) (string, error) {
	for _, relPath := range [][]string{
		{"bom", "bom.csv"},
		{"bom.csv"},
		{"exports", "bom.csv"},
	} {
		candidate := filepath.Clean(filepath.Join(append([]string{absInput}, relPath...)...))
		candidateInfo, candidateErr := os.Stat(candidate)
		if candidateErr == nil {
			if candidateInfo.IsDir() {
				continue
			}
			return candidate, nil
		}
		if !os.IsNotExist(candidateErr) {
			return "", fmt.Errorf("stat BOM candidate: %w", candidateErr)
		}
	}

	for _, tierDir := range []string{
		filepath.Join(absInput, "bom"),
		filepath.Join(absInput, "exports"),
		absInput,
	} {
		candidates, err := findBOMCandidates(tierDir)
		if err != nil {
			return "", err
		}
		if len(candidates) > 0 {
			return candidates[0], nil
		}
	}

	return "", nil
}

func findBOMCandidates(dir string) ([]string, error) {
	absDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, fmt.Errorf("resolve BOM candidate directory: %w", err)
	}

	entries, readErr := os.ReadDir(absDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("read BOM candidate directory: %w", readErr)
	}

	matches := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !matchesBOMCSVPattern(entry.Name()) {
			continue
		}
		matches = append(matches, filepath.Join(absDir, entry.Name()))
	}

	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := filepath.Rel(absDir, matches[i])
		right, rightErr := filepath.Rel(absDir, matches[j])
		if leftErr != nil || rightErr != nil {
			return filepath.ToSlash(matches[i]) < filepath.ToSlash(matches[j])
		}
		return filepath.ToSlash(left) < filepath.ToSlash(right)
	})

	return matches, nil
}

func resolveDetectedNetlistPath(absInput string) (string, error) {
	for _, tierDir := range []string{
		filepath.Join(absInput, "exports"),
		absInput,
	} {
		candidates, err := findNetlistCandidates(tierDir)
		if err != nil {
			return "", err
		}
		if len(candidates) > 0 {
			return candidates[0], nil
		}
	}
	return "", nil
}

func findNetlistCandidates(dir string) ([]string, error) {
	absDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, fmt.Errorf("resolve netlist candidate directory: %w", err)
	}

	entries, readErr := os.ReadDir(absDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("read netlist candidate directory: %w", readErr)
	}

	matches := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !matchesNetlistPattern(entry.Name()) {
			continue
		}
		matches = append(matches, filepath.Join(absDir, entry.Name()))
	}

	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := filepath.Rel(absDir, matches[i])
		right, rightErr := filepath.Rel(absDir, matches[j])
		if leftErr != nil || rightErr != nil {
			return filepath.ToSlash(matches[i]) < filepath.ToSlash(matches[j])
		}
		return filepath.ToSlash(left) < filepath.ToSlash(right)
	})

	return matches, nil
}

func matchesBOMCSVPattern(name string) bool {
	if !strings.EqualFold(filepath.Ext(name), ".csv") {
		return false
	}

	lowerName := strings.ToLower(name)
	return strings.HasSuffix(lowerName, ".bom.csv") || strings.Contains(lowerName, "bom")
}

func matchesNetlistPattern(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".net")
}

// Exit codes (consistent with rv check contract):
// 0 = clean/info only
// 1 = warnings detected
// 2 = violations detected (severity=error)
// 3 = tool execution failure (analysis could not complete)
func scanResultLabel(exitCode int) string {
	switch exitCode {
	case 0:
		return "OK"
	case 1:
		return "WARN"
	case 2:
		return "FAIL"
	default:
		return "ERROR"
	}
}

func scanResultExplanation(exitCode int) string {
	switch exitCode {
	case 0:
		return "no scan violations detected"
	case 1:
		return "scan warnings detected"
	case 2:
		return "scan violations detected"
	default:
		return "scan could not complete reliably"
	}
}

func scanExitColorToken(exitCode int) string {
	switch exitCode {
	case 0:
		return "OK"
	case 1:
		return "WARN"
	default:
		return "ERROR"
	}
}

func scanRuleColorToken(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "", "error":
		return "ERROR"
	case "warn", "warning":
		return "WARN"
	case "info":
		return "INFO"
	default:
		return ""
	}
}

func scanExitCode(result report.VerificationReport) int {
	// Parse errors mean analysis could not complete reliably.
	if result.Summary.ParseErrorsCount > 0 {
		return 3
	}

	hasWarn := false
	hasErr := false
	for _, rule := range result.Rules {
		sev := strings.TrimSpace(strings.ToLower(rule.Severity))
		if sev == "" || sev == "error" {
			hasErr = true
		} else if sev == "warning" {
			hasWarn = true
		}
	}

	// Also treat parse warnings as warnings when no violations exist
	if !hasErr && result.Summary.ParseWarningsCount > 0 {
		hasWarn = true
	}

	if hasErr {
		return 2
	}
	if hasWarn {
		return 1
	}
	return 0
}

func scanRuleViolationCount(result report.VerificationReport) int {
	n := 0
	for _, rule := range result.Rules {
		sev := strings.TrimSpace(strings.ToLower(rule.Severity))
		if sev == "" || sev == "error" {
			n++
		}
	}
	return n
}

func scanRuleWarningCount(result report.VerificationReport) int {
	n := 0
	for _, rule := range result.Rules {
		sev := strings.TrimSpace(strings.ToLower(rule.Severity))
		if sev == "warning" {
			n++
		}
	}
	// include parse warnings as warnings too
	n += result.Summary.ParseWarningsCount
	return n
}

func scanDetectURefs(design *ir.DesignIR) []string {
	seen := map[string]struct{}{}
	refs := make([]string, 0, 16)

	for _, net := range design.Nets {
		for _, p := range net.Pins {
			r := strings.TrimSpace(p.Ref)
			if r == "" {
				continue
			}
			if strings.HasPrefix(strings.ToUpper(r), "U") {
				if _, ok := seen[r]; ok {
					continue
				}
				seen[r] = struct{}{}
				refs = append(refs, r)
			}
		}
	}

	sort.Strings(refs)
	if len(refs) > 10 {
		refs = refs[:10]
	}
	return refs
}

func scanInitialVoltages(inferRes infer.Result, m *meta.Meta) map[string]float64 {
	initial := make(map[string]float64, len(inferRes.Voltages))
	for net, inferred := range inferRes.Voltages {
		initial[net] = inferred.Voltage
	}
	if m != nil {
		for _, source := range m.Sources {
			initial[source.Net] = source.Voltage
		}
	}
	return initial
}

func scanReportNetVoltages(netVoltages map[string]propagate.NetVoltage) []report.NetVoltage {
	out := make([]report.NetVoltage, 0, len(netVoltages))
	for _, nv := range netVoltages {
		out = append(out, report.NetVoltage{
			Net:     nv.Net,
			Voltage: nv.Voltage,
			Source:  nv.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Net < out[j].Net })
	return out
}

func scanReportInferredNetVoltages(inferRes infer.Result) []report.NetVoltage {
	out := make([]report.NetVoltage, 0, len(inferRes.Voltages))
	for _, inferred := range inferRes.Voltages {
		out = append(out, report.NetVoltage{
			Net:     inferred.Net,
			Voltage: inferred.Voltage,
			Source:  inferred.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Net < out[j].Net })
	return out
}

func scanReportUnknownVoltageNets(inferRes infer.Result) []report.UnknownVoltageNet {
	out := make([]report.UnknownVoltageNet, 0, len(inferRes.Unknowns))
	for _, unknown := range inferRes.Unknowns {
		out = append(out, report.UnknownVoltageNet{
			Net:    unknown.Net,
			Reason: unknown.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Net != out[j].Net {
			return out[i].Net < out[j].Net
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func scanPrepareAutoMeta(m *meta.Meta) *meta.Meta {
	if m == nil {
		return &meta.Meta{}
	}

	prepared := &meta.Meta{
		Version: m.Version,
	}
	for _, source := range m.Sources {
		if source.Voltage == 0 {
			continue
		}
		prepared.Sources = append(prepared.Sources, source)
	}
	for _, regulator := range m.Regulators {
		if strings.TrimSpace(regulator.Ref) == "" &&
			strings.TrimSpace(regulator.InPin) == "" &&
			strings.TrimSpace(regulator.OutPin) == "" &&
			regulator.OutVoltage == 0 {
			continue
		}
		prepared.Regulators = append(prepared.Regulators, regulator)
	}
	for _, component := range m.Components {
		if component.MaxVoltage == 0 {
			continue
		}
		prepared.Components = append(prepared.Components, component)
	}
	return prepared
}

func scanMetaHasEntries(m *meta.Meta) bool {
	if m == nil {
		return false
	}
	return len(m.Sources) > 0 || len(m.Regulators) > 0 || len(m.Components) > 0
}

func detectScanInputFormat(path string) (string, error) {
	if strings.EqualFold(filepath.Ext(path), ".csv") {
		return "csv", nil
	}
	if strings.EqualFold(filepath.Ext(path), ".net") {
		return "netlist", nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	record, err := reader.Read()
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}
	if len(record) > 1 {
		return "csv", nil
	}

	return "", fmt.Errorf("unsupported input format for %q (currently supported: CSV, .net)", path)
}
