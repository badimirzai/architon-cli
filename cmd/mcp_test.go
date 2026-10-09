package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPVerify_AgentLoopFixtures(t *testing.T) {
	if !mcpCommandRegistered() {
		t.Fatal("rv mcp is not registered")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	server := newMCPServer()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "architon-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		serverSession.Wait()
	})

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) != 3 {
		t.Fatalf("expected verify, propose, and apply, got %+v", listed.Tools)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{verifyToolName, proposeToolName, applyToolName} {
		if byName[name] == nil {
			t.Fatalf("missing %s in %+v", name, listed.Tools)
		}
	}
	assertVerifyInputSchema(t, byName[verifyToolName])

	broken := examplePath(t, filepath.Join("agent-loop", "broken"))
	fixed := examplePath(t, filepath.Join("agent-loop", "fixed"))

	brokenCLI, brokenCode := scanCLIStdout(t, broken, "")
	if brokenCode != 2 {
		t.Fatalf("broken fixture scan exited %d, want 2\n%s", brokenCode, brokenCLI)
	}
	brokenTool := callVerify(t, ctx, session, broken, "")
	if brokenTool.ExitCode != 2 {
		t.Fatalf("verify exit_code = %d, want 2; error=%s", brokenTool.ExitCode, brokenTool.Error)
	}
	assertSameScanJSON(t, brokenTool.Scan, brokenCLI)
	assertBrokenFixtureFindings(t, brokenTool.Scan)
	if !strings.Contains(string(brokenTool.Scan), `"not_checked"`) {
		t.Fatal("verify result dropped not_checked")
	}

	contractsPath := filepath.Join(broken, ".architon", "contracts.yaml")
	brokenExplicit := callVerify(t, ctx, session, broken, contractsPath)
	explicitCLI, explicitCode := scanCLIStdout(t, broken, contractsPath)
	if explicitCode != 2 || brokenExplicit.ExitCode != 2 {
		t.Fatalf("explicit contracts exit codes tool=%d cli=%d", brokenExplicit.ExitCode, explicitCode)
	}
	assertSameScanJSON(t, brokenExplicit.Scan, explicitCLI)

	fixedCLI, fixedCode := scanCLIStdout(t, fixed, "")
	if fixedCode != 0 {
		t.Fatalf("fixed fixture scan exited %d, want 0\n%s", fixedCode, fixedCLI)
	}
	fixedTool := callVerify(t, ctx, session, fixed, "")
	if fixedTool.ExitCode != 0 {
		t.Fatalf("verify exit_code = %d, want 0; error=%s scan=%s", fixedTool.ExitCode, fixedTool.Error, fixedTool.Scan)
	}
	assertSameScanJSON(t, fixedTool.Scan, fixedCLI)
	fixedScan := decodeScanCI(t, fixedTool.Scan)
	if len(fixedScan.Findings) != 0 || fixedScan.Summary.Violations != 0 || fixedScan.Summary.HasFailures {
		t.Fatalf("expected a clean scan, got %+v", fixedScan)
	}

	empty := t.TempDir()
	emptyCLI, emptyCode := scanCLIStdout(t, empty, "")
	if emptyCode != 3 {
		t.Fatalf("empty project scan exited %d, want 3\n%s", emptyCode, emptyCLI)
	}
	emptyTool := callVerify(t, ctx, session, empty, "")
	if emptyTool.ExitCode != 3 {
		t.Fatalf("verify exit_code = %d, want 3; error=%s", emptyTool.ExitCode, emptyTool.Error)
	}
	if strings.TrimSpace(emptyCLI) == "" {
		if len(emptyTool.Scan) != 0 || emptyTool.Error == "" {
			t.Fatalf("tool failure should keep exit code 3 and the scan error, got %+v", emptyTool)
		}
		return
	}
	assertSameScanJSON(t, emptyTool.Scan, emptyCLI)
}

func mcpCommandRegistered() bool {
	for _, command := range rootCmd.Commands() {
		if command.Name() == "mcp" {
			return true
		}
	}
	return false
}

func assertVerifyInputSchema(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("input schema: %v\n%s", err, data)
	}
	if _, ok := schema.Properties["project_path"]; !ok {
		t.Fatalf("input schema missing project_path: %s", data)
	}
	if _, ok := schema.Properties["contracts_path"]; !ok {
		t.Fatalf("input schema missing contracts_path: %s", data)
	}
	if len(schema.Properties) != 2 {
		t.Fatalf("verify should take only project_path and contracts_path, got %s", data)
	}
	required := map[string]bool{}
	for _, name := range schema.Required {
		required[name] = true
	}
	if !required["project_path"] || required["contracts_path"] {
		t.Fatalf("project_path must be required and contracts_path optional, got %s", data)
	}
}

func assertBrokenFixtureFindings(t *testing.T, raw json.RawMessage) {
	t.Helper()
	scan := decodeScanCI(t, raw)
	if len(scan.Findings) != 2 {
		t.Fatalf("expected two findings, got %+v", scan.Findings)
	}
	pin := requireCIFinding(t, scan, "interface_pin_mismatch")
	if pin.Severity != "ERROR" || pin.ComponentRef != "U1" || pin.Net != "SPI_MOSI" || pin.Pin != "4" || !pin.DesignFixable {
		t.Fatalf("unexpected pin mismatch finding: %+v", pin)
	}
	if pin.Expected == nil || pin.Expected.Text != "PB15" || pin.Observed == nil || pin.Observed.Text != "PA7" {
		t.Fatalf("pin mismatch evidence = %+v %+v", pin.Expected, pin.Observed)
	}
	termination := requireCIFinding(t, scan, "termination_count_low")
	if termination.Severity != "ERROR" || !termination.DesignFixable {
		t.Fatalf("unexpected termination finding: %+v", termination)
	}
	if termination.Expected == nil || termination.Expected.Text != "2" || termination.Observed == nil || termination.Observed.Text != "1" {
		t.Fatalf("termination evidence = %+v %+v", termination.Expected, termination.Observed)
	}
	for _, finding := range scan.Findings {
		if finding.RuleID == "" {
			t.Fatalf("finding missing rule_id: %+v", finding)
		}
	}
}

func callVerify(t *testing.T, ctx context.Context, session *mcp.ClientSession, projectPath, contractsPath string) verifyOutput {
	t.Helper()
	args := map[string]any{"project_path": projectPath}
	if contractsPath != "" {
		args["contracts_path"] = contractsPath
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      verifyToolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("call verify: %v", err)
	}
	if res.IsError {
		t.Fatalf("verify returned a tool error: %s", toolText(res))
	}
	out := decodeVerifyOutput(t, res.StructuredContent)
	text := toolText(res)
	var fromText verifyOutput
	if err := json.Unmarshal([]byte(text), &fromText); err != nil {
		t.Fatalf("verify text is not the result JSON: %v\n%s", err, text)
	}
	if fromText.ExitCode != out.ExitCode || fromText.Error != out.Error {
		t.Fatalf("text result %+v != structured %+v", fromText, out)
	}
	if canonicalJSON(t, fromText.Scan) != canonicalJSON(t, out.Scan) {
		t.Fatal("text scan JSON differs from structured scan JSON")
	}
	return out
}

func scanCLIStdout(t *testing.T, projectPath, contractsPath string) (string, int) {
	t.Helper()
	cmd := newScanCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	args := []string{projectPath, "--format", "json", "--out", filepath.Join(t.TempDir(), "report.json")}
	if contractsPath != "" {
		args = append(args, "--contracts", contractsPath)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return strings.TrimSpace(stdout.String()), exitCodeOf(err)
}

func decodeVerifyOutput(t *testing.T, structured any) verifyOutput {
	t.Helper()
	data, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out verifyOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("structured content: %v\n%s", err, data)
	}
	return out
}

func decodeScanCI(t *testing.T, raw json.RawMessage) scanCIOutput {
	t.Helper()
	var scan scanCIOutput
	if err := json.Unmarshal(raw, &scan); err != nil {
		t.Fatalf("scan JSON: %v\n%s", err, raw)
	}
	return scan
}

func assertSameScanJSON(t *testing.T, got json.RawMessage, cli string) {
	t.Helper()
	if canonicalJSON(t, got) != canonicalJSON(t, []byte(cli)) {
		t.Fatalf("MCP scan JSON differs from rv scan --format json\nMCP: %s\nCLI: %s", got, cli)
	}
}

func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("canonical json: %v\n%s", err, raw)
	}
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal canonical json: %v", err)
	}
	return string(out)
}

func toolText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok || text == nil {
		return ""
	}
	return text.Text
}
