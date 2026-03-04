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
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/report"
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

			resolvedInput, err := resolveScanInput(args[0], bomOverride, netlistOverride)
			if err != nil {
				return fatalError(err)
			}

			design, err := importResolvedScanInput(resolvedInput, mappingFile)
			if err != nil {
				return userError(err)
			}
			designReport := report.NewVerificationReport(design)

			if resolvedInput.BOMDiscovered {
				fmt.Fprintf(cmd.OutOrStdout(), "Detected BOM: %s\n", resolvedInput.BOMPath)
			}
			if resolvedInput.NetlistDiscovered {
				fmt.Fprintf(cmd.OutOrStdout(), "Detected Netlist: %s\n", resolvedInput.NetlistPath)
			}

			if err := report.WriteVerificationReport(outputPath, designReport); err != nil {
				return internalError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", outputPath)

			exitCode := scanExitCode(designReport)
			if exitCode == 0 {
				return nil
			}
			if exitCode == 2 {
				return &ExitError{
					Code: 2,
					Err:  fmt.Errorf("scan completed with %d parse errors; wrote %s", designReport.Summary.ParseErrorsCount, outputPath),
				}
			}
			return &ExitError{
				Code: 1,
				Err:  fmt.Errorf("scan completed with %d rule violations; wrote %s", scanRuleFailureCount(designReport), outputPath),
			}
		},
	}
	cmd.Flags().String("map", "", "Path to YAML file with explicit BOM header mapping")
	cmd.Flags().String("out", defaultScanReportPath, "Path to write the scan report JSON")
	cmd.Flags().String("bom", "", "Override BOM file path when scanning a project directory")
	cmd.Flags().String("netlist", "", "Override netlist file path when scanning a project directory")
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

// Exit codes are part of the rv scan contract:
// 0 = report written with no parse errors and no rule failures
// 1 = report written with one or more rule failures
// 2 = report written with one or more parse errors
func scanExitCode(result report.VerificationReport) int {
	if result.Summary.ParseErrorsCount > 0 {
		return 2
	}
	if scanRuleFailureCount(result) > 0 {
		return 1
	}
	return 0
}

func scanRuleFailureCount(result report.VerificationReport) int {
	count := 0
	for _, rule := range result.Rules {
		severity := strings.TrimSpace(rule.Severity)
		if severity == "" || strings.EqualFold(severity, "error") {
			count++
		}
	}
	return count
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
