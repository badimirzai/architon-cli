package cmd

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/enrichment"
	"github.com/badimirzai/architon-cli/internal/importers"
	"github.com/badimirzai/architon-cli/internal/importers/kicad"
	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
	"github.com/badimirzai/architon-cli/internal/propagate"
	"github.com/badimirzai/architon-cli/internal/rails"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/rules"
	"github.com/badimirzai/architon-cli/internal/ui"
	"github.com/spf13/cobra"
)

const defaultScanReportPath = "architon-report.json"
const defaultKiCadCLI = "kicad-cli"
const generatedKiCadNetlistPath = ".architon/generated.net"
const noScanInputsFoundInProjectDirMessage = "no BOM, netlist, or root KiCad schematic found in project directory (expected bom/bom.csv, bom.csv, exports/bom.csv, or *bom*.csv in root/bom/exports/, plus exports/*.net or *.net in root, or one root *.kicad_sch for kicad-cli netlist export)"

var scanCmd = newScanCmd()

type resolvedScanInput struct {
	DirectPath                  string
	Directory                   bool
	ProjectPath                 string
	BOMPath                     string
	NetlistPath                 string
	GeneratedNetlistDisplayPath string
	SchematicPath               string
	BOMDiscovered               bool
	NetlistDiscovered           bool
	NetlistGenerated            bool
}

type scanInputOptions struct {
	AutoKiCadNetlist bool
	KiCadCLIPath     string
}

// init registers scan with the root CLI.
func init() {
	rootCmd.AddCommand(scanCmd)
}

// newScanCmd builds the rv scan command and wires all scan flags.
func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Scan an electronics BOM and generate a deterministic verification report",
		Long: `Scan an electronics BOM and generate a deterministic verification report.

Current supported input:
  - KiCad BOM CSV
  - KiCad .net S-expression netlist
  - KiCad project directory with a root .kicad_sch schematic

Examples:
  rv scan .
  rv scan bom.csv
  rv scan exports/example.net
  rv scan . --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
  rv scan bom.csv --map mapping.yaml
  rv scan bom.csv --out result.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mappingFile, _ := cmd.Flags().GetString("map")
			outputPath, _ := cmd.Flags().GetString("out")
			bomOverride, _ := cmd.Flags().GetString("bom")
			netlistOverride, _ := cmd.Flags().GetString("netlist")
			metaOverride, _ := cmd.Flags().GetString("meta")
			contractsOverride, _ := cmd.Flags().GetString("contracts")
			outputFormat, _ := cmd.Flags().GetString("format")
			explainRails, _ := cmd.Flags().GetBool("explain-rails")
			railsAlias, _ := cmd.Flags().GetBool("rails")
			noKiCadCLI, _ := cmd.Flags().GetBool("no-kicad-cli")
			kicadCLIPath, _ := cmd.Flags().GetString("kicad-cli")
			explainRails = explainRails || railsAlias
			outputFormat = strings.ToLower(strings.TrimSpace(outputFormat))
			if outputFormat == "" {
				outputFormat = "text"
			}
			if outputFormat != "text" && outputFormat != "json" {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("unsupported output format %q (allowed: text, json)", outputFormat),
				}
			}

			resolvedInput, err := resolveScanInputWithOptions(args[0], bomOverride, netlistOverride, scanInputOptions{
				AutoKiCadNetlist: !noKiCadCLI,
				KiCadCLIPath:     kicadCLIPath,
			})
			if err != nil {
				return fatalError(err)
			}

			design, err := importResolvedScanInput(resolvedInput, mappingFile)
			if err != nil {
				return userError(err)
			}

			// User contracts are loaded before report construction so invalid
			// YAML is a tool error, not a partial scan result.
			userContractsPath, userContracts, err := scanLoadUserContracts(resolvedInput, contractsOverride)
			if err != nil {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("contracts load failed: %w", err),
				}
			}

			designReport := report.NewVerificationReport(design)
			nameInferRes := infer.InferVoltagesFromNetNames(design)

			// -------------------------
			// META: auto-discover existing metadata only. Scan never writes
			// .architon/meta.yaml; rv init owns file creation.
			// -------------------------
			metaPath := strings.TrimSpace(metaOverride)
			metaExplicit := metaPath != ""

			if resolvedInput.Directory {
				defaultMetaPath := filepath.Join(resolvedInput.ProjectPath, ".architon", "meta.yaml")

				if !metaExplicit {
					// Auto-discover existing meta.yaml
					if _, err := os.Stat(defaultMetaPath); err == nil {
						metaPath = defaultMetaPath
					} else if os.IsNotExist(err) {
						metaPath = ""
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

			if metaPath != "" {
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
			initialVoltages := scanInitialVoltages(nameInferRes, metaObj)
			propRes := propagate.Result{NetVoltages: map[string]propagate.NetVoltage{}}
			if designReport.Summary.Nets > 0 && len(initialVoltages) > 0 {
				propRes = propagate.Propagate(*design, *metaObj, initialVoltages)
			}
			// Build the report-facing inference after propagation so explicit
			// metadata and regulator outputs can contribute provenance/confidence.
			railInferRes := infer.InferVoltages(design, infer.VoltageInferenceOptions{
				Evidence:  scanVoltageEvidence(propRes.NetVoltages),
				Conflicts: propRes.Conflicts,
			})

			netVolts := scanReportNetVoltages(propRes.NetVoltages)
			inferredNetVolts := scanReportInferredNetVoltages(nameInferRes)
			unknownVoltageNets := scanReportUnknownVoltageNets(railInferRes)
			railInferences := scanReportRailInferences(railInferRes)
			railCoverage := rails.SummarizeRailCoverage(design, railInferRes.Inferences)
			inferencesByNet := scanInferenceByNet(railInferRes.Inferences)
			if designReport.Summary.Nets > 0 || len(netVolts) > 0 || len(inferredNetVolts) > 0 || len(unknownVoltageNets) > 0 || len(railInferences) > 0 || len(propRes.Conflicts) > 0 {
				designReport.Derived = &report.Derived{
					NetVoltages:         netVolts,
					InferredNetVoltages: inferredNetVolts,
					UnknownVoltageNets:  unknownVoltageNets,
					RailInferences:      railInferences,
					RailCoverage:        railCoverage,
					Conflicts:           propRes.Conflicts,
				}
			}

			if len(propRes.Conflicts) > 0 {
				// Conflicts are violations (severity error => exit 2)
				for _, c := range propRes.Conflicts {
					result := report.RuleResult{
						ID:       "RULE_VOLTAGE_CONFLICT",
						RuleID:   "RULE_VOLTAGE_CONFLICT",
						Severity: "ERROR",
						Message:  c,
						Source:   "voltage-propagation",
						Provenance: &contracts.Provenance{
							Source:   "voltage-propagation",
							SourceID: "RULE_VOLTAGE_CONFLICT",
							Detail:   "deterministic voltage propagation conflict",
						},
						Fix: "Resolve the conflicting voltage evidence for this net.",
					}
					if inference, ok := inferencesByNet[scanConflictNetName(c)]; ok {
						result.Inference = scanReportInferenceProvenance(inference)
					}
					designReport.Rules = append(designReport.Rules, result)
				}
			}

			// Contract enrichment is the boundary between imported design facts
			// and rule-ready electrical intent. Rules below do not read meta.yaml,
			// propagation structs, KiCad parser data, or file paths.
			contractSources := []contracts.ContractSource{}
			if metaLoaded {
				contractSources = append(contractSources, enrichment.NewMetaYAMLSource(metaObj))
			}
			contractSources = append(contractSources,
				contracts.FieldContractSource{},
				contracts.NewBuiltinPartsSource(),
			)
			// Project contracts are appended as another ContractSource. From this
			// point on the evaluator does not care whether a requirement came from
			// built-ins, fields, meta.yaml, or user YAML.
			if len(userContracts) > 0 {
				contractSources = append(contractSources, contracts.NewUserYAMLSource(userContractsPath, userContracts))
			}
			contractSources = append(contractSources,
				enrichment.NewNetVoltageSource("net-voltage-inference", scanContractNetVoltages(propRes.NetVoltages)),
			)
			contractIR, err := (enrichment.ContractEnricher{Sources: contractSources}).Enrich(design)
			if err != nil {
				return internalError(err)
			}
			coverage := contracts.SummarizeCoverage(design, contractIR)
			designReport.ContractCoverage = &coverage
			contractFindings := rules.CheckAll(design, contractIR, rules.DefaultRules())
			designReport.Rules = append(designReport.Rules, scanReportRuleResults(contractFindings, inferencesByNet)...)
			designReport.Rules = append(designReport.Rules, scanReportContractResults(contracts.Evaluate(design, contractIR), inferencesByNet)...)

			if len(designReport.Rules) > 0 {
				// Deterministic rule ordering
				sort.SliceStable(designReport.Rules, func(i, j int) bool {
					if designReport.Rules[i].ID == designReport.Rules[j].ID {
						return designReport.Rules[i].Message < designReport.Rules[j].Message
					}
					return designReport.Rules[i].ID < designReport.Rules[j].ID
				})
			}
			normalizeScanReportRules(designReport.Rules)
			designReport.Findings = append([]report.RuleResult{}, designReport.Rules...)
			// Update summary (because report.NewVerificationReport() computed these before rules existed)
			designReport.Summary.Rules = len(designReport.Findings)
			designReport.Summary.HasFailures = len(design.ParseErrors) > 0 || scanRuleViolationCount(designReport) > 0
			designReport.Summary.PartsMatched = coverage.PartsMatched
			designReport.Summary.UserContractsLoaded = len(userContracts)
			designReport.Summary.BuiltInContractsLoaded = len(contracts.BuiltinContracts())
			designReport.Summary.ActiveUserRequirements = scanActiveUserRequirements(contractIR)
			designReport.Summary.AvailableContractRules = len(coverage.EnabledContractRules)
			designReport.Summary.RequirementsEnabled = designReport.Summary.ActiveUserRequirements
			designReport.Summary.PartContractCoveragePercentage = coverage.CoveragePercentage
			designReport.Summary.ContractsApplied = coverage.ContractsApplied
			designReport.Summary.ContractCoveragePercentage = coverage.CoveragePercentage
			designReport.Summary.UnknownPowerCriticalRefs = coverage.UnknownPowerCriticalRefs
			designReport.Summary.EnabledContractRules = coverage.EnabledContractRules

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
			if outputFormat == "json" {
				jsonReport := report.CanonicalizeVerificationReport(designReport)
				data, err := json.MarshalIndent(jsonReport, "", "  ")
				if err != nil {
					return internalError(fmt.Errorf("marshal scan report JSON: %w", err))
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				switch exitCode {
				case 0:
					return nil
				case 1:
					return &ExitError{Code: 1, Err: errors.New("scan completed with warnings")}
				case 2:
					return &ExitError{Code: 2, Err: errors.New("scan violations detected")}
				case 3:
					return &ExitError{Code: 3, Err: errors.New("scan failed")}
				default:
					return &ExitError{Code: 3, Err: fmt.Errorf("scan failed with unexpected exit code %d", exitCode)}
				}
			}

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
			fmt.Fprintf(cmd.OutOrStdout(), "User contracts loaded: %d\n", designReport.Summary.UserContractsLoaded)
			fmt.Fprintf(cmd.OutOrStdout(), "Built-in contracts loaded: %d\n", designReport.Summary.BuiltInContractsLoaded)
			fmt.Fprintf(cmd.OutOrStdout(), "Active user requirements: %d\n", designReport.Summary.ActiveUserRequirements)
			fmt.Fprintf(cmd.OutOrStdout(), "Available contract rules: %d\n", designReport.Summary.AvailableContractRules)
			fmt.Fprintf(cmd.OutOrStdout(), "Part contract coverage: %.2f%%\n", designReport.Summary.PartContractCoveragePercentage)
			fmt.Fprintf(cmd.OutOrStdout(), "Parts matched: %d/%d\n", designReport.Summary.PartsMatched, designReport.Summary.Parts)
			fmt.Fprintf(cmd.OutOrStdout(), "Unknown power-critical refs: %d\n", len(designReport.Summary.UnknownPowerCriticalRefs))
			fmt.Fprintf(cmd.OutOrStdout(), "Enabled contract rules: %s\n", strings.Join(designReport.Summary.EnabledContractRules, ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "Violations: %d\n", scanRuleViolationCount(designReport))
			fmt.Fprintf(cmd.OutOrStdout(), "Inferred voltages: %d Unknown voltage nets: %d Rail coverage: %s\n", len(nameInferRes.Voltages), len(railInferRes.Unknowns), rails.FormatRailCoverage(railCoverage))
			coveredNets, totalNets := scanInferredVoltageCoverage(design, nameInferRes)
			fmt.Fprintf(cmd.OutOrStdout(), "Inferred rails: %d\n", len(nameInferRes.Voltages))
			fmt.Fprintf(cmd.OutOrStdout(), "Voltage coverage: %d/%d nets with inferred voltage\n", coveredNets, totalNets)
			fmt.Fprintf(cmd.OutOrStdout(), "Metadata: %s\n", scanMetadataMode(metaLoaded, nameInferRes))
			if explainRails {
				scanPrintRailInferences(cmd.OutOrStdout(), railInferences, railCoverage)
			}

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
			if resolvedInput.NetlistGenerated {
				fmt.Fprintf(cmd.OutOrStdout(), "Generated Netlist: %s\n", resolvedInput.GeneratedNetlistDisplayPath)
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
	cmd.Flags().String("meta", "", "Override meta file path (default: .architon/meta.yaml if present)")
	cmd.Flags().String("contracts", "", "Override contracts file path (default: .architon/contracts.yaml if present)")
	cmd.Flags().String("format", "text", "Output format: text or json")
	cmd.Flags().Bool("explain-rails", false, "Print rail voltage inference provenance and confidence")
	cmd.Flags().Bool("rails", false, "Alias for --explain-rails")
	cmd.Flags().Bool("no-kicad-cli", false, "Disable automatic KiCad netlist generation for project directories")
	cmd.Flags().String("kicad-cli", defaultKiCadCLI, "KiCad CLI binary name or path for automatic netlist generation")
	return cmd
}

// importResolvedScanInput keeps CLI discovery separate from importer behavior.
// Today it wires KiCad as the only adapter; future adapters can be added here
// without changing rule packages.
func importResolvedScanInput(input resolvedScanInput, mappingFile string) (*ir.DesignIR, error) {
	mapping, err := loadScanMapping(mappingFile)
	if err != nil {
		return nil, err
	}
	kicadImporter := kicad.NewImporter(mapping)

	if !input.Directory {
		return importDirectScanPath(input.DirectPath, kicadImporter)
	}

	switch {
	case input.BOMPath != "" && input.NetlistPath != "":
		bomDesign, err := kicadImporter.Import(input.BOMPath)
		if err != nil {
			return nil, fmt.Errorf("import KiCad BOM: %w", err)
		}
		netlistDesign, err := kicadImporter.Import(input.NetlistPath)
		if err != nil {
			return nil, fmt.Errorf("import KiCad netlist: %w", err)
		}
		return ir.MergeProjectIR(bomDesign, netlistDesign, input.ProjectPath, time.Now()), nil
	case input.BOMPath != "":
		design, err := kicadImporter.Import(input.BOMPath)
		if err != nil {
			return nil, fmt.Errorf("import KiCad BOM: %w", err)
		}
		return design, nil
	case input.NetlistPath != "":
		design, err := kicadImporter.Import(input.NetlistPath)
		if err != nil {
			return nil, fmt.Errorf("import KiCad netlist: %w", err)
		}
		return design, nil
	default:
		return nil, errors.New(noScanInputsFoundInProjectDirMessage)
	}
}

// importDirectScanPath uses the generic importer interface even for direct
// KiCad files so the single-file path follows the same adapter boundary.
func importDirectScanPath(path string, kicadImporter kicad.Importer) (*ir.DesignIR, error) {
	format, err := detectScanInputFormat(path)
	if err != nil {
		return nil, err
	}

	design, _, importErr := importers.Import(path, []importers.Importer{kicadImporter})
	if importErr != nil {
		return nil, importErr
	}

	switch format {
	case "csv":
		return design, nil
	case "netlist":
		return design, nil
	default:
		return nil, fmt.Errorf("unsupported input format for %q (currently supported: CSV, .net)", path)
	}
}

// loadScanMapping loads an optional BOM column mapping file.
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

// scanLoadUserContracts loads the resolved contracts.yaml, if one is present.
func scanLoadUserContracts(input resolvedScanInput, override string) (string, []contracts.SystemContract, error) {
	path, ok, err := resolveScanContractsPath(input, override)
	if err != nil || !ok {
		return path, nil, err
	}
	loaded, err := contracts.LoadYAMLFile(path)
	if err != nil {
		return path, nil, err
	}
	return path, loaded, nil
}

// resolveScanContractsPath finds the explicit or default project contracts file.
func resolveScanContractsPath(input resolvedScanInput, override string) (string, bool, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		// Explicit --contracts must exist and be valid; otherwise CI should fail.
		path := filepath.Clean(override)
		info, err := os.Stat(path)
		if err != nil {
			return path, false, err
		}
		if info.IsDir() {
			return path, false, fmt.Errorf("%s is a directory", path)
		}
		return path, true, nil
	}

	candidate := filepath.Join(".architon", "contracts.yaml")
	if input.Directory && strings.TrimSpace(input.ProjectPath) != "" {
		candidate = filepath.Join(input.ProjectPath, ".architon", "contracts.yaml")
	}
	info, err := os.Stat(candidate)
	if err == nil {
		if info.IsDir() {
			return candidate, false, fmt.Errorf("%s is a directory", candidate)
		}
		return candidate, true, nil
	}
	if os.IsNotExist(err) {
		// Missing default file is normal. Projects opt in by creating it.
		return "", false, nil
	}
	return candidate, false, err
}

// resolveScanInput resolves scan input with default discovery behavior.
func resolveScanInput(inputPath string, bomOverride string, netlistOverride string) (resolvedScanInput, error) {
	return resolveScanInputWithOptions(inputPath, bomOverride, netlistOverride, scanInputOptions{})
}

// resolveScanInputWithOptions expands a file or project directory into concrete inputs.
func resolveScanInputWithOptions(inputPath string, bomOverride string, netlistOverride string, opts scanInputOptions) (resolvedScanInput, error) {
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

	if netlistOverride == "" && resolved.NetlistPath == "" {
		schematicPath, err := resolveRootSchematicPath(absInput)
		if err != nil {
			return resolvedScanInput{}, err
		}
		resolved.SchematicPath = schematicPath
		if schematicPath != "" && opts.AutoKiCadNetlist {
			generatedNetlist, err := generateKiCadNetlistFromSchematic(opts.KiCadCLIPath, absInput, schematicPath)
			if err != nil {
				return resolvedScanInput{}, err
			}
			resolved.NetlistPath = generatedNetlist
			resolved.GeneratedNetlistDisplayPath = generatedKiCadNetlistPath
			resolved.NetlistGenerated = true
		}
		if schematicPath != "" && !opts.AutoKiCadNetlist && resolved.BOMPath == "" {
			return resolvedScanInput{}, errors.New("root KiCad schematic found but no netlist is available; remove --no-kicad-cli or provide --netlist")
		}
	}

	if resolved.BOMPath == "" && resolved.NetlistPath == "" {
		return resolvedScanInput{}, errors.New(noScanInputsFoundInProjectDirMessage)
	}

	return resolved, nil
}

// resolveDetectedBOMPath finds the preferred BOM CSV in a project directory.
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

// findBOMCandidates lists BOM-like CSV files in one directory.
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

// resolveDetectedNetlistPath finds the preferred KiCad netlist in a project directory.
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

// findNetlistCandidates lists .net files in one directory.
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

// resolveRootSchematicPath finds the single root schematic eligible for export.
func resolveRootSchematicPath(projectPath string) (string, error) {
	candidates, err := findRootSchematicCandidates(projectPath)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}
	if len(candidates) > 1 {
		rel := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			name, err := filepath.Rel(projectPath, candidate)
			if err != nil {
				name = candidate
			}
			rel = append(rel, filepath.ToSlash(name))
		}
		sort.Strings(rel)
		return "", fmt.Errorf("multiple root KiCad schematics found; use --netlist or keep one root *.kicad_sch: %s", strings.Join(rel, ", "))
	}
	return candidates[0], nil
}

// findRootSchematicCandidates lists root-level KiCad schematic files.
func findRootSchematicCandidates(projectPath string) ([]string, error) {
	absDir, err := filepath.Abs(filepath.Clean(projectPath))
	if err != nil {
		return nil, fmt.Errorf("resolve schematic candidate directory: %w", err)
	}

	entries, readErr := os.ReadDir(absDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("read schematic candidate directory: %w", readErr)
	}

	matches := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !matchesKiCadSchematicPattern(entry.Name()) {
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

// generateKiCadNetlistFromSchematic exports a schematic into the project's generated netlist.
func generateKiCadNetlistFromSchematic(kicadCLIPath string, projectPath string, schematicPath string) (string, error) {
	outputPath := filepath.Join(projectPath, generatedKiCadNetlistPath)
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("generate KiCad netlist %s: create output directory: %w", generatedKiCadNetlistPath, err)
	}

	if err := runKiCadNetlistExport(kicadCLIPath, schematicPath, outputPath); err != nil {
		return "", fmt.Errorf("generate KiCad netlist %s: %w", generatedKiCadNetlistPath, err)
	}
	return outputPath, nil
}

// runKiCadNetlistExport executes kicad-cli for netlist export.
func runKiCadNetlistExport(kicadCLIPath string, schematicPath string, outputPath string) error {
	binary, args, err := buildKiCadNetlistCommand(kicadCLIPath, outputPath, schematicPath)
	if err != nil {
		return err
	}
	resolvedBinary, err := resolveKiCadCLIPath(binary)
	if err != nil {
		return err
	}
	cmd := exec.Command(resolvedBinary, args...)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("run KiCad netlist export %q %s: %w: %s", resolvedBinary, strings.Join(args, " "), runErr, message)
		}
		return fmt.Errorf("run KiCad netlist export %q %s: %w", resolvedBinary, strings.Join(args, " "), runErr)
	}
	return nil
}

// resolveKiCadCLIPath locates kicad-cli from the flag or common paths.
func resolveKiCadCLIPath(binary string) (string, error) {
	return resolveKiCadCLIPathWithLookPath(binary, commonKiCadCLIPaths(), exec.LookPath)
}

// resolveKiCadCLIPathWithLookPath is the testable kicad-cli resolver core.
func resolveKiCadCLIPathWithLookPath(binary string, commonPaths []string, lookPath func(string) (string, error)) (string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = defaultKiCadCLI
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if resolved, err := lookPath(binary); err == nil {
		return resolved, nil
	}
	if binary != defaultKiCadCLI {
		return "", fmt.Errorf("KiCad CLI %q not found; pass --kicad-cli with the full path to kicad-cli", binary)
	}
	for _, candidate := range commonPaths {
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("KiCad CLI %q not found in PATH or common install locations; install KiCad, add kicad-cli to PATH, or pass --kicad-cli /full/path/to/kicad-cli", binary)
}

// commonKiCadCLIPaths returns OS-specific fallback kicad-cli locations.
func commonKiCadCLIPaths() []string {
	candidates := make([]string, 0, 16)
	patterns := make([]string, 0, 16)

	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli",
		)
		patterns = append(patterns,
			"/Applications/KiCad*/KiCad.app/Contents/MacOS/kicad-cli",
			"/Applications/KiCad/*.app/Contents/MacOS/kicad-cli",
		)
	case "windows":
		for _, root := range []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LocalAppData"),
		} {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(root, "KiCad", "bin", "kicad-cli.exe"),
			)
			patterns = append(patterns,
				filepath.Join(root, "KiCad", "*", "bin", "kicad-cli.exe"),
				filepath.Join(root, "KiCad", "*", "kicad-cli.exe"),
			)
		}
	default:
		candidates = append(candidates,
			"/usr/bin/kicad-cli",
			"/usr/local/bin/kicad-cli",
			"/opt/kicad/bin/kicad-cli",
			"/snap/bin/kicad-cli",
			"/app/bin/kicad-cli",
		)
		patterns = append(patterns,
			"/opt/kicad*/bin/kicad-cli",
		)
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err == nil {
			candidates = append(candidates, matches...)
		}
	}
	sort.Strings(candidates)
	out := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

// isExecutableFile checks whether a path is a runnable file.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// buildKiCadNetlistCommand creates the kicad-cli argv for export.
func buildKiCadNetlistCommand(kicadCLIPath string, outputPath string, schematicPath string) (string, []string, error) {
	binary := strings.TrimSpace(kicadCLIPath)
	if binary == "" {
		binary = defaultKiCadCLI
	}
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return "", nil, errors.New("KiCad netlist output path is empty")
	}
	schematicPath = strings.TrimSpace(schematicPath)
	if schematicPath == "" {
		return "", nil, errors.New("KiCad schematic path is empty")
	}
	return binary, []string{
		"sch",
		"export",
		"netlist",
		"--format",
		"kicadsexpr",
		"--output",
		outputPath,
		schematicPath,
	}, nil
}

// matchesBOMCSVPattern reports whether a filename looks like a BOM CSV.
func matchesBOMCSVPattern(name string) bool {
	if !strings.EqualFold(filepath.Ext(name), ".csv") {
		return false
	}

	lowerName := strings.ToLower(name)
	return strings.HasSuffix(lowerName, ".bom.csv") || strings.Contains(lowerName, "bom")
}

// matchesNetlistPattern reports whether a filename is a KiCad netlist.
func matchesNetlistPattern(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".net")
}

// matchesKiCadSchematicPattern reports whether a filename is a KiCad schematic.
func matchesKiCadSchematicPattern(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".kicad_sch")
}

// Exit codes (consistent with rv check contract):
// 0 = clean/info only
// 1 = warnings detected
// 2 = violations detected (severity=error)
// 3 = tool execution failure (analysis could not complete)
// scanResultLabel turns an exit code into the compact result label.
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

// scanResultExplanation turns an exit code into one human-readable sentence.
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

// scanExitColorToken maps exit codes to terminal color classes.
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

// scanRuleColorToken maps severities to terminal color classes.
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

func scanActiveUserRequirements(contractIR *contracts.ContractIR) int {
	if contractIR == nil {
		return 0
	}
	count := 0
	for _, req := range contractIR.AppliedRequirements {
		if req.ContractSource == contracts.ContractSourceUserYAML || req.Source == "user_yaml" {
			count++
		}
	}
	return count
}

// scanExitCode derives the final scan exit code from report findings.
func scanExitCode(result report.VerificationReport) int {
	// Parse errors mean analysis could not complete reliably.
	if result.Summary.ParseErrorsCount > 0 {
		return 3
	}

	hasWarn := false
	hasErr := false
	for _, rule := range result.Rules {
		sev := normalizeSeverity(rule.Severity)
		if sev == "ERROR" {
			hasErr = true
		} else if sev == "WARN" {
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

// scanRuleViolationCount counts ERROR scan findings.
func scanRuleViolationCount(result report.VerificationReport) int {
	n := 0
	for _, rule := range result.Rules {
		sev := normalizeSeverity(rule.Severity)
		if sev == "ERROR" {
			n++
		}
	}
	return n
}

// scanRuleWarningCount counts WARN findings plus parse warnings.
func scanRuleWarningCount(result report.VerificationReport) int {
	n := 0
	for _, rule := range result.Rules {
		sev := normalizeSeverity(rule.Severity)
		if sev == "WARN" {
			n++
		}
	}
	// include parse warnings as warnings too
	n += result.Summary.ParseWarningsCount
	return n
}

// scanDetectURefs returns a small sorted sample of U*/IC-like component refs.
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

// scanInitialVoltages combines inferred and explicit source voltages.
func scanInitialVoltages(inferRes infer.Result, m *meta.Meta) map[string]float64 {
	initial := make(map[string]float64, len(inferRes.Voltages))
	for net, inferred := range inferRes.Voltages {
		if infer.IsGroundNetName(net) {
			continue
		}
		initial[net] = inferred.Voltage
	}
	if m != nil {
		for _, source := range m.Sources {
			initial[source.Net] = source.Voltage
		}
	}
	return initial
}

// scanInferredVoltageCoverage counts nets with net-name voltage inference.
func scanInferredVoltageCoverage(design *ir.DesignIR, inferRes infer.Result) (int, int) {
	if design == nil {
		return 0, 0
	}
	total := len(design.Nets)
	covered := 0
	for _, net := range design.Nets {
		if _, ok := inferRes.Voltages[strings.TrimSpace(net.Name)]; ok {
			covered++
		}
	}
	return covered, total
}

// scanMetadataMode labels whether scan used explicit, inferred, mixed, or no metadata.
func scanMetadataMode(metaLoaded bool, inferRes infer.Result) string {
	hasInferred := len(inferRes.Voltages) > 0
	switch {
	case metaLoaded && hasInferred:
		return "mixed"
	case metaLoaded:
		return "explicit"
	case hasInferred:
		return "inferred"
	default:
		return "none"
	}
}

// scanReportNetVoltages converts propagated voltages into report JSON rows.
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

// scanContractNetVoltages adapts propagation output into the enrichment package
// without letting contract sources depend on propagation internals.
func scanContractNetVoltages(netVoltages map[string]propagate.NetVoltage) []enrichment.NetVoltage {
	out := make([]enrichment.NetVoltage, 0, len(netVoltages))
	for _, nv := range netVoltages {
		out = append(out, enrichment.NetVoltage{
			Net:     nv.Net,
			Voltage: nv.Voltage,
			Source:  nv.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Net != out[j].Net {
			return out[i].Net < out[j].Net
		}
		return out[i].Source < out[j].Source
	})
	return out
}

// scanReportRuleResults adapts rule-engine findings into the stable report
// schema and attaches voltage inference provenance when available.
func scanReportRuleResults(findings []rules.Finding, inferencesByNet map[string]infer.VoltageInference) []report.RuleResult {
	out := make([]report.RuleResult, 0, len(findings))
	for _, finding := range findings {
		result := report.RuleResult{
			ID:             finding.RuleID,
			RuleID:         finding.RuleID,
			Severity:       normalizeSeverity(finding.Severity),
			Net:            finding.Net,
			Message:        finding.Message,
			Provider:       finding.Provider,
			Consumer:       finding.Consumer,
			Ref:            finding.Ref,
			ComponentRef:   finding.Ref,
			Pin:            finding.Pin,
			Source:         "contract-rules",
			ContractSource: string(contracts.ContractSourceInferred),
			Requirement:    finding.RuleID,
			Provenance: &contracts.Provenance{
				Source:   "contract-rules",
				SourceID: finding.RuleID,
				Detail:   "deterministic ContractIR rule",
			},
			Fix: fixForRule(finding.RuleID),
		}
		if inference, ok := inferencesByNet[finding.Net]; ok {
			result.Inference = scanReportInferenceProvenance(inference)
		}
		out = append(out, result)
	}
	return out
}

// scanReportContractResults adapts built-in/custom system contract findings
// into the same report schema as the generic scan rules.
func scanReportContractResults(findings []contracts.Finding, inferencesByNet map[string]infer.VoltageInference) []report.RuleResult {
	out := make([]report.RuleResult, 0, len(findings))
	for _, finding := range findings {
		result := report.RuleResult{
			ID:                  finding.RuleID,
			RuleID:              finding.RuleID,
			Severity:            normalizeSeverity(finding.Severity),
			Net:                 finding.Net,
			Message:             finding.Message,
			Ref:                 finding.ComponentRef,
			ComponentRef:        finding.ComponentRef,
			Pin:                 finding.Pin,
			BusID:               finding.BusID,
			BusType:             finding.BusType,
			BusNets:             finding.BusNets,
			EffectivePullupOhms: finding.EffectivePullupOhms,
			MinPullupOhms:       finding.MinPullupOhms,
			MaxPullupOhms:       finding.MaxPullupOhms,
			PullupResistors:     append([]string(nil), finding.PullupResistors...),
			Source:              finding.Source,
			ContractID:          finding.ContractID,
			ContractSource:      string(finding.ContractSource),
			ContractFile:        finding.ContractFile,
			Requirement:         finding.Requirement,
			Fix:                 finding.Fix,
		}
		if finding.Provenance.Source != "" {
			prov := finding.Provenance
			result.Provenance = &prov
		}
		if inference, ok := inferencesByNet[finding.Net]; ok {
			result.Inference = scanReportInferenceProvenance(inference)
		}
		out = append(out, result)
	}
	return out
}

// normalizeScanReportRules fills stable defaults on all report findings.
func normalizeScanReportRules(rules []report.RuleResult) {
	for i := range rules {
		rules[i].Severity = normalizeSeverity(rules[i].Severity)
		if strings.TrimSpace(rules[i].Source) == "" {
			rules[i].Source = "scan-rule"
		}
		if strings.TrimSpace(rules[i].ContractSource) == "" {
			rules[i].ContractSource = string(contracts.ReportContractSource(rules[i].Source))
		}
		if strings.TrimSpace(rules[i].Requirement) == "" {
			rules[i].Requirement = rules[i].RuleID
		}
		if rules[i].Provenance == nil {
			rules[i].Provenance = &contracts.Provenance{
				Source:   rules[i].Source,
				SourceID: rules[i].RuleID,
				Detail:   "deterministic scan finding",
			}
		}
		if strings.TrimSpace(rules[i].Provenance.Source) == "" {
			rules[i].Provenance.Source = rules[i].Source
		}
		if strings.TrimSpace(rules[i].Provenance.SourceID) == "" {
			rules[i].Provenance.SourceID = rules[i].RuleID
		}
		if strings.TrimSpace(rules[i].Fix) == "" {
			rules[i].Fix = fixForRule(rules[i].RuleID)
		}
	}
}

// normalizeSeverity canonicalizes severity strings for reports and exit logic.
func normalizeSeverity(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "", "ERROR":
		return "ERROR"
	case "WARN", "WARNING":
		return "WARN"
	case "INFO":
		return "INFO"
	default:
		return "ERROR"
	}
}

// fixForRule returns the default remediation hint for a rule.
func fixForRule(ruleID string) string {
	switch ruleID {
	case rules.RuleSupplyContract:
		return "Move the consumer to a compatible supply rail or update the provider/consumer contract."
	case rules.RuleLogicLevelContract:
		return "Add level shifting or drive the signal at a compatible voltage."
	case rules.RuleBusRoleContract:
		return "Separate incompatible bus roles or correct the pin role contracts."
	case "RULE_VOLTAGE_CONFLICT":
		return "Resolve the conflicting voltage evidence for this net."
	default:
		return "Review the schematic connection and contract data for this finding."
	}
}

// scanVoltageEvidence converts propagated net voltages into inference evidence.
// Only user-supplied sources and regulator outputs are promoted here because
// plain "initial" entries already came from net-name inference.
func scanVoltageEvidence(netVoltages map[string]propagate.NetVoltage) []infer.VoltageEvidence {
	out := make([]infer.VoltageEvidence, 0, len(netVoltages))
	for _, nv := range netVoltages {
		source := strings.TrimSpace(nv.Source)
		switch {
		case source == "source":
			out = append(out, infer.VoltageEvidence{
				NetName:   nv.Net,
				Voltage:   nv.Voltage,
				Source:    infer.SourceUserOverride,
				BaseScore: 1.00,
				Evidence:  fmt.Sprintf("user override set rail %q to %.2fV", nv.Net, nv.Voltage),
			})
		case strings.HasPrefix(source, "regulator:"):
			ref := strings.TrimSpace(strings.TrimPrefix(source, "regulator:"))
			if ref == "" {
				ref = "unknown"
			}
			out = append(out, infer.VoltageEvidence{
				NetName:   nv.Net,
				Voltage:   nv.Voltage,
				Source:    infer.SourceRegulatorOutput,
				BaseScore: 0.90,
				Evidence:  fmt.Sprintf("regulator %s output mapped to rail %q at %.2fV", ref, nv.Net, nv.Voltage),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NetName != out[j].NetName {
			return out[i].NetName < out[j].NetName
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Voltage < out[j].Voltage
	})
	return out
}

// scanReportInferredNetVoltages adapts net-name inference into report rows.
func scanReportInferredNetVoltages(inferRes infer.Result) []report.NetVoltage {
	out := make([]report.NetVoltage, 0, len(inferRes.Voltages))
	for _, inferred := range inferRes.Voltages {
		out = append(out, report.NetVoltage{
			Net:        inferred.Net,
			Voltage:    inferred.Voltage,
			Source:     inferred.Source,
			Confidence: inferred.Confidence,
			Reason:     inferred.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Net < out[j].Net })
	return out
}

// scanReportUnknownVoltageNets adapts unknown rail data into report rows.
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

// scanReportRailInferences keeps the JSON stable by reporting rails with actual
// voltage evidence and named unknown rail-like nets, while omitting arbitrary
// non-power signals that only produced UNKNOWN/no-evidence records.
func scanReportRailInferences(inferRes infer.Result) []infer.VoltageInference {
	unknownNets := map[string]struct{}{}
	for _, unknown := range inferRes.Unknowns {
		unknownNets[unknown.Net] = struct{}{}
	}

	out := make([]infer.VoltageInference, 0, len(inferRes.Inferences))
	for _, inference := range inferRes.Inferences {
		if inference.Voltage == nil && len(inference.Evidence) == 0 {
			if _, ok := unknownNets[inference.NetName]; !ok {
				continue
			}
		}
		out = append(out, inference)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NetName != out[j].NetName {
			return out[i].NetName < out[j].NetName
		}
		return out[i].Source < out[j].Source
	})
	return out
}

// scanInferenceByNet indexes rail inference provenance by net name.
func scanInferenceByNet(inferences []infer.VoltageInference) map[string]infer.VoltageInference {
	out := make(map[string]infer.VoltageInference, len(inferences))
	for _, inference := range inferences {
		netName := strings.TrimSpace(inference.NetName)
		if netName == "" {
			continue
		}
		out[netName] = inference
	}
	return out
}

// scanReportInferenceProvenance trims rail inference to finding provenance fields.
func scanReportInferenceProvenance(inference infer.VoltageInference) *report.InferenceProvenance {
	return &report.InferenceProvenance{
		NetName:         inference.NetName,
		Source:          inference.Source,
		ConfidenceScore: inference.ConfidenceScore,
		ConfidenceLevel: inference.ConfidenceLevel,
		Reason:          inference.Reason,
	}
}

// scanConflictNetName extracts the net name from a propagation conflict message.
func scanConflictNetName(message string) string {
	const prefix = "Voltage conflict on net "
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(message, prefix)
	if idx := strings.Index(rest, ":"); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

// scanPrintRailInferences renders the same stable inference data as JSON in a
// compact human form for rv scan --explain-rails.
func scanPrintRailInferences(out io.Writer, inferences []infer.VoltageInference, summary rails.RailCoverageSummary) {
	fmt.Fprint(out, rails.FormatRailExplanations(inferences, summary))
}

// scanPrepareAutoMeta removes placeholder metadata entries before validation.
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

// scanMetaHasEntries reports whether auto-discovered meta has usable data.
func scanMetaHasEntries(m *meta.Meta) bool {
	if m == nil {
		return false
	}
	return len(m.Sources) > 0 || len(m.Regulators) > 0 || len(m.Components) > 0
}

// detectScanInputFormat classifies a direct scan input path.
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
