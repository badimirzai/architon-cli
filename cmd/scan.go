package cmd

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/badimirzai/robotics-verifier-cli/internal/importers/kicad"
	"github.com/badimirzai/robotics-verifier-cli/internal/report"
	"github.com/spf13/cobra"
)

const defaultScanReportPath = "architon-report.json"

var scanCmd = newScanCmd()

func init() {
	rootCmd.AddCommand(scanCmd)
}

func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan <input>",
		Args:  cobra.ExactArgs(1),
		Short: "Scan an electronics BOM and generate a deterministic verification report",
		Long: `Scan an electronics BOM and generate a deterministic verification report.

Current supported input:
  - KiCad BOM CSV

Examples:
  rv scan bom.csv
  rv scan bom.csv --map mapping.yaml
  rv scan bom.csv --out result.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputPath := args[0]
			mappingFile, _ := cmd.Flags().GetString("map")
			outputPath, _ := cmd.Flags().GetString("out")

			format, err := detectScanInputFormat(inputPath)
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
				design, err := kicad.ImportKiCadBOM(inputPath, mapping)
				if err != nil {
					return userError(fmt.Errorf("import KiCad BOM: %w", err))
				}
				designReport = report.NewVerificationReport(design)
			default:
				return userError(fmt.Errorf("unsupported input format for %q (currently supported: CSV)", inputPath))
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
