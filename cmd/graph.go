package cmd

import (
	"fmt"
	"os"
	"strings"

	graphir "github.com/badimirzai/architon-cli/internal/graph"
	"github.com/badimirzai/architon-cli/internal/version"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newGraphCmd())
}

func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "graph <path>",
		Args:          cobra.ExactArgs(1),
		Short:         "Emit stable GraphIR JSON for Studio and other tools",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Emit stable machine-readable architecture GraphIR.

The graph command uses the same project detection, import, metadata,
contract-enrichment, voltage-propagation, and deterministic rule pipeline as
rv scan. It does not require Studio and does not call AI.

Examples:
  rv graph . --format json
  rv graph . --format json --out graph.json
  rv graph . --contracts examples/contracts/i2c_policy.yaml --format json --out graph.json
  rv graph exports/example.net --meta .architon/meta.yaml --format json
  rv graph . --bom bom/bom.csv --netlist exports/project.net --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mappingFile, _ := cmd.Flags().GetString("map")
			bomOverride, _ := cmd.Flags().GetString("bom")
			netlistOverride, _ := cmd.Flags().GetString("netlist")
			metaOverride, _ := cmd.Flags().GetString("meta")
			contractsOverride, _ := cmd.Flags().GetString("contracts")
			outputFormat, _ := cmd.Flags().GetString("format")
			outputPath, _ := cmd.Flags().GetString("out")
			noKiCadCLI, _ := cmd.Flags().GetBool("no-kicad-cli")
			kicadCLIPath, _ := cmd.Flags().GetString("kicad-cli")

			outputFormat = strings.ToLower(strings.TrimSpace(outputFormat))
			if outputFormat == "" {
				outputFormat = "json"
			}
			if outputFormat != "json" {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("unsupported output format %q (allowed: json)", outputFormat),
				}
			}

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
				return err
			}

			graph := graphir.Build(graphir.BuildInput{
				RVVersion:  version.Get().Version,
				InputPath:  args[0],
				Design:     pipeline.Design,
				Report:     pipeline.Report,
				ContractIR: pipeline.ContractIR,
			})
			data, err := graphir.RenderJSON(graph)
			if err != nil {
				return internalError(fmt.Errorf("marshal GraphIR JSON: %w", err))
			}
			if strings.TrimSpace(outputPath) != "" {
				if err := os.WriteFile(outputPath, data, 0o644); err != nil {
					return &ExitError{
						Code: 3,
						Err:  fmt.Errorf("write graph JSON %s: %w", outputPath, err),
					}
				}
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().String("map", "", "Path to YAML file with explicit BOM header mapping")
	cmd.Flags().String("bom", "", "Override BOM file path when scanning a project directory")
	cmd.Flags().String("netlist", "", "Override netlist file path when scanning a project directory")
	cmd.Flags().String("meta", "", "Override meta file path (default: .architon/meta.yaml if present)")
	cmd.Flags().String("contracts", "", "Override contracts file path (default: .architon/contracts.yaml if present)")
	cmd.Flags().String("format", "json", "Output format: json")
	cmd.Flags().String("out", "", "Path to write GraphIR JSON")
	cmd.Flags().Bool("no-kicad-cli", false, "Disable automatic KiCad netlist generation for project directories")
	cmd.Flags().String("kicad-cli", defaultKiCadCLI, "KiCad CLI binary name or path for automatic netlist generation")
	return cmd
}
