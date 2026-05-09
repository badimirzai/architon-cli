package graph

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/report"
)

func TestBuild_DistinguishesEdgeTypesDomainsAndInterfaces(t *testing.T) {
	design := &ir.DesignIR{
		Version: ir.SchemaVersion,
		Parts: []ir.Part{
			{Ref: "U1", Value: "Controller"},
			{Ref: "U2", Value: "Peripheral"},
		},
		Nets: []ir.Net{
			twoPinNet("I2C_SDA"),
			twoPinNet("SPI_MOSI"),
			twoPinNet("UART_TX"),
			twoPinNet("GPIO_4"),
			twoPinNet("+3V3"),
			twoPinNet("NET_UNCLASSIFIED"),
		},
	}

	graph := Build(BuildInput{
		RVVersion: "test",
		InputPath: "fixture.net",
		Design:    design,
	})

	tests := []struct {
		net        string
		wantType   string
		wantDomain string
		wantIface  string
	}{
		{net: "I2C_SDA", wantType: "i2c", wantDomain: "data", wantIface: "I2C_1"},
		{net: "SPI_MOSI", wantType: "spi", wantDomain: "data", wantIface: "SPI_1"},
		{net: "UART_TX", wantType: "uart", wantDomain: "data", wantIface: "UART_1"},
		{net: "GPIO_4", wantType: "gpio", wantDomain: "control", wantIface: "GPIO"},
		{net: "+3V3", wantType: "power", wantDomain: "power", wantIface: "+3V3"},
		{net: "NET_UNCLASSIFIED", wantType: "unknown", wantDomain: "unknown", wantIface: "unknown"},
	}

	for _, tt := range tests {
		edge := requireEdgeForNet(t, graph, tt.net)
		if edge.Type != tt.wantType || edge.Domain != tt.wantDomain || edge.Interface != tt.wantIface {
			t.Fatalf("edge %s classified as type=%q domain=%q interface=%q, want type=%q domain=%q interface=%q",
				tt.net, edge.Type, edge.Domain, edge.Interface, tt.wantType, tt.wantDomain, tt.wantIface)
		}
	}
}

func TestBuild_RollsWarningsOntoAffectedGraphElements(t *testing.T) {
	design := &ir.DesignIR{
		Version: ir.SchemaVersion,
		Parts: []ir.Part{
			{Ref: "U1", Value: "Controller"},
			{Ref: "U2", Value: "Peripheral"},
		},
		Nets: []ir.Net{twoPinNet("GPIO_4")},
	}

	graph := Build(BuildInput{
		RVVersion: "test",
		InputPath: "fixture.net",
		Design:    design,
		Report: report.VerificationReport{
			Findings: []report.RuleResult{
				{ID: "RULE_WARN", RuleID: "RULE_WARN", Severity: "WARN", Net: "GPIO_4", ComponentRef: "U1", Message: "test warning"},
			},
		},
	})

	edge := requireEdgeForNet(t, graph, "GPIO_4")
	if edge.Warnings != 1 || edge.Violations != 0 {
		t.Fatalf("expected warning rollup on GPIO edge, got %+v", edge)
	}
	if len(edge.FindingIDs) != 1 || edge.FindingIDs[0] != "RULE_WARN" {
		t.Fatalf("expected edge finding_ids to include RULE_WARN, got %+v", edge.FindingIDs)
	}
	link := graph.FindingsIndex["RULE_WARN"]
	if len(link.EdgeIDs) != 1 || link.EdgeIDs[0] != edge.ID {
		t.Fatalf("expected finding index to link edge %s, got %+v", edge.ID, link)
	}
	if node := requireNodeForID(t, graph, "U1"); node.Warnings != 1 || node.Violations != 0 {
		t.Fatalf("expected warning rollup on U1, got %+v", node)
	}
	if node := requireNodeForID(t, graph, "U2"); node.Warnings != 0 || node.Violations != 0 {
		t.Fatalf("expected warning not to roll up to unrelated U2, got %+v", node)
	}
}

func twoPinNet(name string) ir.Net {
	return ir.Net{
		Name: name,
		Pins: []ir.PinRef{
			{Ref: "U1", Pin: name + "_A", Name: name},
			{Ref: "U2", Pin: name + "_B", Name: name},
		},
	}
}

func requireEdgeForNet(t *testing.T, graph GraphIR, net string) Edge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Net == net {
			return edge
		}
	}
	t.Fatalf("missing edge for net %q in %+v", net, graph.Edges)
	return Edge{}
}

func requireNodeForID(t *testing.T, graph GraphIR, id string) Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing node %q in %+v", id, graph.Nodes)
	return Node{}
}
