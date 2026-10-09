package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPInstructions(t *testing.T) {
	for _, phrase := range []string{
		"Call propose first.",
		"copying a pin from that entry's candidates",
		"rejected by apply with no file change",
		"Call apply, then call verify",
		"exit_code",
		"not_checked",
		"proved checks only",
		"verify is the only pass",
		"Do not edit the schematic except through apply.",
	} {
		if !strings.Contains(mcpInstructions, phrase) {
			t.Fatalf("instructions missing %q\n%s", phrase, mcpInstructions)
		}
	}
}

func TestMCPServerListsVerifyProposeAndApply(t *testing.T) {
	ctx, session := startMCPSession(t)
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
			t.Fatalf("missing %s", name)
		}
	}
	assertVerifyInputSchema(t, byName[verifyToolName])
	assertToolProperties(t, byName[proposeToolName].InputSchema, []string{"project_path"}, []string{"write", "force"})
	assertToolProperties(t, byName[proposeToolName].OutputSchema, []string{"decided", "needs_choice", "conflict", "citations"}, []string{"path", "error"})
	assertToolProperties(t, byName[applyToolName].InputSchema, []string{"proposal_path"}, nil)
	assertToolProperties(t, byName[applyToolName].OutputSchema, []string{"files_changed", "ids_applied"}, []string{"error"})
	assertSchemaOmits(t, byName[proposeToolName].OutputSchema, "exit_code")
	assertSchemaOmits(t, byName[applyToolName].OutputSchema, "exit_code")
	if !byName[proposeToolName].Annotations.ReadOnlyHint {
		t.Fatal("propose should be marked read-only")
	}
	if byName[applyToolName].Annotations.ReadOnlyHint {
		t.Fatal("apply writes schematic labels")
	}
}

func TestMCPProposeV019Fixture(t *testing.T) {
	dir := t.TempDir()
	writeScanTestFile(t, filepath.Join(dir, "design.net"), i2cProposalNetlist)
	contractsPath := filepath.Join(dir, ".architon", "contracts.yaml")
	schPath := filepath.Join(dir, "board.kicad_sch")
	contractsBefore := []byte("contracts: []\n")
	schBefore := []byte("(kicad_sch (version 20231120) (generator eeschema))\n")
	writeScanTestFile(t, contractsPath, string(contractsBefore))
	writeScanTestFile(t, schPath, string(schBefore))
	proposalPath := filepath.Join(dir, ".architon", "connections.proposal.yaml")

	ctx, session := startMCPSession(t)
	got := callPropose(t, ctx, session, dir, false, false)
	if got.Error != "" {
		t.Fatalf("propose error: %s", got.Error)
	}
	if got.Path != "" {
		t.Fatalf("read-only propose wrote path %s", got.Path)
	}
	assertMCUChoice(t, got)
	if _, err := os.Stat(proposalPath); !os.IsNotExist(err) {
		t.Fatalf("propose wrote a file: %v", err)
	}
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	assertBytesUnchanged(t, schPath, schBefore)

	document, builtPath, err := buildConnectionProposal(dir)
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := proposeOutputFromYAML(document.YAML)
	if err != nil {
		t.Fatal(err)
	}
	if proposeCanon(t, got) != proposeCanon(t, fromYAML) {
		t.Fatalf("propose JSON differs from the proposal YAML fields\njson: %+v\nyaml: %+v", got, fromYAML)
	}

	written := callPropose(t, ctx, session, dir, true, false)
	if written.Error != "" || written.Path != builtPath {
		t.Fatalf("write result path %q error %q, want %s", written.Path, written.Error, builtPath)
	}
	file, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(file) != document.YAML {
		t.Fatalf("written proposal differs from rv connections propose YAML:\n%s", file)
	}
	if strings.Contains(string(file), "accepted:") || strings.Contains(string(file), "pin: IO") || strings.Contains(string(file), "pin: GPIO") {
		t.Fatalf("a GPIO was marked accepted:\n%s", file)
	}
	info, err := os.Stat(proposalPath)
	if err != nil {
		t.Fatal(err)
	}
	again := callPropose(t, ctx, session, dir, true, false)
	if again.Error == "" || !strings.Contains(again.Error, "already exists") || again.Path != "" {
		t.Fatalf("second write = path %q error %q", again.Path, again.Error)
	}
	assertBytesUnchanged(t, proposalPath, file)
	infoAfter, err := os.Stat(proposalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(info.ModTime()) || infoAfter.Size() != info.Size() {
		t.Fatal("propose without force rewrote the proposal")
	}
	forced := callPropose(t, ctx, session, dir, true, true)
	if forced.Error != "" || forced.Path != builtPath {
		t.Fatalf("force write path %q error %q", forced.Path, forced.Error)
	}
	assertBytesUnchanged(t, proposalPath, file)
	assertBytesUnchanged(t, contractsPath, contractsBefore)
	assertBytesUnchanged(t, schPath, schBefore)
	assertMCUChoice(t, forced)
}

func TestMCPApplyUnlistedCandidateWritesNothing(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	schPath := filepath.Join(root, "board.kicad_sch")
	proposalPath := filepath.Join(root, ".architon", "connections.proposal.yaml")
	schBefore := []byte(twoSDASchematic())
	proposalBefore := []byte(unlistedChoiceProposal)
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, proposalPath, string(proposalBefore))
	info, err := os.Stat(schPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx, session := startMCPSession(t)
	got := callApply(t, ctx, session, proposalPath)
	if got.Error == "" || !strings.Contains(got.Error, "not in candidates") {
		t.Fatalf("error = %q", got.Error)
	}
	if len(got.Files) != 0 || len(got.IDs) != 0 {
		t.Fatalf("rejected apply returned %+v", got)
	}
	assertBytesUnchanged(t, schPath, schBefore)
	assertBytesUnchanged(t, proposalPath, proposalBefore)
	infoAfter, err := os.Stat(schPath)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(info.ModTime()) || infoAfter.Size() != info.Size() {
		t.Fatal("apply rewrote the schematic")
	}
}

func TestMCPApplyListedCandidateAndSkipsNeedsChoice(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	schPath := filepath.Join(root, "board.kicad_sch")
	proposalPath := filepath.Join(root, ".architon", "connections.proposal.yaml")
	schBefore := []byte(twoSDASchematic())
	writeScanTestFile(t, schPath, string(schBefore))
	writeScanTestFile(t, proposalPath, strings.Replace(listedChoiceProposal, "status: accepted", "status: needs_choice", 1))

	ctx, session := startMCPSession(t)
	skipped := callApply(t, ctx, session, proposalPath)
	if skipped.Error != "" || len(skipped.Files) != 0 || len(skipped.IDs) != 0 {
		t.Fatalf("needs_choice was treated as accepted: %+v", skipped)
	}
	assertBytesUnchanged(t, schPath, schBefore)

	if err := os.WriteFile(proposalPath, []byte(listedChoiceProposal), 0o644); err != nil {
		t.Fatal(err)
	}
	applied := callApply(t, ctx, session, proposalPath)
	if applied.Error != "" {
		t.Fatalf("apply error: %s", applied.Error)
	}
	if len(applied.Files) != 1 || applied.Files[0] != schPath {
		t.Fatalf("files = %+v", applied.Files)
	}
	if len(applied.IDs) != 1 || applied.IDs[0] != "sda-U2" {
		t.Fatalf("ids = %+v", applied.IDs)
	}
	got, err := os.ReadFile(schPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(schBefore, got) || !strings.Contains(string(got), "(label \"I2C_SDA\"") {
		t.Fatalf("listed pin was not labeled:\n%s", got)
	}
	if strings.Contains(string(got), "(wire") || strings.Contains(string(got), "(no_connect") {
		t.Fatal("apply added a wire or no-connect")
	}
}

func TestMCPVerifyBrokenFixtureStillExits2(t *testing.T) {
	ctx, session := startMCPSession(t)
	broken := examplePath(t, filepath.Join("agent-loop", "broken"))
	got := callVerify(t, ctx, session, broken, "")
	if got.ExitCode != 2 {
		t.Fatalf("verify exit_code = %d, want 2; error=%s", got.ExitCode, got.Error)
	}
	if !strings.Contains(string(got.Scan), `"not_checked"`) {
		t.Fatal("verify result dropped not_checked")
	}
	assertBrokenFixtureFindings(t, got.Scan)
}

func startMCPSession(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
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
	return ctx, session
}

func callPropose(t *testing.T, ctx context.Context, session *mcp.ClientSession, projectPath string, write, force bool) proposeOutput {
	t.Helper()
	args := map[string]any{"project_path": projectPath}
	if write {
		args["write"] = true
	}
	if force {
		args["force"] = true
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      proposeToolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("call propose: %v", err)
	}
	if res.IsError {
		t.Fatalf("propose returned a tool error: %s", toolText(res))
	}
	out := decodeProposeOutput(t, res.StructuredContent)
	var fromText proposeOutput
	if err := json.Unmarshal([]byte(toolText(res)), &fromText); err != nil {
		t.Fatalf("propose text is not the result JSON: %v\n%s", err, toolText(res))
	}
	left, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(fromText)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalJSON(t, left) != canonicalJSON(t, right) {
		t.Fatalf("text result differs from structured result\ntext: %s\nstructured: %s", right, left)
	}
	assertNoExitCode(t, res.StructuredContent)
	return out
}

func callApply(t *testing.T, ctx context.Context, session *mcp.ClientSession, proposalPath string) applyOutput {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      applyToolName,
		Arguments: map[string]any{"proposal_path": proposalPath},
	})
	if err != nil {
		t.Fatalf("call apply: %v", err)
	}
	if res.IsError {
		t.Fatalf("apply returned a tool error: %s", toolText(res))
	}
	out := decodeApplyOutput(t, res.StructuredContent)
	var fromText applyOutput
	if err := json.Unmarshal([]byte(toolText(res)), &fromText); err != nil {
		t.Fatalf("apply text is not the result JSON: %v\n%s", err, toolText(res))
	}
	if applyCanon(t, out) != applyCanon(t, fromText) {
		t.Fatalf("text %+v != structured %+v", fromText, out)
	}
	assertNoExitCode(t, res.StructuredContent)
	return out
}

func assertMCUChoice(t *testing.T, got proposeOutput) {
	t.Helper()
	if len(got.Decided) != 1 || got.Decided[0].Status != "decided" {
		t.Fatalf("decided = %+v", got.Decided)
	}
	for _, part := range got.Decided[0].Parts {
		if part.Ref == "U1" || strings.HasPrefix(part.Pin, "IO") || strings.HasPrefix(part.Pin, "GPIO") {
			t.Fatalf("ESP32 pin was chosen: %+v", got.Decided[0].Parts)
		}
	}
	if len(got.Conflict) != 0 {
		t.Fatalf("conflict = %+v", got.Conflict)
	}
	mcu := 0
	var sda proposeEntry
	for _, entry := range got.NeedsChoice {
		if entry.Status != "needs_choice" || entry.Ref != "U1" || entry.MPN != "ESP32-WROOM-32" {
			t.Fatalf("choice = %+v", entry)
		}
		if len(entry.Candidates) < 2 {
			t.Fatalf("expected every ESP32 candidate, got %d", len(entry.Candidates))
		}
		for _, candidate := range entry.Candidates {
			if len(candidate.Pins) == 0 || candidate.Citation.Datasheet == "" {
				t.Fatalf("candidate = %+v", candidate)
			}
		}
		if entry.Signal == "SDA" {
			sda = entry
		}
		mcu++
	}
	if mcu != 2 || sda.Net != "I2C_SDA" {
		t.Fatalf("needs_choice for the MCU = %d, sda net %q", mcu, sda.Net)
	}
	raw := proposeCanon(t, got)
	for _, forbidden := range []string{`"status":"accepted"`, `"pin":"IO`, `"pin":"GPIO`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("a GPIO was marked accepted with %s:\n%s", forbidden, raw)
		}
	}
	found := false
	for _, citation := range got.Citations {
		if citation.Datasheet == "ESP32-WROOM-32 Datasheet" && citation.Revision != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("citations = %+v", got.Citations)
	}
}

func assertToolProperties(t *testing.T, schema any, required []string, optional []string) {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var parsed struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("schema: %v\n%s", err, data)
	}
	gotRequired := map[string]bool{}
	for _, name := range parsed.Required {
		gotRequired[name] = true
	}
	for _, name := range required {
		if _, ok := parsed.Properties[name]; !ok || !gotRequired[name] {
			t.Fatalf("schema missing required %s: %s", name, data)
		}
	}
	for _, name := range optional {
		if _, ok := parsed.Properties[name]; !ok || gotRequired[name] {
			t.Fatalf("schema should keep %s optional: %s", name, data)
		}
	}
	if len(parsed.Properties) != len(required)+len(optional) {
		t.Fatalf("schema properties = %s", data)
	}
}

func assertSchemaOmits(t *testing.T, schema any, name string) {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"`+name+`"`) {
		t.Fatalf("schema includes %s: %s", name, data)
	}
}

func assertNoExitCode(t *testing.T, structured any) {
	t.Helper()
	data, err := json.Marshal(structured)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["exit_code"]; ok {
		t.Fatalf("result includes exit_code: %s", data)
	}
}

func decodeProposeOutput(t *testing.T, structured any) proposeOutput {
	t.Helper()
	data, err := json.Marshal(structured)
	if err != nil {
		t.Fatal(err)
	}
	var out proposeOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("structured content: %v\n%s", err, data)
	}
	return out
}

func decodeApplyOutput(t *testing.T, structured any) applyOutput {
	t.Helper()
	data, err := json.Marshal(structured)
	if err != nil {
		t.Fatal(err)
	}
	var out applyOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("structured content: %v\n%s", err, data)
	}
	return out
}

func proposeCanon(t *testing.T, out proposeOutput) string {
	t.Helper()
	out.Path = ""
	out.Error = ""
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return canonicalJSON(t, data)
}

func applyCanon(t *testing.T, out applyOutput) string {
	t.Helper()
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return canonicalJSON(t, data)
}
