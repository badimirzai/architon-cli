package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/internal/ui"
)

type graphCommandOutput struct {
	GraphVersion  string                       `json:"graph_version"`
	RVVersion     string                       `json:"rv_version"`
	InputPath     string                       `json:"input_path"`
	Summary       graphCommandSummary          `json:"summary"`
	Nodes         []graphCommandNode           `json:"nodes"`
	Edges         []graphCommandEdge           `json:"edges"`
	Rails         []graphCommandRail           `json:"rails"`
	Interfaces    []graphCommandInterface      `json:"interfaces"`
	Findings      []graphCommandFinding        `json:"findings"`
	FindingsIndex map[string]graphFindingLinks `json:"findings_index"`
}

type graphCommandSummary struct {
	Violations int `json:"violations"`
	Warnings   int `json:"warnings"`
	Infos      int `json:"infos"`
	Findings   int `json:"findings"`
}

type graphCommandNode struct {
	ID               string   `json:"id"`
	Ref              string   `json:"ref"`
	Label            string   `json:"label"`
	Type             string   `json:"type"`
	ContractCoverage string   `json:"contract_coverage"`
	Violations       int      `json:"violations"`
	Warnings         int      `json:"warnings"`
	FindingIDs       []string `json:"finding_ids"`
	Nets             []string `json:"nets"`
}

type graphCommandEdge struct {
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Net        string   `json:"net"`
	Type       string   `json:"type"`
	Domain     string   `json:"domain"`
	Interface  string   `json:"interface"`
	VoltageV   *float64 `json:"voltage_v"`
	Violations int      `json:"violations"`
	Warnings   int      `json:"warnings"`
	FindingIDs []string `json:"finding_ids"`
}

type graphCommandRail struct {
	Name       string   `json:"name"`
	VoltageV   *float64 `json:"voltage_v"`
	SourceRef  string   `json:"source_ref"`
	Consumers  []string `json:"consumers"`
	Violations int      `json:"violations"`
	Warnings   int      `json:"warnings"`
	FindingIDs []string `json:"finding_ids"`
}

type graphCommandInterface struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Nets         []string               `json:"nets"`
	Participants []string               `json:"participants"`
	Pullups      []graphInterfacePullup `json:"pullups"`
	Violations   int                    `json:"violations"`
	Warnings     int                    `json:"warnings"`
	FindingIDs   []string               `json:"finding_ids"`
}

type graphInterfacePullup struct {
	Ref            string  `json:"ref"`
	Net            string  `json:"net"`
	ResistanceOhms float64 `json:"resistance_ohms"`
	ToNet          string  `json:"to_net"`
}

type graphCommandFinding struct {
	ID             string `json:"id"`
	RuleID         string `json:"rule_id"`
	ContractID     string `json:"contract_id"`
	ContractSource string `json:"contract_source"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	ComponentRef   string `json:"component_ref"`
	Net            string `json:"net"`
	Pin            string `json:"pin"`
	Requirement    string `json:"requirement"`
	Fix            string `json:"fix"`
	Provenance     string `json:"provenance"`
}

type graphFindingLinks struct {
	NodeIDs      []string `json:"node_ids"`
	EdgeIDs      []string `json:"edge_ids"`
	RailNames    []string `json:"rail_names"`
	InterfaceIDs []string `json:"interface_ids"`
}

func runGraphCommand(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	ui.EnableColors(false)
	t.Cleanup(func() {
		ui.EnableColors(ui.DefaultColorEnabled())
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	cmd := newGraphCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout.String(), err
}

func parseGraphOutput(t *testing.T, stdout string) graphCommandOutput {
	t.Helper()
	var graph graphCommandOutput
	if err := json.Unmarshal([]byte(stdout), &graph); err != nil {
		t.Fatalf("graph output is not valid JSON: %v\n%s", err, stdout)
	}
	return graph
}

func TestGraphCommand_JSONGraphRollupsAndDeterminism(t *testing.T) {
	cwd := filepath.Dir(rootFixturePath(t, "esp32_overvoltage/netlist.net"))
	netlist := filepath.Base(rootFixturePath(t, "esp32_overvoltage/netlist.net"))
	meta := filepath.Base(rootFixturePath(t, "esp32_overvoltage/meta.yaml"))

	stdout, err := runGraphCommand(t, cwd, netlist, "--meta", meta, "--format", "json")
	if err != nil {
		t.Fatalf("expected graph command to succeed, got %v\n%s", err, stdout)
	}
	graph := parseGraphOutput(t, stdout)
	if graph.GraphVersion != "1" {
		t.Fatalf("expected graph_version 1, got %q", graph.GraphVersion)
	}
	if graph.RVVersion == "" {
		t.Fatalf("expected rv_version to be populated")
	}
	if graph.InputPath != netlist {
		t.Fatalf("expected input_path %q, got %q", netlist, graph.InputPath)
	}
	if len(graph.Nodes) == 0 {
		t.Fatalf("expected nodes to be generated")
	}
	if len(graph.Edges) == 0 {
		t.Fatalf("expected edges to be generated")
	}
	if len(graph.Rails) == 0 {
		t.Fatalf("expected rails to be generated")
	}
	if graph.Summary.Violations == 0 || graph.Summary.Findings == 0 {
		t.Fatalf("expected graph summary to include finding counts, got %+v", graph.Summary)
	}
	if len(graph.Findings) == 0 {
		t.Fatalf("expected graph findings to be embedded")
	}

	u1 := requireGraphNode(t, graph, "U1")
	if u1.Type != "mcu" {
		t.Fatalf("expected U1 to be classified as mcu, got %q", u1.Type)
	}
	if u1.ContractCoverage != "full" {
		t.Fatalf("expected U1 full contract coverage, got %q", u1.ContractCoverage)
	}
	if u1.Violations == 0 {
		t.Fatalf("expected U1 violation rollup")
	}
	if !containsString(u1.FindingIDs, "supply_abs_max") {
		t.Fatalf("expected U1 finding_ids to include supply_abs_max, got %+v", u1.FindingIDs)
	}

	powerEdge := requireGraphEdgeByNet(t, graph, "/+5V")
	if powerEdge.Type != "power" || powerEdge.Domain != "power" || powerEdge.Interface != "/+5V" {
		t.Fatalf("unexpected power edge classification: %+v", powerEdge)
	}
	if powerEdge.VoltageV == nil || *powerEdge.VoltageV != 5 {
		t.Fatalf("expected /+5V edge voltage 5V, got %+v", powerEdge.VoltageV)
	}
	if powerEdge.Violations == 0 || !containsString(powerEdge.FindingIDs, "supply_abs_max") {
		t.Fatalf("expected power edge violation and finding link, got %+v", powerEdge)
	}

	rail := requireGraphRail(t, graph, "/+5V")
	if rail.VoltageV == nil || *rail.VoltageV != 5 {
		t.Fatalf("expected /+5V rail voltage 5V, got %+v", rail.VoltageV)
	}
	if rail.Violations == 0 || !containsString(rail.FindingIDs, "supply_abs_max") {
		t.Fatalf("expected /+5V rail violation and finding link, got %+v", rail)
	}

	link, ok := graph.FindingsIndex["supply_abs_max"]
	if !ok {
		t.Fatalf("expected finding index entry for supply_abs_max")
	}
	if !containsString(link.NodeIDs, "U1") {
		t.Fatalf("expected finding to link to U1, got %+v", link)
	}
	if !containsString(link.EdgeIDs, powerEdge.ID) {
		t.Fatalf("expected finding to link to %s, got %+v", powerEdge.ID, link)
	}
	if !containsString(link.RailNames, "/+5V") {
		t.Fatalf("expected finding to link to /+5V rail, got %+v", link)
	}

	stdoutAgain, err := runGraphCommand(t, cwd, netlist, "--meta", meta, "--format", "json")
	if err != nil {
		t.Fatalf("expected repeated graph command to succeed, got %v\n%s", err, stdoutAgain)
	}
	if stdoutAgain != stdout {
		t.Fatalf("expected deterministic graph output across repeated runs\nfirst:\n%s\nsecond:\n%s", stdout, stdoutAgain)
	}
}

func TestGraphCommand_IncludesSameScanFindings(t *testing.T) {
	cwd := writeGraphPullupFixture(t, nil, false)

	scanStdout, scanErr := runScanCommand(t, cwd, "design.net", "--contracts", "contracts.yaml", "--format", "json", "--out", "scan.json")
	var exitErr *ExitError
	if !errors.As(scanErr, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected scan violation exit 2, got err=%v stdout=%s", scanErr, scanStdout)
	}
	var scan scanCIOutput
	if err := json.Unmarshal([]byte(scanStdout), &scan); err != nil {
		t.Fatalf("scan output is not valid JSON: %v\n%s", err, scanStdout)
	}
	if scan.Summary.Violations == 0 || len(scan.Findings) == 0 {
		t.Fatalf("expected scan pullup violations, got %+v", scan)
	}

	graphStdout, err := runGraphCommand(t, cwd, "design.net", "--contracts", "contracts.yaml", "--format", "json")
	if err != nil {
		t.Fatalf("expected graph command to succeed, got %v\n%s", err, graphStdout)
	}
	graph := parseGraphOutput(t, graphStdout)
	if graph.Summary.Violations != scan.Summary.Violations || graph.Summary.Findings != len(scan.Findings) {
		t.Fatalf("expected graph summary to mirror scan findings, scan=%+v graph=%+v", scan.Summary, graph.Summary)
	}
	for _, scanFinding := range scan.Findings {
		if !graphHasFinding(t, graph, scanFinding.ID, scanFinding.RuleID, scanFinding.Message) {
			t.Fatalf("expected graph findings to include scan finding id=%q rule=%q message=%q; graph findings=%+v", scanFinding.ID, scanFinding.RuleID, scanFinding.Message, graph.Findings)
		}
		if _, ok := graph.FindingsIndex[scanFinding.ID]; !ok {
			t.Fatalf("expected findings_index to include scan finding id %q, got %+v", scanFinding.ID, graph.FindingsIndex)
		}
	}
	if len(graph.FindingsIndex) == 0 {
		t.Fatalf("expected findings_index to be populated")
	}
}

func TestGraphCommand_PullupFindingRollups(t *testing.T) {
	t.Run("no pullups", func(t *testing.T) {
		graph := runPullupGraph(t, nil)
		if graph.Summary.Violations == 0 {
			t.Fatalf("expected no-pullup violations, got %+v", graph.Summary)
		}
		requireViolatingEdgeForNet(t, graph, "/I2C_SDA")
		requireViolatingEdgeForNet(t, graph, "/I2C_SCL")
		if iface := requireGraphInterface(t, graph, "I2C_MAIN"); iface.Violations == 0 {
			t.Fatalf("expected I2C interface violation rollup, got %+v", iface)
		}
	})

	t.Run("valid pullups", func(t *testing.T) {
		graph := runPullupGraph(t, []graphResistor{
			{Ref: "R1", Value: "4.7k", A: "/I2C_SDA", B: "/+3V3"},
			{Ref: "R2", Value: "4.7k", A: "/I2C_SCL", B: "/+3V3"},
		})
		requireNoGraphViolations(t, graph)
		if iface := requireGraphInterface(t, graph, "I2C_MAIN"); len(iface.Pullups) != 2 {
			t.Fatalf("expected interface pullup metadata for valid pullups, got %+v", iface.Pullups)
		}
	})

	t.Run("too strong pullups", func(t *testing.T) {
		graph := runPullupGraph(t, []graphResistor{
			{Ref: "R1", Value: "1k", A: "/I2C_SDA", B: "/+3V3"},
			{Ref: "R2", Value: "1k", A: "/I2C_SCL", B: "/+3V3"},
		})
		requireViolatingNode(t, graph, "R1")
		requireViolatingEdgeForNet(t, graph, "/I2C_SDA")
		if iface := requireGraphInterface(t, graph, "I2C_MAIN"); iface.Violations == 0 {
			t.Fatalf("expected I2C interface violation rollup, got %+v", iface)
		}
	})

	t.Run("too weak pullups", func(t *testing.T) {
		graph := runPullupGraph(t, []graphResistor{
			{Ref: "R1", Value: "20k", A: "/I2C_SDA", B: "/+3V3"},
			{Ref: "R2", Value: "20k", A: "/I2C_SCL", B: "/+3V3"},
		})
		requireViolatingNode(t, graph, "R1")
		requireViolatingEdgeForNet(t, graph, "/I2C_SDA")
		if iface := requireGraphInterface(t, graph, "I2C_MAIN"); iface.Violations == 0 {
			t.Fatalf("expected I2C interface violation rollup, got %+v", iface)
		}
	})

	t.Run("parallel pullups", func(t *testing.T) {
		graph := runPullupGraph(t, []graphResistor{
			{Ref: "R1", Value: "10k", A: "/I2C_SDA", B: "/+3V3"},
			{Ref: "R2", Value: "10k", A: "/I2C_SCL", B: "/+3V3"},
			{Ref: "R3", Value: "10k", A: "/I2C_SDA", B: "/+3V3"},
			{Ref: "R4", Value: "10k", A: "/I2C_SCL", B: "/+3V3"},
		})
		requireNoGraphViolations(t, graph)
	})

	t.Run("pulldown", func(t *testing.T) {
		graph := runPullupGraph(t, []graphResistor{
			{Ref: "R1", Value: "4.7k", A: "/I2C_SDA", B: "GND"},
			{Ref: "R2", Value: "4.7k", A: "/I2C_SCL", B: "GND"},
		})
		requireViolatingNode(t, graph, "R1")
		requireViolatingEdgeForNet(t, graph, "/I2C_SDA")
		if iface := requireGraphInterface(t, graph, "I2C_MAIN"); iface.Violations == 0 {
			t.Fatalf("expected I2C interface violation rollup, got %+v", iface)
		}
	})
}

func TestGraphCommand_DefaultContractsAndOutFlag(t *testing.T) {
	cwd := writeGraphPullupFixture(t, nil, true)

	stdout, err := runGraphCommand(t, cwd, ".", "--format", "json", "--out", "graph.json")
	if err != nil {
		t.Fatalf("expected graph command with --out to succeed, got %v\n%s", err, stdout)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("expected JSON stdout without ANSI escapes, got %q", stdout)
	}
	stdoutGraph := parseGraphOutput(t, stdout)
	if stdoutGraph.Summary.Violations == 0 || len(stdoutGraph.FindingsIndex) == 0 {
		t.Fatalf("expected default .architon/contracts.yaml to be loaded, got %+v", stdoutGraph)
	}
	fileData, err := os.ReadFile(filepath.Join(cwd, "graph.json"))
	if err != nil {
		t.Fatalf("read graph out file: %v", err)
	}
	if strings.Contains(string(fileData), "\x1b[") {
		t.Fatalf("expected JSON file without ANSI escapes, got %q", string(fileData))
	}
	fileGraph := parseGraphOutput(t, string(fileData))
	if string(fileData) != stdout {
		t.Fatalf("expected --out file to match stdout\nstdout:\n%s\nfile:\n%s", stdout, string(fileData))
	}
	if fileGraph.Summary != stdoutGraph.Summary {
		t.Fatalf("expected stdout/file graph summaries to match, stdout=%+v file=%+v", stdoutGraph.Summary, fileGraph.Summary)
	}
}

func TestGraphCommand_ContractsFlagOverridesArchitonContractsYAML(t *testing.T) {
	cwd := writeGraphPullupFixture(t, nil, true)
	writeScanTestFile(t, filepath.Join(cwd, "custom.yaml"), `contracts:
  - id: address_policy
    scope:
      bus_type: i2c
    require:
      no_i2c_address_conflict: true
    severity: error
`)

	stdout, err := runGraphCommand(t, cwd, ".", "--contracts", "custom.yaml", "--format", "json")
	if err != nil {
		t.Fatalf("expected graph command to succeed, got %v\n%s", err, stdout)
	}
	requireNoGraphViolations(t, parseGraphOutput(t, stdout))
}

func requireGraphNode(t *testing.T, graph graphCommandOutput, id string) graphCommandNode {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing graph node %q in %+v", id, graph.Nodes)
	return graphCommandNode{}
}

func requireGraphEdgeByNet(t *testing.T, graph graphCommandOutput, net string) graphCommandEdge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Net == net {
			return edge
		}
	}
	t.Fatalf("missing graph edge for net %q in %+v", net, graph.Edges)
	return graphCommandEdge{}
}

func requireGraphRail(t *testing.T, graph graphCommandOutput, name string) graphCommandRail {
	t.Helper()
	for _, rail := range graph.Rails {
		if rail.Name == name {
			return rail
		}
	}
	t.Fatalf("missing graph rail %q in %+v", name, graph.Rails)
	return graphCommandRail{}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type graphResistor struct {
	Ref   string
	Value string
	A     string
	B     string
}

func runPullupGraph(t *testing.T, resistors []graphResistor) graphCommandOutput {
	t.Helper()
	cwd := writeGraphPullupFixture(t, resistors, false)
	stdout, err := runGraphCommand(t, cwd, "design.net", "--contracts", "contracts.yaml", "--format", "json")
	if err != nil {
		t.Fatalf("expected graph command to succeed, got %v\n%s", err, stdout)
	}
	return parseGraphOutput(t, stdout)
}

func writeGraphPullupFixture(t *testing.T, resistors []graphResistor, defaultContracts bool) string {
	t.Helper()
	dir := t.TempDir()
	writeScanTestFile(t, filepath.Join(dir, "design.net"), graphPullupNetlist(resistors))
	contractsPath := filepath.Join(dir, "contracts.yaml")
	if defaultContracts {
		contractsPath = filepath.Join(dir, ".architon", "contracts.yaml")
	}
	writeScanTestFile(t, contractsPath, graphPullupContracts())
	return dir
}

func graphPullupContracts() string {
	return `contracts:
  - id: i2c_pullups
    description: Ensure I2C pullups are between 2.2k and 10k.
    scope:
      bus_type: i2c
      bus_id: i2c_main
      nets:
        sda: /I2C_SDA
        scl: /I2C_SCL
    require:
      pullup_ohms:
        min: 2200
        max: 10000
    severity: error
`
}

func graphPullupNetlist(resistors []graphResistor) string {
	components := strings.Builder{}
	for _, resistor := range resistors {
		components.WriteString(`    (comp (ref "` + resistor.Ref + `") (value "` + resistor.Value + `"))` + "\n")
	}
	netNodes := map[string][]string{
		"/I2C_SDA": {
			`      (node (ref "U1") (pin "1") (pinfunction "SDA"))`,
			`      (node (ref "U2") (pin "1") (pinfunction "SDA"))`,
		},
		"/I2C_SCL": {
			`      (node (ref "U1") (pin "2") (pinfunction "SCL"))`,
			`      (node (ref "U2") (pin "2") (pinfunction "SCL"))`,
		},
		"GND": {
			`      (node (ref "U1") (pin "3") (pinfunction "GND"))`,
			`      (node (ref "U2") (pin "3") (pinfunction "GND"))`,
		},
		"/+3V3": {
			`      (node (ref "U1") (pin "4") (pinfunction "VDD"))`,
			`      (node (ref "U2") (pin "4") (pinfunction "VDD"))`,
		},
	}
	for _, resistor := range resistors {
		netNodes[resistor.A] = append(netNodes[resistor.A], `      (node (ref "`+resistor.Ref+`") (pin "1") (pinfunction "1"))`)
		netNodes[resistor.B] = append(netNodes[resistor.B], `      (node (ref "`+resistor.Ref+`") (pin "2") (pinfunction "2"))`)
	}
	netNames := []string{"/I2C_SDA", "/I2C_SCL", "GND", "/+3V3"}
	nets := strings.Builder{}
	for i, netName := range netNames {
		nets.WriteString(`    (net (code "` + string(rune('1'+i)) + `") (name "` + netName + `")` + "\n")
		nets.WriteString(strings.Join(netNodes[netName], "\n"))
		nets.WriteString(")\n")
	}
	return `(export (version "E")
  (design (source "pullup_fixture.kicad_sch"))
  (components
    (comp (ref "U1") (value "MCU_A")
      (fields (field (name "i2c_address") "0x10")))
    (comp (ref "U2") (value "IMU_B")
      (fields (field (name "i2c_address") "0x11")))
` + components.String() + `  )
  (libparts)
  (nets
` + nets.String() + `  ))
`
}

func graphHasFinding(t *testing.T, graph graphCommandOutput, id string, ruleID string, message string) bool {
	t.Helper()
	for _, finding := range graph.Findings {
		if finding.ID == id && finding.RuleID == ruleID && finding.Message == message {
			return true
		}
	}
	return false
}

func requireViolatingEdgeForNet(t *testing.T, graph graphCommandOutput, net string) graphCommandEdge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Net == net && edge.Violations > 0 && len(edge.FindingIDs) > 0 {
			return edge
		}
	}
	t.Fatalf("expected violating edge for net %s, got %+v", net, graph.Edges)
	return graphCommandEdge{}
}

func requireViolatingNode(t *testing.T, graph graphCommandOutput, id string) graphCommandNode {
	t.Helper()
	node := requireGraphNode(t, graph, id)
	if node.Violations == 0 {
		t.Fatalf("expected node %s to have violation rollup, got %+v", id, node)
	}
	return node
}

func requireGraphInterface(t *testing.T, graph graphCommandOutput, id string) graphCommandInterface {
	t.Helper()
	for _, iface := range graph.Interfaces {
		if iface.ID == id {
			return iface
		}
	}
	t.Fatalf("missing graph interface %q in %+v", id, graph.Interfaces)
	return graphCommandInterface{}
}

func requireNoGraphViolations(t *testing.T, graph graphCommandOutput) {
	t.Helper()
	if graph.Summary.Violations != 0 || len(graph.FindingsIndex) != 0 || len(graph.Findings) != 0 {
		t.Fatalf("expected no graph violations/findings, got summary=%+v findings=%+v index=%+v", graph.Summary, graph.Findings, graph.FindingsIndex)
	}
}
