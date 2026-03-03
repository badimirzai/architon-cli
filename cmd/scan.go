package cmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/importers/kicad"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/spf13/cobra"
)

const defaultScanReportPath = "architon-report.json"
const noBOMFoundInProjectDirMessage = "no BOM file found in project directory (expected bom/bom.csv, bom.csv, exports/bom.csv, or *bom*.csv in root/bom/exports/)"

var scanCmd = newScanCmd()

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

Examples:
  rv scan .
  rv scan bom.csv
  rv scan bom.csv --map mapping.yaml
  rv scan bom.csv --out result.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedPath, discovered, err := resolveScanInput(args[0])
			if err != nil {
				return fatalError(err)
			}

			mappingFile, _ := cmd.Flags().GetString("map")
			outputPath, _ := cmd.Flags().GetString("out")

			format, err := detectScanInputFormat(resolvedPath)
			if err != nil {
				return userError(err)
			}

			var mapping kicad.ColumnMapping
			if mappingFile != "" {
				mapping, err = kicad.LoadColumnMapping(mappingFile)
				if err != nil {
					return userError(fmt.Errorf("load mapping: %w", err))
				}
			}

			var designReport report.VerificationReport
			switch format {
			case "csv":
				design, err := kicad.ImportKiCadBOM(resolvedPath, mapping)
				if err != nil {
					return userError(fmt.Errorf("import KiCad BOM: %w", err))
				}
				designReport = report.NewVerificationReport(design)
			default:
				return userError(fmt.Errorf("unsupported input format for %q (currently supported: CSV)", resolvedPath))
			}

			if discovered {
				fmt.Fprintf(cmd.OutOrStdout(), "Detected BOM: %s\n", resolvedPath)
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
	return cmd
}

func resolveScanInput(inputPath string) (resolvedPath string, discovered bool, err error) {
	cleanInput := filepath.Clean(inputPath)

	info, statErr := os.Stat(cleanInput)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return cleanInput, false, nil
		}
		return "", false, fmt.Errorf("stat input path: %w", statErr)
	}
	if !info.IsDir() {
		return cleanInput, false, nil
	}

	absInput, err := filepath.Abs(cleanInput)
	if err != nil {
		return "", true, fmt.Errorf("resolve project directory: %w", err)
	}
	absInput = filepath.Clean(absInput)

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
			return candidate, true, nil
		}
		if !os.IsNotExist(candidateErr) {
			return "", true, fmt.Errorf("stat BOM candidate: %w", candidateErr)
		}
	}

	for _, tierDir := range []string{
		filepath.Join(absInput, "bom"),
		filepath.Join(absInput, "exports"),
		absInput,
	} {
		candidates, err := findBOMCandidates(tierDir)
		if err != nil {
			return "", true, err
		}
		if len(candidates) > 0 {
			return candidates[0], true, nil
		}
	}

	return "", true, errors.New(noBOMFoundInProjectDirMessage)
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

func matchesBOMCSVPattern(name string) bool {
	if !strings.EqualFold(filepath.Ext(name), ".csv") {
		return false
	}

	lowerName := strings.ToLower(name)
	return strings.HasSuffix(lowerName, ".bom.csv") || strings.Contains(lowerName, "bom")
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

	return "", fmt.Errorf("unsupported input format for %q (currently supported: CSV)", path)
}
