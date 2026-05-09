package cmd

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	graphir "github.com/badimirzai/architon-cli/internal/graph"
	"github.com/badimirzai/architon-cli/internal/report"
	"github.com/badimirzai/architon-cli/internal/version"
	"github.com/spf13/cobra"
)

const (
	defaultHTMLReportPath = "architon-report.html"
	htmlReportVersion     = "1"
)

func init() {
	rootCmd.AddCommand(newReportCmd())
}

func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "report <path>",
		Args:          cobra.ExactArgs(1),
		Short:         "Generate a static offline HTML report from scan and GraphIR data",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Generate a professional local HTML report from Architon scan and GraphIR data.

The report command runs the deterministic scan pipeline and GraphIR generation
internally, embeds both JSON payloads, and writes a static offline HTML artifact.

Examples:
  rv report . --format html --out architon-report.html
  rv report exports/project.net --meta .architon/meta.yaml --format html --out report.html
  rv report . --contracts .architon/contracts.yaml --format html --out report.html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mappingFile, _ := cmd.Flags().GetString("map")
			bomOverride, _ := cmd.Flags().GetString("bom")
			netlistOverride, _ := cmd.Flags().GetString("netlist")
			metaOverride, _ := cmd.Flags().GetString("meta")
			contractsOverride, _ := cmd.Flags().GetString("contracts")
			outputFormat, _ := cmd.Flags().GetString("format")
			outputPath, _ := cmd.Flags().GetString("out")
			scanOutputPath, _ := cmd.Flags().GetString("scan-out")
			graphOutputPath, _ := cmd.Flags().GetString("graph-out")
			noKiCadCLI, _ := cmd.Flags().GetBool("no-kicad-cli")
			kicadCLIPath, _ := cmd.Flags().GetString("kicad-cli")

			outputFormat = strings.ToLower(strings.TrimSpace(outputFormat))
			if outputFormat == "" {
				outputFormat = "html"
			}
			if outputFormat != "html" {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("unsupported output format %q (allowed: html)", outputFormat),
				}
			}
			if strings.TrimSpace(outputPath) == "" {
				outputPath = defaultHTMLReportPath
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

			scanResult := report.CanonicalizeVerificationReport(pipeline.Report)
			scanJSON, err := json.MarshalIndent(scanResult, "", "  ")
			if err != nil {
				return internalError(fmt.Errorf("marshal embedded scan JSON: %w", err))
			}
			graph := graphir.Build(graphir.BuildInput{
				RVVersion:  version.Get().Version,
				InputPath:  args[0],
				Design:     pipeline.Design,
				Report:     scanResult,
				ContractIR: pipeline.ContractIR,
			})
			graphJSON, err := graphir.RenderJSON(graph)
			if err != nil {
				return internalError(fmt.Errorf("marshal embedded GraphIR JSON: %w", err))
			}

			html, err := renderHTMLReport(htmlReportInput{
				InputPath:         args[0],
				Scan:              scanResult,
				Graph:             graph,
				ContractIR:        pipeline.ContractIR,
				UserContracts:     pipeline.UserContracts,
				EmbeddedScanJSON:  scanJSON,
				EmbeddedGraphJSON: graphJSON,
			})
			if err != nil {
				return internalError(fmt.Errorf("render HTML report: %w", err))
			}
			if err := os.WriteFile(outputPath, html, 0o644); err != nil {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("write HTML report %s: %w", outputPath, err),
				}
			}
			if strings.TrimSpace(scanOutputPath) != "" {
				if err := os.WriteFile(scanOutputPath, scanJSON, 0o644); err != nil {
					return &ExitError{
						Code: 3,
						Err:  fmt.Errorf("write embedded scan JSON %s: %w", scanOutputPath, err),
					}
				}
			}
			if strings.TrimSpace(graphOutputPath) != "" {
				if err := os.WriteFile(graphOutputPath, graphJSON, 0o644); err != nil {
					return &ExitError{
						Code: 3,
						Err:  fmt.Errorf("write embedded GraphIR JSON %s: %w", graphOutputPath, err),
					}
				}
			}

			exitCode := scanExitCode(scanResult)
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", outputPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Embedded scan findings: %d\n", len(scanResult.Findings))
			fmt.Fprintf(cmd.OutOrStdout(), "Embedded graph findings: %d\n", graph.Summary.Findings)
			fmt.Fprintf(cmd.OutOrStdout(), "User contracts loaded: %d\n", scanResult.Summary.UserContractsLoaded)
			fmt.Fprintf(cmd.OutOrStdout(), "exit code: %d\n", exitCode)
			return scanReturnExit(exitCode)
		},
	}

	cmd.Flags().String("map", "", "Path to YAML file with explicit BOM header mapping")
	cmd.Flags().String("bom", "", "Override BOM file path when scanning a project directory")
	cmd.Flags().String("netlist", "", "Override netlist file path when scanning a project directory")
	cmd.Flags().String("meta", "", "Override meta file path (default: .architon/meta.yaml if present)")
	cmd.Flags().String("contracts", "", "Override contracts file path (default: .architon/contracts.yaml if present)")
	cmd.Flags().String("format", "html", "Output format: html")
	cmd.Flags().String("out", defaultHTMLReportPath, "Path to write the offline HTML report")
	cmd.Flags().String("scan-out", "", "Optional path to write the exact embedded scan JSON")
	cmd.Flags().String("graph-out", "", "Optional path to write the exact embedded GraphIR JSON")
	cmd.Flags().Bool("no-kicad-cli", false, "Disable automatic KiCad netlist generation for project directories")
	cmd.Flags().String("kicad-cli", defaultKiCadCLI, "KiCad CLI binary name or path for automatic netlist generation")
	return cmd
}

type htmlReportInput struct {
	InputPath         string
	Scan              report.VerificationReport
	Graph             graphir.GraphIR
	ContractIR        *contracts.ContractIR
	UserContracts     []contracts.SystemContract
	EmbeddedScanJSON  []byte
	EmbeddedGraphJSON []byte
}

type htmlReportView struct {
	Title             string
	InputPath         string
	Status            string
	StatusClass       string
	RVVersion         string
	ReportVersion     string
	Summary           htmlSummary
	Findings          []htmlFinding
	Contracts         []htmlContract
	Components        []htmlComponent
	Rails             []htmlRail
	EmbeddedScanJSON  template.JS
	EmbeddedGraphJSON template.JS
}

type htmlSummary struct {
	Violations             int
	Warnings               int
	ContractsLoaded        int
	UserContractsLoaded    int
	BuiltInContractsLoaded int
	ContractCoverage       string
	RailCoverage           string
}

type htmlFinding struct {
	Severity       string
	Class          string
	ContractID     string
	Source         string
	Component      string
	Net            string
	Message        string
	WhyThisMatters string
	Fix            string
}

type htmlContract struct {
	ID        string
	Source    string
	Severity  string
	Component string
	Type      string
}

type htmlComponent struct {
	Ref              string
	Value            string
	Type             string
	ContractCoverage string
	FindingsCount    int
}

type htmlRail struct {
	Name          string
	Voltage       string
	Source        string
	Consumers     string
	FindingsCount int
}

func renderHTMLReport(input htmlReportInput) ([]byte, error) {
	view, err := buildHTMLReportView(input)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	tmpl, err := template.New("offline_html_report").Parse(htmlReportTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, view); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func buildHTMLReportView(input htmlReportInput) (htmlReportView, error) {
	scanResult := report.CanonicalizeVerificationReport(input.Scan)
	scanJSON := input.EmbeddedScanJSON
	if len(scanJSON) == 0 {
		var err error
		scanJSON, err = json.MarshalIndent(scanResult, "", "  ")
		if err != nil {
			return htmlReportView{}, fmt.Errorf("marshal embedded scan JSON: %w", err)
		}
	}
	graphJSON := input.EmbeddedGraphJSON
	if len(graphJSON) == 0 {
		var err error
		graphJSON, err = graphir.RenderJSON(input.Graph)
		if err != nil {
			return htmlReportView{}, fmt.Errorf("marshal embedded GraphIR JSON: %w", err)
		}
	}

	violations, findingWarnings, _ := scanFindingSeverityCounts(scanResult.Findings)
	warnings := findingWarnings + scanResult.Summary.ParseWarningsCount
	exitCode := scanExitCode(scanResult)
	status, statusClass := htmlStatus(exitCode)
	inputPath := strings.TrimSpace(input.InputPath)
	if inputPath == "" {
		inputPath = scanResult.Summary.InputFile
	}
	displayPath := htmlReportDisplayInputPath(inputPath)

	return htmlReportView{
		Title:         "Architon Offline HTML Report",
		InputPath:     displayPath,
		Status:        status,
		StatusClass:   statusClass,
		RVVersion:     version.Get().Version,
		ReportVersion: htmlReportVersion,
		Summary: htmlSummary{
			Violations:             violations,
			Warnings:               warnings,
			ContractsLoaded:        scanResult.Summary.UserContractsLoaded + scanResult.Summary.BuiltInContractsLoaded,
			UserContractsLoaded:    scanResult.Summary.UserContractsLoaded,
			BuiltInContractsLoaded: scanResult.Summary.BuiltInContractsLoaded,
			ContractCoverage:       fmt.Sprintf("%.2f%%", scanResult.Summary.ContractCoveragePercentage),
			RailCoverage:           htmlRailCoverage(scanResult),
		},
		Findings:          htmlFindings(scanResult),
		Contracts:         htmlContracts(input.ContractIR, input.UserContracts),
		Components:        htmlComponents(input.Graph),
		Rails:             htmlRails(input.Graph),
		EmbeddedScanJSON:  template.JS(string(scanJSON)),
		EmbeddedGraphJSON: template.JS(string(graphJSON)),
	}, nil
}

func htmlStatus(exitCode int) (string, string) {
	switch exitCode {
	case 0:
		return "PASS", "status-pass"
	case 1:
		return "WARN", "status-warn"
	default:
		return "FAIL", "status-fail"
	}
}

func htmlRailCoverage(scanResult report.VerificationReport) string {
	if scanResult.Derived == nil {
		return "n/a"
	}
	coverage := scanResult.Derived.RailCoverage
	if coverage.TotalNets == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% %s", coverage.CoverageRatio*100, strings.TrimSpace(coverage.OverallLevel))
}

func htmlFindings(scanResult report.VerificationReport) []htmlFinding {
	scanResult = report.CanonicalizeVerificationReport(scanResult)
	out := make([]htmlFinding, 0, len(scanResult.Findings))
	for _, finding := range scanResult.Findings {
		ciFinding := scanBuildCIFinding(finding)
		out = append(out, htmlFinding{
			Severity:       ciFinding.Severity,
			Class:          "severity-" + strings.ToLower(ciFinding.Severity),
			ContractID:     ciFinding.ContractID,
			Source:         ciFinding.ContractSource,
			Component:      ciFinding.ComponentRef,
			Net:            ciFinding.Net,
			Message:        ciFinding.Message,
			WhyThisMatters: htmlOptionalText(ciFinding.WhyThisMatters),
			Fix:            ciFinding.Fix,
		})
	}
	return out
}

func htmlOptionalText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func htmlReportDisplayInputPath(inputPath string) string {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return ""
	}
	clean := filepath.Clean(inputPath)
	if clean == "." {
		if wd, err := os.Getwd(); err == nil {
			if base := filepath.Base(wd); base != "." && base != string(filepath.Separator) {
				return base
			}
		}
		return clean
	}
	if info, err := os.Stat(clean); err == nil && info.IsDir() {
		return filepath.Base(clean)
	}
	if strings.EqualFold(filepath.Base(clean), "generated.net") && filepath.Base(filepath.Dir(clean)) == ".architon" {
		projectRoot := filepath.Dir(filepath.Dir(clean))
		if projectRoot == "." || projectRoot == "" {
			if wd, err := os.Getwd(); err == nil {
				return filepath.Base(wd)
			}
		} else {
			projectBase := filepath.Base(projectRoot)
			if projectBase != string(filepath.Separator) && projectBase != "" {
				return projectBase
			}
		}
	}
	return inputPath
}

func htmlContracts(contractIR *contracts.ContractIR, userContracts []contracts.SystemContract) []htmlContract {
	rows := make([]htmlContract, 0)
	seen := map[string]struct{}{}
	addSystemContract := func(contract contracts.SystemContract, defaultSource contracts.ContractSourceKind) {
		id := strings.TrimSpace(contract.ID)
		if id == "" {
			id = strings.TrimSpace(contract.MPN)
		}
		if id == "" {
			return
		}
		source := contract.SourceKind
		if source == "" {
			source = defaultSource
		}
		requirementTypes := make([]string, 0, len(contract.Requirements))
		requirementSeen := map[string]struct{}{}
		severity := ""
		for _, req := range contract.Requirements {
			if req.Type != "" {
				reqType := string(req.Type)
				if _, ok := requirementSeen[reqType]; !ok {
					requirementSeen[reqType] = struct{}{}
					requirementTypes = append(requirementTypes, reqType)
				}
			}
			severity = htmlMaxSeverity(severity, req.Severity)
		}
		sort.Strings(requirementTypes)
		row := htmlContract{
			ID:       id,
			Source:   string(source),
			Severity: normalizeSeverity(severity),
			Type:     strings.Join(requirementTypes, ", "),
		}
		key := htmlContractKey(row)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		rows = append(rows, row)
	}

	for _, contract := range userContracts {
		addSystemContract(contract, contracts.ContractSourceUserYAML)
	}
	for _, contract := range contracts.BuiltinContracts() {
		addSystemContract(contract, contracts.ContractSourceBuiltIn)
	}

	if contractIR == nil {
		sortHTMLContracts(rows)
		return rows
	}
	for _, req := range contractIR.AppliedRequirements {
		id := strings.TrimSpace(req.ContractID)
		if id == "" {
			id = strings.TrimSpace(req.Provenance.SourceID)
		}
		if id == "" {
			id = string(req.Type)
		}
		source := strings.TrimSpace(string(req.ContractSource))
		if source == "" {
			source = string(contracts.ReportContractSource(req.Source))
		}
		if source == string(contracts.ContractSourceUserYAML) || source == string(contracts.ContractSourceBuiltIn) {
			continue
		}
		severity := normalizeSeverity(req.Severity)
		row := htmlContract{
			ID:        id,
			Source:    source,
			Severity:  severity,
			Component: req.ComponentRef,
			Type:      string(req.Type),
		}
		key := htmlContractKey(row)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, row)
	}
	sortHTMLContracts(rows)
	return rows
}

func sortHTMLContracts(rows []htmlContract) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ID != rows[j].ID {
			return rows[i].ID < rows[j].ID
		}
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		return rows[i].Type < rows[j].Type
	})
}

func htmlContractKey(row htmlContract) string {
	return row.ID + "\x00" + row.Source + "\x00" + row.Component + "\x00" + row.Type
}

func htmlMaxSeverity(current string, next string) string {
	current = normalizeSeverity(current)
	next = normalizeSeverity(next)
	rank := map[string]int{"INFO": 1, "WARN": 2, "ERROR": 3}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func htmlComponents(graph graphir.GraphIR) []htmlComponent {
	out := make([]htmlComponent, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		out = append(out, htmlComponent{
			Ref:              node.Ref,
			Value:            node.Metadata.Value,
			Type:             node.Type,
			ContractCoverage: node.ContractCoverage,
			FindingsCount:    len(node.FindingIDs),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

func htmlRails(graph graphir.GraphIR) []htmlRail {
	out := make([]htmlRail, 0, len(graph.Rails))
	for _, rail := range graph.Rails {
		voltage := "unknown"
		if rail.VoltageV != nil {
			voltage = fmt.Sprintf("%.3g V", *rail.VoltageV)
		}
		consumers := append([]string{}, rail.Consumers...)
		sort.Strings(consumers)
		out = append(out, htmlRail{
			Name:          rail.Name,
			Voltage:       voltage,
			Source:        rail.SourceRef,
			Consumers:     strings.Join(consumers, ", "),
			FindingsCount: len(rail.FindingIDs),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

const htmlReportTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f5f7fa;
      --panel: #ffffff;
      --panel-2: #eef2f6;
      --text: #17202a;
      --muted: #5f6f82;
      --line: #d9e0e8;
      --pass: #147d4f;
      --warn: #96620d;
      --fail: #b42318;
      --ink: #0f1720;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      line-height: 1.45;
    }
    main {
      width: min(1180px, calc(100% - 32px));
      margin: 0 auto;
      padding: 28px 0 44px;
    }
    header {
      background: var(--ink);
      color: #ffffff;
      border-bottom: 4px solid #5fb3a3;
    }
    .header-inner {
      width: min(1180px, calc(100% - 32px));
      margin: 0 auto;
      padding: 26px 0 24px;
      display: grid;
      gap: 18px;
      grid-template-columns: 1fr auto;
      align-items: end;
    }
    h1, h2 { margin: 0; letter-spacing: 0; }
    h1 { font-size: clamp(26px, 4vw, 42px); line-height: 1.05; }
    h2 { font-size: 20px; margin-bottom: 14px; }
    .subhead { color: #c9d3df; margin-top: 10px; overflow-wrap: anywhere; }
    .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 14px;
      color: #dce4ec;
      font-size: 13px;
    }
    .pill {
      border: 1px solid rgba(255,255,255,.22);
      border-radius: 999px;
      padding: 4px 9px;
      background: rgba(255,255,255,.06);
      white-space: nowrap;
    }
    .status {
      min-width: 108px;
      border-radius: 6px;
      padding: 12px 16px;
      text-align: center;
      font-size: 20px;
      font-weight: 800;
      color: #ffffff;
    }
    .status-pass { background: var(--pass); }
    .status-warn { background: var(--warn); }
    .status-fail { background: var(--fail); }
    .cards {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
      margin: 22px 0;
    }
    .card, section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: 0 1px 2px rgba(16, 24, 40, .04);
    }
    .card { padding: 14px; min-height: 92px; }
    .label {
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .04em;
    }
    .value {
      margin-top: 8px;
      font-size: 26px;
      line-height: 1.1;
      font-weight: 800;
      overflow-wrap: anywhere;
    }
    section { padding: 18px; margin-top: 16px; overflow: hidden; }
    .table-wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
      min-width: 900px;
    }
    th {
      background: var(--panel-2);
      color: #354253;
      text-align: left;
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: .04em;
      padding: 10px;
      border-bottom: 1px solid var(--line);
      white-space: nowrap;
    }
    td {
      padding: 10px;
      border-bottom: 1px solid var(--line);
      vertical-align: top;
      overflow-wrap: anywhere;
    }
    tr:last-child td { border-bottom: 0; }
    code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
      font-size: 12px;
      background: #eef2f6;
      border: 1px solid #d8e0e8;
      border-radius: 4px;
      padding: 1px 4px;
    }
    .sev {
      display: inline-block;
      min-width: 58px;
      border-radius: 999px;
      padding: 3px 8px;
      color: #ffffff;
      font-weight: 800;
      font-size: 11px;
      text-align: center;
    }
    .severity-error { background: var(--fail); }
    .severity-warn { background: var(--warn); }
    .severity-info { background: #38698a; }
    .empty {
      color: var(--muted);
      background: #fafbfc;
      border: 1px dashed var(--line);
      border-radius: 8px;
      padding: 18px;
    }
    .data-note {
      color: var(--muted);
      font-size: 13px;
      margin: 0;
    }
    @media (max-width: 900px) {
      .header-inner { grid-template-columns: 1fr; align-items: start; }
      .status { width: max-content; }
      .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    }
    @media (max-width: 560px) {
      main, .header-inner { width: min(100% - 20px, 1180px); }
      .cards { grid-template-columns: 1fr; }
      section { padding: 14px; }
    }
  </style>
</head>
<body>
  <header>
    <div class="header-inner">
      <div>
        <h1>Architon Offline Report</h1>
        <div class="subhead">{{.InputPath}}</div>
        <div class="meta">
          <span class="pill">rv {{.RVVersion}}</span>
          <span class="pill">report version {{.ReportVersion}}</span>
        </div>
      </div>
      <div class="status {{.StatusClass}}">{{.Status}}</div>
    </div>
  </header>
  <main>
    <div class="cards" aria-label="Summary">
      <div class="card"><div class="label">Violations</div><div class="value">{{.Summary.Violations}}</div></div>
      <div class="card"><div class="label">Warnings</div><div class="value">{{.Summary.Warnings}}</div></div>
      <div class="card"><div class="label">Contracts Loaded</div><div class="value">{{.Summary.ContractsLoaded}}</div></div>
      <div class="card"><div class="label">User Contracts</div><div class="value">{{.Summary.UserContractsLoaded}}</div></div>
      <div class="card"><div class="label">Built-in Contracts</div><div class="value">{{.Summary.BuiltInContractsLoaded}}</div></div>
      <div class="card"><div class="label">Contract Coverage</div><div class="value">{{.Summary.ContractCoverage}}</div></div>
      <div class="card"><div class="label">Rail Coverage</div><div class="value">{{.Summary.RailCoverage}}</div></div>
    </div>

    <section>
      <h2>Findings</h2>
      {{if .Findings}}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Severity</th><th>Contract ID</th><th>Source</th><th>Component</th><th>Net</th><th>Message</th><th>Why it matters</th><th>Fix</th>
            </tr>
          </thead>
          <tbody>
            {{range .Findings}}
            <tr>
              <td><span class="sev {{.Class}}">{{.Severity}}</span></td>
              <td><code>{{.ContractID}}</code></td>
              <td>{{.Source}}</td>
              <td>{{.Component}}</td>
              <td>{{.Net}}</td>
              <td>{{.Message}}</td>
              <td>{{.WhyThisMatters}}</td>
              <td>{{.Fix}}</td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
      {{else}}
      <div class="empty">No findings.</div>
      {{end}}
    </section>

    <section>
      <h2>Contracts</h2>
      {{if .Contracts}}
      <div class="table-wrap">
        <table>
          <thead><tr><th>Contract ID</th><th>Source</th><th>Severity</th><th>Component</th><th>Requirement</th></tr></thead>
          <tbody>
            {{range .Contracts}}
            <tr><td><code>{{.ID}}</code></td><td>{{.Source}}</td><td>{{.Severity}}</td><td>{{.Component}}</td><td>{{.Type}}</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
      {{else}}
      <div class="empty">No contract requirements were applied.</div>
      {{end}}
    </section>

    <section>
      <h2>Components</h2>
      {{if .Components}}
      <div class="table-wrap">
        <table>
          <thead><tr><th>Component Ref</th><th>Value</th><th>Type</th><th>Contract Coverage</th><th>Findings Count</th></tr></thead>
          <tbody>
            {{range .Components}}
            <tr><td><code>{{.Ref}}</code></td><td>{{.Value}}</td><td>{{.Type}}</td><td>{{.ContractCoverage}}</td><td>{{.FindingsCount}}</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
      {{else}}
      <div class="empty">No components were imported.</div>
      {{end}}
    </section>

    <section>
      <h2>Rails</h2>
      {{if .Rails}}
      <div class="table-wrap">
        <table>
          <thead><tr><th>Rail</th><th>Voltage</th><th>Source</th><th>Consumers</th><th>Findings</th></tr></thead>
          <tbody>
            {{range .Rails}}
            <tr><td><code>{{.Name}}</code></td><td>{{.Voltage}}</td><td>{{.Source}}</td><td>{{.Consumers}}</td><td>{{.FindingsCount}}</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
      {{else}}
      <div class="empty">No rails were detected.</div>
      {{end}}
    </section>

    <section>
      <h2>Embedded Data</h2>
      <p class="data-note">The scan report and GraphIR payloads are embedded below for offline inspection and artifact reuse.</p>
    </section>
  </main>
  <script type="application/json" id="architon-scan-json">{{.EmbeddedScanJSON}}</script>
  <script type="application/json" id="architon-graph-json">{{.EmbeddedGraphJSON}}</script>
</body>
</html>
`
