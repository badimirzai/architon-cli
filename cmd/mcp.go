package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/badimirzai/architon-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const verifyToolName = "verify"

const verifyToolDescription = "Verify a project with the same scan as `rv scan <project_path> --format json`. " +
	"contracts_path is optional and overrides .architon/contracts.yaml. " +
	"scan is the JSON that command prints. Read findings for rule_id, severity, component_ref, net, pin, expected, observed, and design_fixable. " +
	"exit_code is that scan's process result: 0 clean or info, 1 warnings, 2 violations, 3 tool or import failure. " +
	"Call again after the project changes."

const mcpInstructions = "Call verify with a project path. scan is the JSON from rv scan --format json. " +
	"Read rule_id, severity, component_ref, net, pin, expected, observed, and design_fixable from scan.findings. " +
	"exit_code is 0, 1, 2, or 3. Call verify again after the project changes."

// mcpClosedWorld marks verify as closed-world: it does not reach the network.
var mcpClosedWorld = false

type verifyArgs struct {
	ProjectPath   string `json:"project_path" jsonschema:"directory or design file to verify"`
	ContractsPath string `json:"contracts_path,omitempty" jsonschema:"optional contracts file; when omitted, scan uses .architon/contracts.yaml if present"`
}

// verifyOutput is the verify tool result.
// Scan is the JSON object printed by rv scan --format json, unchanged.
type verifyOutput struct {
	ExitCode int             `json:"exit_code"`
	Scan     json.RawMessage `json:"scan,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func init() {
	rootCmd.AddCommand(newMCPCmd())
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "mcp",
		Short:         "Serve project verification as one MCP tool",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Serve one MCP tool on stdio.

The tool is verify. It runs the same scan as:

  rv scan <project_path> --format json
  rv scan <project_path> --format json --contracts <contracts_path>

verify arguments:
  project_path    Project directory or design file
  contracts_path  Optional contracts file. When omitted, scan uses
                  .architon/contracts.yaml if that file exists.

The result is JSON:
  exit_code  Scan process result: 0 clean or info, 1 warnings, 2 violations,
             3 tool, import, or internal failure
  scan       The JSON object rv scan --format json prints, including findings
             with rule_id, severity, component_ref, net, pin, expected,
             observed, and design_fixable
  error      Set when scan exits before writing that JSON

Call verify again after the project changes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := serveMCP(cmd.Context())
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				return fatalError(err)
			}
			return nil
		},
	}
}

func serveMCP(ctx context.Context) error {
	return newMCPServer().Run(ctx, &mcp.StdioTransport{})
}

func newMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "architon",
		Version: version.Get().Version,
		Title:   "Architon",
	}, &mcp.ServerOptions{
		Instructions: mcpInstructions,
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        verifyToolName,
		Title:       "Verify project",
		Description: verifyToolDescription,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &mcpClosedWorld,
			Title:          "Verify project",
		},
	}, verifyTool)
	return server
}

// verifyTool returns any so the scan JSON is passed through without an output schema.
func verifyTool(ctx context.Context, _ *mcp.CallToolRequest, args verifyArgs) (*mcp.CallToolResult, any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	scanJSON, exitCode, failure := runScanFormatJSON(args.ProjectPath, args.ContractsPath)
	out := verifyOutput{ExitCode: exitCode, Error: failure}
	if scanJSON != "" {
		out.Scan = json.RawMessage(scanJSON)
	}
	return nil, out, nil
}

// runScanFormatJSON runs `rv scan <project> --format json` and returns its stdout.
// The report file is temporary so the MCP process does not leave architon-report.json
// in its working directory. The findings are that command's JSON.
func runScanFormatJSON(projectPath, contractsPath string) (string, int, string) {
	projectPath = strings.TrimSpace(projectPath)
	contractsPath = strings.TrimSpace(contractsPath)
	if projectPath == "" {
		return "", 3, "project_path is required"
	}

	report, err := os.CreateTemp("", "architon-scan-*.json")
	if err != nil {
		return "", 3, err.Error()
	}
	reportPath := report.Name()
	_ = report.Close()
	defer os.Remove(reportPath)

	cmd := newScanCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	scanArgs := []string{projectPath, "--format", "json", "--out", reportPath}
	if contractsPath != "" {
		scanArgs = append(scanArgs, "--contracts", contractsPath)
	}
	cmd.SetArgs(scanArgs)

	err = cmd.Execute()
	exitCode := exitCodeOf(err)
	text := strings.TrimSpace(stdout.String())
	if json.Valid([]byte(text)) {
		return text, exitCode, ""
	}

	message := scanFailureMessage(err)
	if message == "" {
		message = strings.TrimSpace(stderr.String())
	}
	if message == "" {
		message = "scan failed"
	}
	if exitCode == 0 {
		exitCode = 3
	}
	return "", exitCode, message
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Code >= 0 && exitErr.Code <= 3 {
			return exitErr.Code
		}
		return 3
	}
	return 3
}

func scanFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Err != nil {
			return exitErr.Err.Error()
		}
		return ""
	}
	return err.Error()
}
