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

var checkCmd = &cobra.Command{
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
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		outputFormat := strings.ToLower(strings.TrimSpace(getOutputFormat(cmd)))
		styleFlag, _ := cmd.Flags().GetString("style")
		styleFlagSet := cmd.Flags().Changed("style")
		prettyOutput, _ := cmd.Flags().GetBool("pretty")
		outFile, _ := cmd.Flags().GetString("out-file")
		var specFile string
		stdoutTTY := isWriterTTY(cmd.OutOrStdout())

		defer func() {
			if recovered := recover(); recovered != nil {
				msg := fmt.Sprintf("panic: %v", recovered)
				stack := string(debug.Stack())
				if outputFormat == "json" {
					var dbg *output.Debug
					if debugEnabled {
						dbg = &output.Debug{InternalError: msg, Stacktrace: stack}
					}
					_ = renderJSONErrorOutputs(specFile, 3, "internal error: unexpected panic", prettyOutput, outFile, dbg)
				} else {
					if debugEnabled {
						fmt.Fprintln(os.Stderr, msg)
						fmt.Fprintln(os.Stderr, stack)
					} else {
						fmt.Fprintln(os.Stderr, "internal error: unexpected panic (run with --debug or RV_DEBUG=1)")
					}
					printExitCode(3)
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
			return handleCheckError(outputFormat, 3, "", fmt.Errorf("missing spec file (arg or -f/--file)"), nil, prettyOutput, outFile)
		}
		mode, modeErr := selectRenderMode(renderModeOptions{
			OutputFormat:  outputFormat,
			Style:         styleFlag,
			StyleProvided: styleFlagSet,
			StdoutIsTTY:   stdoutTTY,
		})
		if modeErr != nil {
			return handleCheckError(outputFormat, 3, path, modeErr, nil, prettyOutput, outFile)
		}
		if outFile != "" && mode != renderModeJSON {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("--out-file requires --output json"), nil, prettyOutput, outFile)
		}
		if prettyOutput && mode != renderModeJSON {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("--pretty requires --output json"), nil, prettyOutput, outFile)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("read spec: %w", err), nil, prettyOutput, outFile)
		}

		var raw model.RobotSpec
		var doc yaml.Node
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("parse yaml: %w", err), nil, prettyOutput, outFile)
		}
		if err := doc.Decode(&raw); err != nil {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("decode yaml: %w", err), nil, prettyOutput, outFile)
		}

		partsDirs, _ := cmd.Flags().GetStringArray("parts-dir")
		store, err := buildPartsStore(partsDirs, os.Getenv("RV_PARTS_DIRS"))
		if err != nil {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("build parts search paths: %w", err), nil, prettyOutput, outFile)
		}
		resolved, err := resolve.ResolveAll(raw, store)
		if err != nil {
			return handleCheckError(outputFormat, 3, path, fmt.Errorf("resolve spec with parts: %w", err), nil, prettyOutput, outFile)
		}

		locs := buildLocationMap(path, &doc)
		rep := validate.RunAll(resolved, locs)
		exitCode := 0
		if rep.HasErrors() {
			exitCode = 2
		}

		if mode == renderModeJSON {
			if err := renderJSONOutputs(path, rep, exitCode, prettyOutput, outFile, nil); err != nil {
				return err
			}
		} else if mode == renderModeClassic {
			rendered := output.ClassicRenderer{}.Render(output.CheckResult{
				Target:   path,
				Report:   rep,
				ExitCode: exitCode,
			}, output.RenderOptions{})
			fmt.Println(rendered)
			printExitCode(exitCode)
		} else {
			rendered := output.ReportRenderer{}.Render(output.CheckResult{
				Target:   path,
				Report:   rep,
				ExitCode: exitCode,
			}, output.RenderOptions{Width: 92})
			fmt.Print(rendered)
		}

		if exitCode != 0 {
			return silentExit(exitCode)
		}
		return nil
	},
}

func init() {
	checkCmd.Flags().StringP("file", "f", "", "Path to YAML spec")
	checkCmd.Flags().StringP("output", "o", "text", "Output format: text or json")
	checkCmd.Flags().String("style", "", "Human output style: report or classic")
	checkCmd.Flags().Bool("pretty", false, "Pretty print JSON to stdout (requires --output json)")
	checkCmd.Flags().String("out-file", "", "Write compact JSON to file (requires --output json)")
	checkCmd.Flags().StringArray("parts-dir", nil, "Additional parts directory (repeatable; after rv_parts and built-in parts)")
	rootCmd.AddCommand(checkCmd)
}

func getOutputFormat(cmd *cobra.Command) string {
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" {
		return "text"
	}
	return outputFormat
}

func printExitCode(code int) {
	fmt.Printf("exit code: %d\n", code)
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

func handleCheckError(outputFormat string, exitCode int, specFile string, err error, debugInfo *output.Debug, pretty bool, outFile string) error {
	if outputFormat == "json" {
		if err := renderJSONErrorOutputs(specFile, exitCode, err.Error(), pretty, outFile, debugInfo); err != nil {
			return err
		}
		return silentExit(exitCode)
	}
	if debugEnabled && debugInfo != nil && debugInfo.InternalError != "" {
		fmt.Fprintln(os.Stderr, ui.Colorize("ERROR", debugInfo.InternalError))
		if debugInfo.Stacktrace != "" {
			fmt.Fprintln(os.Stderr, debugInfo.Stacktrace)
		}
	} else {
		fmt.Fprintln(os.Stderr, ui.Colorize("ERROR", err.Error()))
	}
	return silentExit(exitCode)
}

func renderJSONOutputs(path string, report validate.Report, exitCode int, pretty bool, outFile string, debugInfo *output.Debug) error {
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
			fmt.Fprintln(os.Stderr, "write json:", writeErr)
			return silentExit(3)
		}
	}

	prettyBytes, err := output.FormatJSON(payload, pretty)
	if err != nil {
		return internalError(err)
	}
	prettyBytes = output.ColorizeJSON(prettyBytes)
	fmt.Println(string(prettyBytes))
	if outFile != "" && !pretty {
		fmt.Printf("Written to %s\n", outFile)
	}
	return nil
}

func renderJSONErrorOutputs(specFile string, exitCode int, message string, pretty bool, outFile string, debugInfo *output.Debug) error {
	path := specFile
	if path == "" {
		path = "spec.yaml"
	}
	payload, summary, err := output.RenderJSONError(path, exitCode, message, debugInfo)
	if err != nil {
		fmt.Printf(`{"spec_file":"%s","summary":{"errors":1,"warnings":0,"infos":0,"exit_code":%d},"findings":[{"id":"PARSER_ERROR","severity":"ERROR","message":"failed to render json error","path":null,"location":null,"meta":{}}]}`+"\n", path, exitCode)
		return nil
	}
	_ = summary

	if outFile != "" {
		compact, err := output.FormatJSON(payload, false)
		if err != nil {
			return internalError(err)
		}
		if writeErr := os.WriteFile(outFile, compact, 0o644); writeErr != nil {
			fmt.Fprintln(os.Stderr, "write json:", writeErr)
			return silentExit(3)
		}
	}

	b, err := output.FormatJSON(payload, pretty)
	if err != nil {
		return internalError(err)
	}
	b = output.ColorizeJSON(b)
	fmt.Println(string(b))
	if outFile != "" && !pretty {
		fmt.Printf("Written to %s\n", outFile)
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
