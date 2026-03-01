package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/badimirzai/architon-cli/internal/model"
	"github.com/badimirzai/architon-cli/internal/output"
	"github.com/badimirzai/architon-cli/internal/resolve"
	"github.com/badimirzai/architon-cli/internal/ui"
	"github.com/badimirzai/architon-cli/internal/validate"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkCmd = newCheckCmd()

type checkRunResult struct {
	Target   string
	Report   validate.Report
	Errors   int
	Warnings int
	Notes    int
}

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check <spec.yaml>",
		Aliases: []string{"validate"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Validate a robot spec against deterministic electrical rules",
		Long: `Validate a robot spec against deterministic electrical rules.

Output control flags:
  --output json             print machine readable JSON to stdout
  --style report|classic    force human-readable style
  --pretty                  pretty print JSON to stdout (requires --output json)
  --out-file <path>         write compact JSON to file (requires --output json)
  --debug                   enable debug mode (or use RV_DEBUG=1)

Examples:
  rv check robot.yaml --output json
  rv check robot.yaml --output json --pretty
  rv check robot.yaml --output json --out-file report.json
  rv check robot.yaml --output json --pretty --out-file report.json`,
		RunE: runCheckCommand,
	}
	cmd.Flags().StringP("file", "f", "", "Path to YAML spec")
	cmd.Flags().StringP("output", "o", "text", "Output format: text or json")
	cmd.Flags().String("style", "", "Human output style: report or classic")
	cmd.Flags().Bool("pretty", false, "Pretty print JSON to stdout (requires --output json)")
	cmd.Flags().String("out-file", "", "Write compact JSON to file (requires --output json)")
	cmd.Flags().StringArray("parts-dir", nil, "Additional parts directory (repeatable; after rv_parts and built-in parts)")
	return cmd
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheckCommand(cmd *cobra.Command, args []string) (err error) {
	outputFormat := strings.ToLower(strings.TrimSpace(getOutputFormat(cmd)))
	styleFlag, _ := cmd.Flags().GetString("style")
	styleFlagSet := cmd.Flags().Changed("style")
	prettyOutput, _ := cmd.Flags().GetBool("pretty")
	outFile, _ := cmd.Flags().GetString("out-file")
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()
	stdoutTTY := isWriterTTY(stdout)
	var specFile string

	defer func() {
		if recovered := recover(); recovered != nil {
			msg := fmt.Sprintf("panic: %v", recovered)
			stack := string(debug.Stack())
			if outputFormat == "json" {
				var dbg *output.Debug
				if debugEnabled {
					dbg = &output.Debug{InternalError: msg, Stacktrace: stack}
				}
				_ = renderJSONErrorOutputs(stdout, stderr, specFile, 3, "internal error: unexpected panic", prettyOutput, outFile, dbg)
			} else {
				if debugEnabled {
					fmt.Fprintln(stderr, msg)
					fmt.Fprintln(stderr, stack)
				} else {
					fmt.Fprintln(stderr, "internal error: unexpected panic (run with --debug or RV_DEBUG=1)")
				}
				printExitCode(stdout, 3)
			}
			err = silentExit(3)
		}
	}()

	path, _ := cmd.Flags().GetString("file")
	if path == "" && len(args) > 0 {
		path = args[0]
	}
	specFile = path
	if path == "" {
		return handleCheckError(stdout, stderr, outputFormat, 3, "", fmt.Errorf("missing spec file (arg or -f/--file)"), nil, prettyOutput, outFile)
	}

	mode, modeErr := selectRenderMode(renderModeOptions{
		OutputFormat:  outputFormat,
		Style:         styleFlag,
		StyleProvided: styleFlagSet,
		StdoutIsTTY:   stdoutTTY,
	})
	if modeErr != nil {
		return handleCheckError(stdout, stderr, outputFormat, 3, path, modeErr, nil, prettyOutput, outFile)
	}
	if outFile != "" && mode != renderModeJSON {
		return handleCheckError(stdout, stderr, outputFormat, 3, path, fmt.Errorf("--out-file requires --output json"), nil, prettyOutput, outFile)
	}
	if prettyOutput && mode != renderModeJSON {
		return handleCheckError(stdout, stderr, outputFormat, 3, path, fmt.Errorf("--pretty requires --output json"), nil, prettyOutput, outFile)
	}

	partsDirs, _ := cmd.Flags().GetStringArray("parts-dir")
	result, runErr := executeCheck(path, partsDirs, os.Getenv("RV_PARTS_DIRS"))
	if runErr != nil {
		return handleCheckError(stdout, stderr, outputFormat, 3, path, runErr, nil, prettyOutput, outFile)
	}

	exitCode := checkExitCode(result)
	renderResult := output.CheckResult{
		Target:   result.Target,
		Report:   result.Report,
		ExitCode: exitCode,
	}

	if mode == renderModeJSON {
		if err := renderJSONOutputs(stdout, stderr, result.Target, result.Report, exitCode, prettyOutput, outFile, nil); err != nil {
			return err
		}
	} else if mode == renderModeClassic {
		rendered := output.ClassicRenderer{}.Render(renderResult, output.RenderOptions{})
		fmt.Fprintln(stdout, rendered)
		printExitCode(stdout, exitCode)
	} else {
		rendered := output.ReportRenderer{}.Render(renderResult, output.RenderOptions{Width: 92})
		fmt.Fprint(stdout, rendered)
	}

	if exitCode != 0 {
		return silentExit(exitCode)
	}
	return nil
}

func getOutputFormat(cmd *cobra.Command) string {
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" {
		return "text"
	}
	return outputFormat
}

func executeCheck(path string, partsDirs []string, partsEnv string) (*checkRunResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	var raw model.RobotSpec
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := doc.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}

	store, err := buildPartsStore(partsDirs, partsEnv)
	if err != nil {
		return nil, fmt.Errorf("build parts search paths: %w", err)
	}
	resolved, err := resolve.ResolveAll(raw, store)
	if err != nil {
		return nil, fmt.Errorf("resolve spec with parts: %w", err)
	}

	locs := buildLocationMap(path, &doc)
	rep := validate.RunAll(resolved, locs)
	result := &checkRunResult{
		Target: path,
		Report: rep,
	}
	for _, finding := range rep.Findings {
		switch finding.Severity {
		case validate.SevError:
			result.Errors++
		case validate.SevWarn:
			result.Warnings++
		case validate.SevInfo:
			result.Notes++
		}
	}
	return result, nil
}

func checkExitCode(result *checkRunResult) int {
	if result == nil {
		return 3
	}
	if result.Errors > 0 {
		return 2
	}
	if result.Warnings > 0 {
		return 1
	}
	return 0
}

func printExitCode(w io.Writer, code int) {
	fmt.Fprintf(w, "exit code: %d\n", code)
}

type renderMode string

const (
	renderModeJSON    renderMode = "json"
	renderModeClassic renderMode = "classic"
	renderModeReport  renderMode = "report"
)

type renderModeOptions struct {
	OutputFormat  string
	Style         string
	StyleProvided bool
	StdoutIsTTY   bool
}

func selectRenderMode(opts renderModeOptions) (renderMode, error) {
	outputFormat := strings.ToLower(strings.TrimSpace(opts.OutputFormat))
	switch outputFormat {
	case "", "text":
		// Continue below with human style selection.
	case string(renderModeJSON):
		return renderModeJSON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (allowed: text, json)", outputFormat)
	}

	if opts.StyleProvided {
		style := strings.ToLower(strings.TrimSpace(opts.Style))
		switch style {
		case string(renderModeClassic):
			return renderModeClassic, nil
		case string(renderModeReport):
			return renderModeReport, nil
		default:
			return "", fmt.Errorf("unsupported style %q (allowed: report, classic)", opts.Style)
		}
	}

	if opts.StdoutIsTTY {
		return renderModeReport, nil
	}
	return renderModeClassic, nil
}

func isWriterTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func handleCheckError(stdout io.Writer, stderr io.Writer, outputFormat string, exitCode int, specFile string, err error, debugInfo *output.Debug, pretty bool, outFile string) error {
	if outputFormat == "json" {
		if err := renderJSONErrorOutputs(stdout, stderr, specFile, exitCode, err.Error(), pretty, outFile, debugInfo); err != nil {
			return err
		}
		return silentExit(exitCode)
	}
	if debugEnabled && debugInfo != nil && debugInfo.InternalError != "" {
		fmt.Fprintln(stderr, ui.Colorize("ERROR", debugInfo.InternalError))
		if debugInfo.Stacktrace != "" {
			fmt.Fprintln(stderr, debugInfo.Stacktrace)
		}
	} else {
		fmt.Fprintln(stderr, ui.Colorize("ERROR", err.Error()))
	}
	printExitCode(stdout, exitCode)
	return silentExit(exitCode)
}

func renderJSONOutputs(stdout io.Writer, stderr io.Writer, path string, report validate.Report, exitCode int, pretty bool, outFile string, debugInfo *output.Debug) error {
	payload, summary, err := output.RenderJSONReport(path, report, exitCode, debugInfo)
	if err != nil {
		return internalError(err)
	}
	_ = summary

	if outFile != "" {
		compact, err := output.FormatJSON(payload, false)
		if err != nil {
			return internalError(err)
		}
		if writeErr := os.WriteFile(outFile, compact, 0o644); writeErr != nil {
			fmt.Fprintln(stderr, "write json:", writeErr)
			return silentExit(3)
		}
	}

	prettyBytes, err := output.FormatJSON(payload, pretty)
	if err != nil {
		return internalError(err)
	}
	prettyBytes = output.ColorizeJSON(prettyBytes)
	fmt.Fprintln(stdout, string(prettyBytes))
	if outFile != "" && !pretty {
		fmt.Fprintf(stdout, "Written to %s\n", outFile)
	}
	return nil
}

func renderJSONErrorOutputs(stdout io.Writer, stderr io.Writer, specFile string, exitCode int, message string, pretty bool, outFile string, debugInfo *output.Debug) error {
	path := specFile
	if path == "" {
		path = "spec.yaml"
	}
	payload, summary, err := output.RenderJSONError(path, exitCode, message, debugInfo)
	if err != nil {
		fmt.Fprintf(stdout, `{"spec_file":"%s","summary":{"errors":1,"warnings":0,"infos":0,"exit_code":%d},"findings":[{"id":"PARSER_ERROR","severity":"ERROR","message":"failed to render json error","path":null,"location":null,"meta":{}}]}`+"\n", path, exitCode)
		return nil
	}
	_ = summary

	if outFile != "" {
		compact, err := output.FormatJSON(payload, false)
		if err != nil {
			return internalError(err)
		}
		if writeErr := os.WriteFile(outFile, compact, 0o644); writeErr != nil {
			fmt.Fprintln(stderr, "write json:", writeErr)
			return silentExit(3)
		}
	}

	b, err := output.FormatJSON(payload, pretty)
	if err != nil {
		return internalError(err)
	}
	b = output.ColorizeJSON(b)
	fmt.Fprintln(stdout, string(b))
	if outFile != "" && !pretty {
		fmt.Fprintf(stdout, "Written to %s\n", outFile)
	}
	return nil
}

func buildLocationMap(path string, doc *yaml.Node) map[string]validate.Location {
	locs := make(map[string]validate.Location)

	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, child := range n.Content {
				walk(child, prefix)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				key := n.Content[i]
				val := n.Content[i+1]
				if key.Kind != yaml.ScalarNode {
					continue
				}
				next := key.Value
				if prefix != "" {
					next = prefix + "." + key.Value
				}
				locs[next] = validate.Location{File: path, Line: key.Line, Column: key.Column}
				if val.Kind == yaml.ScalarNode {
					locs[next] = validate.Location{File: path, Line: val.Line, Column: val.Column}
				}
				walk(val, next)
			}
		case yaml.SequenceNode:
			for i, item := range n.Content {
				next := fmt.Sprintf("%s[%d]", prefix, i)
				locs[next] = validate.Location{File: path, Line: item.Line, Column: item.Column}
				if item.Kind == yaml.ScalarNode {
					locs[next] = validate.Location{File: path, Line: item.Line, Column: item.Column}
				}
				walk(item, next)
			}
		}
	}

	if doc != nil {
		walk(doc, "")
	}

	return locs
}
