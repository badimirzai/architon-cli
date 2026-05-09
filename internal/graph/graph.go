package graph

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/infer"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/report"
)

const SchemaVersion = "1"

type GraphIR struct {
	GraphVersion  string                 `json:"graph_version"`
	RVVersion     string                 `json:"rv_version"`
	InputPath     string                 `json:"input_path"`
	Summary       Summary                `json:"summary"`
	Nodes         []Node                 `json:"nodes"`
	Edges         []Edge                 `json:"edges"`
	Rails         []Rail                 `json:"rails"`
	Interfaces    []Interface            `json:"interfaces"`
	Findings      []Finding              `json:"findings"`
	FindingsIndex map[string]FindingLink `json:"findings_index"`
}

type Summary struct {
	Violations             int  `json:"violations"`
	Warnings               int  `json:"warnings"`
	Infos                  int  `json:"infos"`
	Findings               int  `json:"findings"`
	HasFailures            bool `json:"has_failures"`
	UserContractsLoaded    int  `json:"user_contracts_loaded"`
	BuiltInContractsLoaded int  `json:"built_in_contracts_loaded"`
	ActiveUserRequirements int  `json:"active_user_requirements"`
}

type Node struct {
	ID               string       `json:"id"`
	Ref              string       `json:"ref"`
	Label            string       `json:"label"`
	Type             string       `json:"type"`
	ContractCoverage string       `json:"contract_coverage"`
	Violations       int          `json:"violations"`
	Warnings         int          `json:"warnings"`
	FindingIDs       []string     `json:"finding_ids"`
	Nets             []string     `json:"nets"`
	Metadata         NodeMetadata `json:"metadata"`
}

type NodeMetadata struct {
	Value        string `json:"value"`
	Footprint    string `json:"footprint"`
	MPN          string `json:"mpn"`
	Manufacturer string `json:"manufacturer"`
}

type Edge struct {
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Net        string   `json:"net"`
	Type       string   `json:"type"`
	Domain     string   `json:"domain"`
	Interface  string   `json:"interface"`
	VoltageV   *float64 `json:"voltage_v,omitempty"`
	Violations int      `json:"violations"`
	Warnings   int      `json:"warnings"`
	FindingIDs []string `json:"finding_ids"`
}

type Rail struct {
	Name       string   `json:"name"`
	VoltageV   *float64 `json:"voltage_v,omitempty"`
	SourceRef  string   `json:"source_ref"`
	Consumers  []string `json:"consumers"`
	Violations int      `json:"violations"`
	Warnings   int      `json:"warnings"`
	FindingIDs []string `json:"finding_ids"`
}

type Interface struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Nets         []string          `json:"nets"`
	Participants []string          `json:"participants"`
	Pullups      []InterfacePullup `json:"pullups"`
	Violations   int               `json:"violations"`
	Warnings     int               `json:"warnings"`
	FindingIDs   []string          `json:"finding_ids"`
}

type InterfacePullup struct {
	Ref            string  `json:"ref"`
	Net            string  `json:"net"`
	ResistanceOhms float64 `json:"resistance_ohms"`
	ToNet          string  `json:"to_net"`
}

type Finding struct {
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
	WhyThisMatters string `json:"why_this_matters,omitempty"`
	Provenance     string `json:"provenance"`
}

type FindingLink struct {
	NodeIDs      []string `json:"node_ids"`
	EdgeIDs      []string `json:"edge_ids"`
	RailNames    []string `json:"rail_names"`
	InterfaceIDs []string `json:"interface_ids,omitempty"`
}

type BuildInput struct {
	RVVersion  string
	InputPath  string
	Design     *ir.DesignIR
	Report     report.VerificationReport
	ContractIR *contracts.ContractIR
}

type builder struct {
	input          BuildInput
	design         *ir.DesignIR
	partByRef      map[string]ir.Part
	netByName      map[string]ir.Net
	netsByRef      map[string][]string
	pinsByRef      map[string][]ir.PinRef
	voltageByNet   map[string]float64
	voltageSource  map[string]string
	interfaceByNet map[string]string
	findings       []findingRecord
	nodes          []Node
	edges          []Edge
	rails          []Rail
	interfaces     []Interface
	nodeIndex      map[string]int
	edgeIndex      map[string]int
	railIndex      map[string]int
	interfaceIndex map[string]int
}

type netClass struct {
	Type      string
	Domain    string
	Interface string
}

type netGroup struct {
	net string
	key string
}

type findingRecord struct {
	id      string
	raw     report.RuleResult
	finding Finding
}

func Build(input BuildInput) GraphIR {
	design := input.Design
	if design == nil {
		design = &ir.DesignIR{Version: ir.SchemaVersion}
	}
	b := &builder{
		input:          input,
		design:         design,
		partByRef:      map[string]ir.Part{},
		netByName:      map[string]ir.Net{},
		netsByRef:      map[string][]string{},
		pinsByRef:      map[string][]ir.PinRef{},
		voltageByNet:   map[string]float64{},
		voltageSource:  map[string]string{},
		interfaceByNet: map[string]string{},
		nodeIndex:      map[string]int{},
		edgeIndex:      map[string]int{},
		railIndex:      map[string]int{},
		interfaceIndex: map[string]int{},
	}
	b.indexDesign()
	b.indexVoltages()
	b.interfaceByNet = buildInterfaceIndex(design, input.ContractIR, input.Report)
	b.findings = buildFindingRecords(input.Report)
	b.nodes = b.buildNodes()
	b.edges = b.buildEdges()
	b.rails = b.buildRails()
	b.interfaces = b.buildInterfaces()
	findingsIndex := b.attachFindings()
	findings := b.graphFindings()
	summary := summarizeFindings(findings)
	summary.HasFailures = input.Report.Summary.HasFailures || summary.Violations > 0
	summary.UserContractsLoaded = input.Report.Summary.UserContractsLoaded
	summary.BuiltInContractsLoaded = input.Report.Summary.BuiltInContractsLoaded
	summary.ActiveUserRequirements = input.Report.Summary.ActiveUserRequirements

	return GraphIR{
		GraphVersion:  SchemaVersion,
		RVVersion:     strings.TrimSpace(input.RVVersion),
		InputPath:     input.InputPath,
		Summary:       summary,
		Nodes:         b.nodes,
		Edges:         b.edges,
		Rails:         b.rails,
		Interfaces:    b.interfaces,
		Findings:      findings,
		FindingsIndex: findingsIndex,
	}
}

func RenderJSON(graph GraphIR) ([]byte, error) {
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func buildFindingRecords(result report.VerificationReport) []findingRecord {
	result = report.CanonicalizeVerificationReport(result)
	out := make([]findingRecord, 0, len(result.Findings))
	for _, finding := range result.Findings {
		id := strings.TrimSpace(finding.ID)
		if id == "" {
			id = sanitizeIDPart(firstNonEmpty(finding.RuleID, "finding"))
		}
		out = append(out, findingRecord{
			id:      id,
			raw:     finding,
			finding: buildGraphFinding(id, finding),
		})
	}
	return out
}

func buildGraphFinding(id string, finding report.RuleResult) Finding {
	ruleID := strings.TrimSpace(finding.RuleID)
	if ruleID == "" {
		ruleID = strings.TrimSpace(finding.ID)
	}
	if ruleID == "" {
		ruleID = id
	}

	componentRef := strings.TrimSpace(finding.ComponentRef)
	if componentRef == "" {
		componentRef = strings.TrimSpace(finding.Ref)
	}

	return Finding{
		ID:             id,
		RuleID:         ruleID,
		ContractID:     findingContractID(finding, ruleID),
		ContractSource: findingContractSource(finding),
		Severity:       normalizeSeverity(finding.Severity),
		Message:        strings.TrimSpace(finding.Message),
		ComponentRef:   componentRef,
		Net:            strings.TrimSpace(finding.Net),
		Pin:            strings.TrimSpace(finding.Pin),
		Requirement:    findingRequirement(finding, ruleID),
		Fix:            strings.TrimSpace(finding.Fix),
		WhyThisMatters: strings.TrimSpace(finding.WhyThisMatters),
		Provenance:     findingProvenance(finding),
	}
}

func (b *builder) graphFindings() []Finding {
	out := make([]Finding, 0, len(b.findings))
	for _, record := range b.findings {
		out = append(out, record.finding)
	}
	return out
}

func summarizeFindings(findings []Finding) Summary {
	summary := Summary{Findings: len(findings)}
	for _, finding := range findings {
		switch normalizeSeverity(finding.Severity) {
		case "ERROR":
			summary.Violations++
		case "WARN":
			summary.Warnings++
		case "INFO":
			summary.Infos++
		}
	}
	return summary
}

func (b *builder) indexDesign() {
	for _, part := range b.design.Parts {
		ref := strings.TrimSpace(part.Ref)
		if ref == "" {
			continue
		}
		part.Ref = ref
		b.partByRef[ref] = part
	}
	for _, net := range b.design.Nets {
		netName := strings.TrimSpace(net.Name)
		if netName == "" {
			continue
		}
		net.Name = netName
		pins := append([]ir.PinRef(nil), net.Pins...)
		sort.Slice(pins, func(i, j int) bool {
			if pins[i].Ref != pins[j].Ref {
				return pins[i].Ref < pins[j].Ref
			}
			if pins[i].Pin != pins[j].Pin {
				return pins[i].Pin < pins[j].Pin
			}
			return pins[i].Name < pins[j].Name
		})
		net.Pins = pins
		b.netByName[netName] = net
		for _, pin := range pins {
			ref := strings.TrimSpace(pin.Ref)
			if ref == "" {
				continue
			}
			if _, ok := b.partByRef[ref]; !ok {
				b.partByRef[ref] = ir.Part{Ref: ref}
			}
			b.netsByRef[ref] = appendUnique(b.netsByRef[ref], netName)
			b.pinsByRef[ref] = append(b.pinsByRef[ref], ir.PinRef{Ref: ref, Pin: strings.TrimSpace(pin.Pin), Name: strings.TrimSpace(pin.Name)})
		}
	}
	for ref := range b.netsByRef {
		sort.Strings(b.netsByRef[ref])
	}
	for ref := range b.pinsByRef {
		sort.Slice(b.pinsByRef[ref], func(i, j int) bool {
			if b.pinsByRef[ref][i].Pin != b.pinsByRef[ref][j].Pin {
				return b.pinsByRef[ref][i].Pin < b.pinsByRef[ref][j].Pin
			}
			return b.pinsByRef[ref][i].Name < b.pinsByRef[ref][j].Name
		})
	}
}

func (b *builder) indexVoltages() {
	set := func(net string, voltage float64, source string, overwrite bool) {
		net = strings.TrimSpace(net)
		if net == "" {
			return
		}
		if _, exists := b.voltageByNet[net]; exists && !overwrite {
			return
		}
		b.voltageByNet[net] = voltage
		b.voltageSource[net] = strings.TrimSpace(source)
	}
	if b.input.Report.Derived != nil {
		for _, inferred := range b.input.Report.Derived.InferredNetVoltages {
			set(inferred.Net, inferred.Voltage, inferred.Source, false)
		}
		for _, inference := range b.input.Report.Derived.RailInferences {
			if inference.Voltage == nil {
				continue
			}
			set(inference.NetName, *inference.Voltage, inference.Source, false)
		}
		for _, netVoltage := range b.input.Report.Derived.NetVoltages {
			set(netVoltage.Net, netVoltage.Voltage, netVoltage.Source, true)
		}
	}
	for _, net := range b.design.Nets {
		if infer.IsGroundNetName(net.Name) {
			set(net.Name, 0, "ground", false)
		}
	}
}

func (b *builder) buildNodes() []Node {
	refs := make([]string, 0, len(b.partByRef))
	for ref := range b.partByRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	nodes := make([]Node, 0, len(refs))
	for _, ref := range refs {
		part := b.partByRef[ref]
		node := Node{
			ID:               ref,
			Ref:              ref,
			Label:            nodeLabel(part),
			Type:             classifyNode(part, b.input.ContractIR),
			ContractCoverage: componentCoverage(ref, b.pinsByRef[ref], b.input.ContractIR),
			FindingIDs:       []string{},
			Nets:             append([]string{}, b.netsByRef[ref]...),
			Metadata: NodeMetadata{
				Value:        strings.TrimSpace(part.Value),
				Footprint:    strings.TrimSpace(part.Footprint),
				MPN:          firstNonEmpty(part.MPN, fieldValue(part.Fields, "mpn", "manufacturerpartnumber", "partnumber", "mfrpartnumber", "mfrpn", "pn")),
				Manufacturer: firstNonEmpty(part.Manufacturer, fieldValue(part.Fields, "manufacturer", "mfr", "vendor")),
			},
		}
		b.nodeIndex[node.ID] = len(nodes)
		nodes = append(nodes, node)
	}
	return nodes
}

func (b *builder) buildEdges() []Edge {
	netNames := make([]string, 0, len(b.netByName))
	for netName := range b.netByName {
		netNames = append(netNames, netName)
	}
	sort.Strings(netNames)
	usedIDs := map[string]int{}
	edges := make([]Edge, 0)
	for _, netName := range netNames {
		net := b.netByName[netName]
		refs := uniqueRefsForNet(net)
		if len(refs) < 2 {
			continue
		}
		class := b.classifyNet(net)
		voltage := b.edgeVoltage(net.Name)
		for i := 0; i < len(refs); i++ {
			for j := i + 1; j < len(refs); j++ {
				source := refs[i]
				target := refs[j]
				id := uniqueGraphID("edge_"+sanitizeIDPart(source)+"_"+sanitizeIDPart(target)+"_"+sanitizeIDPart(net.Name), usedIDs)
				edge := Edge{
					ID:         id,
					Source:     source,
					Target:     target,
					Net:        net.Name,
					Type:       class.Type,
					Domain:     class.Domain,
					Interface:  class.Interface,
					VoltageV:   voltage,
					FindingIDs: []string{},
				}
				edges = append(edges, edge)
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	for i, edge := range edges {
		b.edgeIndex[edge.ID] = i
	}
	return edges
}

func (b *builder) buildRails() []Rail {
	netNames := make([]string, 0, len(b.voltageByNet))
	for netName := range b.voltageByNet {
		if _, ok := b.netByName[netName]; !ok {
			continue
		}
		if !b.isRailNet(netName) {
			continue
		}
		netNames = append(netNames, netName)
	}
	sort.Strings(netNames)
	rails := make([]Rail, 0, len(netNames))
	for _, netName := range netNames {
		net := b.netByName[netName]
		voltage := b.voltageByNet[netName]
		sourceRef := b.sourceRefForRail(net)
		consumers := uniqueRefsForNet(net)
		if sourceRef != "" {
			consumers = removeString(consumers, sourceRef)
		}
		rail := Rail{
			Name:       netName,
			VoltageV:   &voltage,
			SourceRef:  sourceRef,
			Consumers:  consumers,
			FindingIDs: []string{},
		}
		b.railIndex[rail.Name] = len(rails)
		rails = append(rails, rail)
	}
	return rails
}

func (b *builder) buildInterfaces() []Interface {
	type ifaceBuilder struct {
		id           string
		kind         string
		nets         map[string]struct{}
		participants map[string]struct{}
		pullups      map[string]InterfacePullup
	}
	byID := map[string]*ifaceBuilder{}
	get := func(id string, kind string) *ifaceBuilder {
		id = strings.TrimSpace(id)
		if id == "" || id == "unknown" {
			return nil
		}
		if kind != "i2c" && kind != "spi" && kind != "uart" {
			return nil
		}
		if byID[id] == nil {
			byID[id] = &ifaceBuilder{
				id:           id,
				kind:         kind,
				nets:         map[string]struct{}{},
				participants: map[string]struct{}{},
				pullups:      map[string]InterfacePullup{},
			}
		}
		return byID[id]
	}
	for _, edge := range b.edges {
		builder := get(edge.Interface, edge.Type)
		if builder == nil {
			continue
		}
		builder.nets[edge.Net] = struct{}{}
		builder.participants[edge.Source] = struct{}{}
		builder.participants[edge.Target] = struct{}{}
	}
	for _, builder := range byID {
		for _, pullup := range b.pullupsForInterface(builder.nets) {
			builder.pullups[pullup.Ref+"\x00"+pullup.Net+"\x00"+pullup.ToNet] = pullup
			delete(builder.participants, pullup.Ref)
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Interface, 0, len(ids))
	for _, id := range ids {
		builder := byID[id]
		pullups := make([]InterfacePullup, 0, len(builder.pullups))
		for _, pullup := range builder.pullups {
			pullups = append(pullups, pullup)
		}
		sort.Slice(pullups, func(i, j int) bool {
			if pullups[i].Ref != pullups[j].Ref {
				return pullups[i].Ref < pullups[j].Ref
			}
			if pullups[i].Net != pullups[j].Net {
				return pullups[i].Net < pullups[j].Net
			}
			return pullups[i].ToNet < pullups[j].ToNet
		})
		iface := Interface{
			ID:           id,
			Type:         builder.kind,
			Nets:         sortedSet(builder.nets),
			Participants: sortedSet(builder.participants),
			Pullups:      pullups,
			FindingIDs:   []string{},
		}
		b.interfaceIndex[iface.ID] = len(out)
		out = append(out, iface)
	}
	return out
}

func (b *builder) pullupsForInterface(nets map[string]struct{}) []InterfacePullup {
	out := []InterfacePullup{}
	for ref := range b.pinsByRef {
		part := b.partByRef[ref]
		if classifyNode(part, b.input.ContractIR) != "passive" || !strings.HasPrefix(strings.ToUpper(ref), "R") {
			continue
		}
		resistance, ok := parseResistanceOhms(firstNonEmpty(part.Value, fieldValue(part.Fields, "resistance_ohms", "resistor_ohms", "ohms", "pullup_ohms")))
		if !ok || resistance <= 0 {
			continue
		}
		connectedNets := b.netsByRef[ref]
		for _, signal := range connectedNets {
			if _, ok := nets[signal]; !ok {
				continue
			}
			for _, toNet := range connectedNets {
				if toNet == signal || infer.IsGroundNetName(toNet) || !b.isRailNet(toNet) {
					continue
				}
				out = append(out, InterfacePullup{
					Ref:            ref,
					Net:            signal,
					ResistanceOhms: resistance,
					ToNet:          toNet,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref != out[j].Ref {
			return out[i].Ref < out[j].Ref
		}
		if out[i].Net != out[j].Net {
			return out[i].Net < out[j].Net
		}
		return out[i].ToNet < out[j].ToNet
	})
	return out
}

func (b *builder) classifyNet(net ir.Net) netClass {
	if iface, ok := b.interfaceByNet[net.Name]; ok && strings.HasPrefix(iface, "I2C") {
		return netClass{Type: "i2c", Domain: "data", Interface: iface}
	}
	if b.netHasRole(net, contracts.RoleI2CSDA, contracts.RoleI2CSCL) || netHasAnyToken(net, "I2C", "SDA", "SCL") {
		return netClass{Type: "i2c", Domain: "data", Interface: interfaceOrDefault(b.interfaceByNet[net.Name], "I2C_1")}
	}
	if b.netHasRole(net, contracts.RoleSPI) || netHasAnyToken(net, "SPI", "MISO", "MOSI", "SCK", "SCLK", "NSS", "CS", "CSN") {
		return netClass{Type: "spi", Domain: "data", Interface: interfaceOrDefault(b.interfaceByNet[net.Name], "SPI_1")}
	}
	if b.netHasRole(net, contracts.RoleUART) || netHasAnyToken(net, "UART", "USART", "TX", "RX", "RTS", "CTS") {
		return netClass{Type: "uart", Domain: "data", Interface: interfaceOrDefault(b.interfaceByNet[net.Name], "UART_1")}
	}
	if b.isRailNet(net.Name) || b.netHasRole(net, contracts.RolePowerIn, contracts.RolePowerOut, contracts.RoleRegulatorOut, contracts.RoleSource, contracts.RoleGround) {
		return netClass{Type: "power", Domain: "power", Interface: net.Name}
	}
	if b.netHasRole(net, contracts.RoleMotorOut) || netHasAnyToken(net, "MOTOR", "MOT", "OUTA", "OUTB", "AOUT", "BOUT") {
		return netClass{Type: "motor", Domain: "control", Interface: "MOTOR"}
	}
	if netHasAnyToken(net, "ADC", "DAC", "AIN", "AOUT", "ANALOG") {
		return netClass{Type: "analog", Domain: "data", Interface: "ANALOG"}
	}
	if b.netHasRole(net, contracts.RoleGPIO) || netHasAnyToken(net, "GPIO") {
		return netClass{Type: "gpio", Domain: "control", Interface: "GPIO"}
	}
	return netClass{Type: "unknown", Domain: "unknown", Interface: "unknown"}
}

func (b *builder) edgeVoltage(net string) *float64 {
	voltage, ok := b.voltageByNet[strings.TrimSpace(net)]
	if !ok {
		return nil
	}
	return &voltage
}

func (b *builder) isRailNet(netName string) bool {
	netName = strings.TrimSpace(netName)
	if netName == "" {
		return false
	}
	if _, ok := b.voltageByNet[netName]; ok {
		return true
	}
	if infer.IsGroundNetName(netName) {
		return true
	}
	return looksLikePowerNetName(netName)
}

func (b *builder) netHasRole(net ir.Net, roles ...contracts.PinRole) bool {
	if b.input.ContractIR == nil {
		return false
	}
	wanted := map[contracts.PinRole]struct{}{}
	for _, role := range roles {
		wanted[role] = struct{}{}
	}
	for _, pin := range net.Pins {
		contract, ok := b.input.ContractIR.Pin(pin.Ref, pin.Pin)
		if !ok {
			continue
		}
		if _, ok := wanted[contract.Role]; ok {
			return true
		}
	}
	return false
}

func (b *builder) sourceRefForRail(net ir.Net) string {
	if source := strings.TrimSpace(b.voltageSource[net.Name]); strings.HasPrefix(source, "regulator:") {
		ref := strings.TrimSpace(strings.TrimPrefix(source, "regulator:"))
		if ref != "" {
			return ref
		}
	}
	if b.input.ContractIR != nil {
		for _, pin := range net.Pins {
			contract, ok := b.input.ContractIR.Pin(pin.Ref, pin.Pin)
			if !ok {
				continue
			}
			switch contract.Role {
			case contracts.RoleSource, contracts.RolePowerOut, contracts.RoleRegulatorOut:
				return pin.Ref
			}
		}
	}
	refs := uniqueRefsForNet(net)
	for _, ref := range refs {
		if classifyNode(b.partByRef[ref], b.input.ContractIR) == "power_source" {
			return ref
		}
	}
	return ""
}

func (b *builder) attachFindings() map[string]FindingLink {
	index := map[string]FindingLink{}
	for _, record := range b.findings {
		finding := record.raw
		findingID := record.id
		nets := b.affectedNets(finding)
		refs := b.affectedRefs(finding, nets)
		interfaceIDs := b.affectedInterfaceIDs(finding, nets)
		link := FindingLink{
			NodeIDs:      sortedSet(refs),
			EdgeIDs:      b.affectedEdgeIDs(nets, refs, interfaceIDs, finding),
			RailNames:    b.affectedRailNames(nets),
			InterfaceIDs: interfaceIDs,
		}
		index[findingID] = link
		severity := normalizeSeverity(finding.Severity)
		for _, nodeID := range link.NodeIDs {
			if idx, ok := b.nodeIndex[nodeID]; ok {
				incrementSeverity(severity, &b.nodes[idx].Violations, &b.nodes[idx].Warnings)
				b.nodes[idx].FindingIDs = appendUnique(b.nodes[idx].FindingIDs, findingID)
			}
		}
		for _, edgeID := range link.EdgeIDs {
			if idx, ok := b.edgeIndex[edgeID]; ok {
				incrementSeverity(severity, &b.edges[idx].Violations, &b.edges[idx].Warnings)
				b.edges[idx].FindingIDs = appendUnique(b.edges[idx].FindingIDs, findingID)
			}
		}
		for _, railName := range link.RailNames {
			if idx, ok := b.railIndex[railName]; ok {
				incrementSeverity(severity, &b.rails[idx].Violations, &b.rails[idx].Warnings)
				b.rails[idx].FindingIDs = appendUnique(b.rails[idx].FindingIDs, findingID)
			}
		}
		for _, interfaceID := range link.InterfaceIDs {
			if idx, ok := b.interfaceIndex[interfaceID]; ok {
				incrementSeverity(severity, &b.interfaces[idx].Violations, &b.interfaces[idx].Warnings)
				b.interfaces[idx].FindingIDs = appendUnique(b.interfaces[idx].FindingIDs, findingID)
			}
		}
	}
	b.sortFindingIDs()
	return index
}

func (b *builder) affectedNets(finding report.RuleResult) []string {
	nets := []string{}
	add := func(net string) {
		net = strings.TrimSpace(net)
		if net != "" {
			nets = appendUnique(nets, net)
		}
	}
	add(finding.Net)
	if strings.TrimSpace(finding.Net) == "" && finding.BusNets != nil {
		add(finding.BusNets.SDA)
		add(finding.BusNets.SCL)
	}
	if finding.Inference != nil {
		add(finding.Inference.NetName)
	}
	add(conflictNetName(finding.Message))
	for netName := range b.netByName {
		if containsExactText(finding.Message, netName) {
			add(netName)
		}
	}
	sort.Strings(nets)
	return nets
}

func (b *builder) affectedRefs(finding report.RuleResult, nets []string) map[string]struct{} {
	refs := map[string]struct{}{}
	addRef := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		if _, ok := b.partByRef[ref]; ok {
			refs[ref] = struct{}{}
		}
	}
	addRef(finding.Ref)
	addRef(finding.ComponentRef)
	for _, ref := range finding.PullupResistors {
		addRef(ref)
	}
	for _, text := range []string{finding.Provider, finding.Consumer, finding.Message} {
		for _, ref := range refsMentioned(text, b.partByRef) {
			addRef(ref)
		}
	}
	return refs
}

func (b *builder) affectedEdgeIDs(nets []string, refs map[string]struct{}, interfaceIDs []string, finding report.RuleResult) []string {
	netSet := exactStringSet(nets)
	interfaceSet := exactStringSet(interfaceIDs)
	includeInterfaceEdges := isBusLevelFinding(finding, nets)
	out := []string{}
	for _, edge := range b.edges {
		_, netMatch := netSet[edge.Net]
		_, interfaceMatch := interfaceSet[edge.Interface]
		if !netMatch && !(includeInterfaceEdges && interfaceMatch) {
			continue
		}
		if netMatch && len(refs) > 0 {
			_, source := refs[edge.Source]
			_, target := refs[edge.Target]
			if !source && !target {
				continue
			}
		}
		out = append(out, edge.ID)
	}
	sort.Strings(out)
	return out
}

func (b *builder) affectedInterfaceIDs(finding report.RuleResult, nets []string) []string {
	out := []string{}
	if finding.BusNets != nil {
		for _, net := range []string{finding.BusNets.SDA, finding.BusNets.SCL} {
			if iface := strings.TrimSpace(b.interfaceByNet[net]); iface != "" {
				out = appendUnique(out, iface)
			}
		}
	}
	for _, net := range nets {
		if iface := strings.TrimSpace(b.interfaceByNet[net]); iface != "" {
			out = appendUnique(out, iface)
		}
	}
	if strings.EqualFold(finding.BusType, "i2c") && strings.TrimSpace(finding.BusID) != "" {
		iface := normalizeInterfaceID("I2C", finding.BusID)
		if _, ok := b.interfaceIndex[iface]; ok {
			out = appendUnique(out, iface)
		}
	}
	sort.Strings(out)
	return out
}

func isBusLevelFinding(finding report.RuleResult, nets []string) bool {
	if !strings.EqualFold(finding.BusType, "i2c") && finding.BusNets == nil {
		return false
	}
	return strings.TrimSpace(finding.Net) == "" && len(nets) > 0
}

func (b *builder) affectedRailNames(nets []string) []string {
	out := []string{}
	for _, net := range nets {
		if _, ok := b.railIndex[net]; ok {
			out = appendUnique(out, net)
		}
	}
	sort.Strings(out)
	return out
}

func (b *builder) sortFindingIDs() {
	for i := range b.nodes {
		sort.Strings(b.nodes[i].FindingIDs)
	}
	for i := range b.edges {
		sort.Strings(b.edges[i].FindingIDs)
	}
	for i := range b.rails {
		sort.Strings(b.rails[i].FindingIDs)
	}
	for i := range b.interfaces {
		sort.Strings(b.interfaces[i].FindingIDs)
	}
}

func buildInterfaceIndex(design *ir.DesignIR, contractIR *contracts.ContractIR, result report.VerificationReport) map[string]string {
	out := map[string]string{}
	addI2C := func(busID string, nets *contracts.I2CBusNets) {
		if nets == nil {
			return
		}
		iface := normalizeInterfaceID("I2C", busID)
		if strings.TrimSpace(nets.SDA) != "" {
			out[strings.TrimSpace(nets.SDA)] = iface
		}
		if strings.TrimSpace(nets.SCL) != "" {
			out[strings.TrimSpace(nets.SCL)] = iface
		}
	}
	for _, finding := range result.Findings {
		if strings.EqualFold(finding.BusType, "i2c") || finding.BusNets != nil {
			addI2C(finding.BusID, finding.BusNets)
		}
	}
	if contractIR != nil {
		for _, req := range contractIR.AppliedRequirements {
			if strings.EqualFold(req.Scope.BusType, "i2c") || req.Scope.Nets != nil {
				addI2C(req.Scope.BusID, req.Scope.Nets)
			}
		}
	}

	i2cGroups := []netGroup{}
	spiGroups := []netGroup{}
	uartGroups := []netGroup{}
	if design != nil {
		for _, net := range design.Nets {
			if _, ok := out[net.Name]; ok {
				continue
			}
			if key, ok := busKeyFromNetName(net.Name, "I2C", []string{"SDA", "SCL"}); ok {
				i2cGroups = append(i2cGroups, netGroup{net: net.Name, key: key})
			}
			if key, ok := busKeyFromNetName(net.Name, "SPI", []string{"MISO", "MOSI", "SCK", "SCLK", "NSS", "CS", "CSN"}); ok {
				spiGroups = append(spiGroups, netGroup{net: net.Name, key: key})
			}
			if key, ok := busKeyFromNetName(net.Name, "UART", []string{"TX", "RX", "RTS", "CTS"}); ok {
				uartGroups = append(uartGroups, netGroup{net: net.Name, key: key})
			}
		}
	}
	assignInferredInterfaces(out, "I2C", i2cGroups)
	assignInferredInterfaces(out, "SPI", spiGroups)
	assignInferredInterfaces(out, "UART", uartGroups)
	return out
}

func assignInferredInterfaces(out map[string]string, prefix string, groups []netGroup) {
	if len(groups) == 0 {
		return
	}
	groups = append([]netGroup(nil), groups...)
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].key != groups[j].key {
			return groups[i].key < groups[j].key
		}
		return groups[i].net < groups[j].net
	})
	keys := []string{}
	for _, group := range groups {
		keys = appendUnique(keys, group.key)
	}
	sort.Strings(keys)
	interfaces := map[string]string{}
	for i, key := range keys {
		interfaces[key] = fmt.Sprintf("%s_%d", strings.ToUpper(prefix), i+1)
	}
	for _, group := range groups {
		if _, exists := out[group.net]; exists {
			continue
		}
		out[group.net] = interfaces[group.key]
	}
}

func nodeLabel(part ir.Part) string {
	ref := strings.TrimSpace(part.Ref)
	value := strings.TrimSpace(part.Value)
	if value == "" {
		return ref
	}
	return ref + " " + value
}

func classifyNode(part ir.Part, contractIR *contracts.ContractIR) string {
	ref := strings.ToUpper(strings.TrimSpace(part.Ref))
	text := strings.ToUpper(strings.Join([]string{
		part.Ref,
		part.Value,
		part.Footprint,
		part.MPN,
		part.Manufacturer,
		fieldValue(part.Fields, "description", "datasheet", "device", "part", "component"),
	}, " "))
	normalized := compactAlphaNum(text)

	if hasAny(normalized, "TB6612", "L298", "L293", "A4988", "DRV8", "TMC", "MOTORDRIVER", "MOTORCONTROLLER") || strings.Contains(text, "MOTOR DRIVER") {
		return "motor_driver"
	}
	if hasAny(normalized, "ESP32", "ESP8266", "RP2040", "STM32", "ATMEGA", "ATTINY", "ARDUINO", "WROOM", "WROVER", "MICROCONTROLLER") || tokenInText(text, "MCU") {
		return "mcu"
	}
	if hasAny(normalized, "BME", "BMP", "MPU", "VL53", "VL6180", "LSM", "LIS", "SHT", "DHT", "IMU") || tokenInText(text, "SENSOR") {
		return "sensor"
	}
	if hasAny(normalized, "AMS1117", "LM1117", "AP2112", "MP1584", "LM2596") || tokenInText(text, "REGULATOR") || tokenInText(text, "LDO") || tokenInText(text, "BUCK") || tokenInText(text, "BOOST") {
		return "regulator"
	}
	if strings.HasPrefix(ref, "J") || strings.HasPrefix(ref, "P") || strings.HasPrefix(ref, "CN") || tokenInText(text, "CONNECTOR") || tokenInText(text, "HEADER") || strings.Contains(text, "CONN_") || tokenInText(text, "USB") {
		return "connector"
	}
	if strings.HasPrefix(ref, "BT") || strings.HasPrefix(ref, "BAT") || tokenInText(text, "BATTERY") || tokenInText(text, "POWER_SOURCE") {
		return "power_source"
	}
	if len(ref) > 0 {
		switch ref[0] {
		case 'R', 'C', 'L', 'D', 'F', 'Y':
			return "passive"
		}
	}
	if contractIR != nil {
		if component, ok := contractIR.Components[part.Ref]; ok {
			for _, pin := range component.Pins {
				switch pin.Role {
				case contracts.RoleRegulatorOut:
					return "regulator"
				case contracts.RoleMotorOut:
					return "motor_driver"
				case contracts.RoleSource:
					return "power_source"
				}
			}
		}
	}
	return "unknown"
}

func componentCoverage(ref string, pins []ir.PinRef, contractIR *contracts.ContractIR) string {
	if contractIR == nil {
		return "unknown"
	}
	component, hasComponent := contractIR.Components[strings.TrimSpace(ref)]
	componentCovered := hasComponent && (component.MPN != "" || component.Source != "" || component.VoltageMax != nil || len(component.Pins) > 0)
	seenPins := map[string]struct{}{}
	for _, pin := range pins {
		if strings.TrimSpace(pin.Pin) != "" {
			seenPins[pin.Pin] = struct{}{}
		}
	}
	if len(seenPins) == 0 {
		if componentCovered {
			return "partial"
		}
		return "missing"
	}
	coveredPins := 0
	for pin := range seenPins {
		if _, ok := contractIR.Pin(ref, pin); ok {
			coveredPins++
		}
	}
	switch {
	case componentCovered && coveredPins == len(seenPins):
		return "full"
	case componentCovered || coveredPins > 0:
		return "partial"
	default:
		return "missing"
	}
}

func normalizeInterfaceID(prefix string, id string) string {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	id = strings.TrimSpace(id)
	if id == "" {
		return prefix + "_1"
	}
	normalized := strings.ToUpper(sanitizeIDPart(id))
	if strings.HasPrefix(normalized, prefix+"_") || normalized == prefix {
		return normalized
	}
	return prefix + "_" + normalized
}

func busKeyFromNetName(netName string, busPrefix string, signalTokens []string) (string, bool) {
	tokens := netTokens(netName)
	if len(tokens) == 0 {
		return "", false
	}
	signalSet := stringSet(signalTokens)
	hasSignal := false
	keyTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, ok := signalSet[token]; ok {
			hasSignal = true
			continue
		}
		keyTokens = append(keyTokens, token)
	}
	if !hasSignal {
		return "", false
	}
	hasPrefix := false
	for _, token := range keyTokens {
		if token == busPrefix {
			hasPrefix = true
			break
		}
	}
	if !hasPrefix && busPrefix != "UART" {
		return "", false
	}
	if len(keyTokens) == 0 {
		return busPrefix, true
	}
	return strings.Join(keyTokens, "_"), true
}

func netHasAnyToken(net ir.Net, tokens ...string) bool {
	wanted := stringSet(tokens)
	for _, token := range netTokens(net.Name) {
		if _, ok := wanted[token]; ok {
			return true
		}
	}
	for _, pin := range net.Pins {
		for _, token := range netTokens(pin.Name) {
			if _, ok := wanted[token]; ok {
				return true
			}
		}
	}
	return false
}

func looksLikePowerNetName(netName string) bool {
	if infer.IsGroundNetName(netName) {
		return true
	}
	tokens := netTokens(netName)
	for _, token := range tokens {
		if _, ok := map[string]struct{}{
			"VCC": {}, "VDD": {}, "VSS": {}, "VIN": {}, "VBAT": {}, "VBUS": {}, "VSYS": {}, "VREF": {},
			"+5V": {}, "+3V3": {}, "5V": {}, "3V3": {}, "12V": {}, "24V": {}, "POWER": {}, "PWR": {},
		}[token]; ok {
			return true
		}
		if voltageTokenPattern.MatchString(token) {
			return true
		}
	}
	return false
}

var voltageTokenPattern = regexp.MustCompile(`^\+?[0-9]+(?:V[0-9]+|(?:\.[0-9]+)?V)$`)

func netTokens(value string) []string {
	fields := strings.FieldsFunc(strings.ToUpper(strings.TrimSpace(value)), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+')
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func findingContractID(finding report.RuleResult, fallback string) string {
	if id := strings.TrimSpace(finding.ContractID); id != "" {
		return id
	}
	if finding.Provenance != nil {
		if id := strings.TrimSpace(finding.Provenance.SourceID); id != "" {
			return id
		}
	}
	return strings.TrimSpace(fallback)
}

func findingContractSource(finding report.RuleResult) string {
	source := strings.TrimSpace(finding.ContractSource)
	switch source {
	case string(contracts.ContractSourceBuiltIn),
		string(contracts.ContractSourceUserYAML),
		string(contracts.ContractSourceMetaYAML),
		string(contracts.ContractSourceInferred):
		return source
	}
	return string(contracts.ReportContractSource(finding.Source))
}

func findingRequirement(finding report.RuleResult, fallback string) string {
	if requirement := strings.TrimSpace(finding.Requirement); requirement != "" {
		return requirement
	}
	return strings.TrimSpace(fallback)
}

func findingProvenance(finding report.RuleResult) string {
	if finding.Provenance == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if source := strings.TrimSpace(finding.Provenance.Source); source != "" {
		parts = append(parts, "source="+source)
	}
	if sourceID := strings.TrimSpace(finding.Provenance.SourceID); sourceID != "" {
		parts = append(parts, "source_id="+sourceID)
	}
	if detail := strings.TrimSpace(finding.Provenance.Detail); detail != "" {
		parts = append(parts, "detail="+detail)
	}
	return strings.Join(parts, "; ")
}

func normalizeSeverity(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "WARN", "WARNING":
		return "WARN"
	case "INFO":
		return "INFO"
	default:
		return "ERROR"
	}
}

func incrementSeverity(severity string, violations *int, warnings *int) {
	switch severity {
	case "WARN":
		(*warnings)++
	case "ERROR":
		(*violations)++
	}
}

func refsMentioned(text string, parts map[string]ir.Part) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	out := []string{}
	for ref := range parts {
		if containsRefToken(text, ref) {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

func containsRefToken(text string, ref string) bool {
	return containsExactText(text, ref)
}

func containsExactText(text string, value string) bool {
	upperText := strings.ToUpper(text)
	upperRef := strings.ToUpper(value)
	if upperRef == "" || len(upperText) < len(upperRef) {
		return false
	}
	start := 0
	for {
		idx := strings.Index(upperText[start:], upperRef)
		if idx < 0 {
			return false
		}
		idx += start
		beforeOK := idx == 0 || !isRefByte(upperText[idx-1])
		afterIndex := idx + len(upperRef)
		afterOK := afterIndex >= len(upperText) || !isRefByte(upperText[afterIndex])
		if beforeOK && afterOK {
			return true
		}
		start = idx + len(upperRef)
		if start >= len(upperText) {
			return false
		}
	}
}

func isRefByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func conflictNetName(message string) string {
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

func uniqueRefsForNet(net ir.Net) []string {
	seen := map[string]struct{}{}
	refs := []string{}
	for _, pin := range net.Pins {
		ref := strings.TrimSpace(pin.Ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeString(values []string, remove string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != remove {
			out = append(out, value)
		}
	}
	return out
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func exactStringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func interfaceOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func sanitizeIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}

func uniqueGraphID(base string, used map[string]int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "id"
	}
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, used[base])
}

func fieldValue(fields map[string]string, keys ...string) string {
	if len(fields) == 0 {
		return ""
	}
	wanted := map[string]struct{}{}
	for _, key := range keys {
		wanted[compactAlphaNum(key)] = struct{}{}
	}
	matches := make([]string, 0, len(fields))
	for key, value := range fields {
		if _, ok := wanted[compactAlphaNum(key)]; ok && strings.TrimSpace(value) != "" {
			matches = append(matches, strings.TrimSpace(value))
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func parseResistanceOhms(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.Trim(value, `"'`)
	if value == "" {
		return 0, false
	}
	value = strings.ReplaceAll(value, "ohms", "")
	value = strings.ReplaceAll(value, "ohm", "")
	value = strings.ReplaceAll(value, "Ω", "")
	value = strings.TrimSpace(value)
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "kohm"):
		multiplier = 1000
		value = strings.TrimSuffix(value, "kohm")
	case strings.HasSuffix(value, "k"):
		multiplier = 1000
		value = strings.TrimSuffix(value, "k")
	case strings.HasSuffix(value, "m"):
		multiplier = 1000000
		value = strings.TrimSuffix(value, "m")
	}
	if strings.Contains(value, "k") {
		value = strings.Replace(value, "k", ".", 1)
		multiplier = 1000
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed * multiplier, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactAlphaNum(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, compactAlphaNum(needle)) {
			return true
		}
	}
	return false
}

func tokenInText(text string, token string) bool {
	token = strings.ToUpper(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	for _, got := range netTokens(text) {
		if got == token {
			return true
		}
	}
	return false
}
