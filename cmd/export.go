package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	graphir "github.com/badimirzai/architon-cli/internal/graph"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/version"
	"github.com/spf13/cobra"
)

// Studio imports these two names from <project>/.architon/studio/.
// report.json is the scan verification report. graph.json is the GraphIR.
const (
	studioExportDirName    = "studio"
	studioExportReportFile = "report.json"
	studioExportGraphFile  = "graph.json"
)

func init() {
	rootCmd.AddCommand(newExportCmd())
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "export <path>",
		Args:          cobra.ExactArgs(1),
		Short:         "Write the Architon Studio report and GraphIR for a project",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Write the two files Architon Studio imports.

rv export runs the same scan pipeline as rv scan, once, then writes:

  .architon/studio/report.json
  .architon/studio/graph.json

The report is the verification report from rv scan --out. The graph is the
GraphIR from rv graph --format json --out. Exit codes match rv scan. Exit 2
still writes both files. A failure before a report exists writes nothing new.

Examples:
  rv export .
  rv export . --contracts .architon/contracts.yaml
  rv export . --meta .architon/meta.yaml --no-kicad-cli
  rv export . --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mappingFile, _ := cmd.Flags().GetString("map")
			bomOverride, _ := cmd.Flags().GetString("bom")
			netlistOverride, _ := cmd.Flags().GetString("netlist")
			metaOverride, _ := cmd.Flags().GetString("meta")
			contractsOverride, _ := cmd.Flags().GetString("contracts")
			noKiCadCLI, _ := cmd.Flags().GetBool("no-kicad-cli")
			kicadCLIPath, _ := cmd.Flags().GetString("kicad-cli")

			// Same ingestion and rules as rv scan. Export does not reimplement them.
			pipeline, err := runScanPipeline(args[0], scanPipelineOptions{
				MappingFile:       mappingFile,
				BOMOverride:       bomOverride,
				NetlistOverride:   netlistOverride,
				MetaOverride:      metaOverride,
				ContractsOverride: contractsOverride,
				NoKiCadCLI:        noKiCadCLI,
				KiCadCLIPath:      kicadCLIPath,
			})
			if err != nil {
				// No report yet, so leave the project untouched. This is exit 3
				// for a directory with no schematic or netlist.
				return err
			}

			// Build the graph in memory first. A marshal failure then writes nothing.
			graph := graphir.Build(graphir.BuildInput{
				RVVersion:  version.Get().Version,
				InputPath:  args[0],
				Design:     pipeline.Design,
				Report:     pipeline.Report,
				ContractIR: pipeline.ContractIR,
			})
			graphData, err := graphir.RenderJSON(graph)
			if err != nil {
				return internalError(fmt.Errorf("marshal GraphIR JSON: %w", err))
			}

			// Create .architon/studio only after the pipeline produced a report.
			dir := studioExportDir(pipeline.Input)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fatalError(fmt.Errorf("create Studio export directory %s: %w", dir, err))
			}
			reportPath := filepath.Join(dir, studioExportReportFile)
			graphPath := filepath.Join(dir, studioExportGraphFile)
			if err := report.WriteVerificationReport(reportPath, pipeline.Report); err != nil {
				return fatalError(err)
			}
			if err := os.WriteFile(graphPath, graphData, 0o644); err != nil {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("write graph JSON %s: %w", graphPath, err),
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", reportPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", graphPath)
			// 0 clean, 1 warnings, 2 violations, 3 tool failure.
			// Exit 2 still leaves both files on disk.
			return scanReturnExit(scanExitCode(pipeline.Report))
		},
	}

	cmd.Flags().String("map", "", "Path to YAML file with explicit BOM header mapping")
	cmd.Flags().String("bom", "", "Override BOM file path when scanning a project directory")
	cmd.Flags().String("netlist", "", "Override netlist file path when scanning a project directory")
	cmd.Flags().String("meta", "", "Override meta file path (default: .architon/meta.yaml if present)")
	cmd.Flags().String("contracts", "", "Override contracts file path (default: .architon/contracts.yaml if present)")
	cmd.Flags().Bool("no-kicad-cli", false, "Disable automatic KiCad netlist generation for project directories")
	cmd.Flags().String("kicad-cli", defaultKiCadCLI, "KiCad CLI binary name or path for automatic netlist generation")
	return cmd
}

// studioExportDir returns <project>/.architon/studio.
// A project directory uses that directory. A single BOM or netlist file uses
// the folder that contains the file.
func studioExportDir(input resolvedScanInput) string {
	root := input.ProjectPath
	if !input.Directory {
		root = filepath.Dir(input.DirectPath)
	}
	return filepath.Join(root, ".architon", studioExportDirName)
}
