package cmd

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

const proposeToolName = "propose"

const proposeToolDescription = "Read a connection proposal from the same pin functions and netlist as `rv connections propose`. " +
	"Returns decided, needs_choice, conflict, and citations as JSON. This is not a scan result and not a pass. " +
	"A needs_choice entry lists every candidate pin and does not mark a GPIO accepted. " +
	"Does not write a file unless write is true. When write is true, writes .architon/connections.proposal.yaml. " +
	"If that file exists, the write fails and the file is left unchanged unless force is true, the same rule as `rv connections propose --force`. " +
	"Does not edit a schematic or .architon/contracts.yaml. " +
	"To accept a needs_choice entry, edit the written YAML: set status to accepted and set pin to one name or number copied from that entry's candidates. Leave candidates on the entry."

const applyToolName = "apply"

const applyToolDescription = "Add net labels for entries marked accepted, the same label apply as `rv connections apply`. " +
	"proposal_path is .architon/connections.proposal.yaml or the project directory that contains it. " +
	"Returns files_changed and ids_applied. Does not return an exit code and does not scan. " +
	"decided, needs_choice, and conflict are not applied. A needs_choice entry is skipped. " +
	"An accepted needs_choice entry may name only a pin copied from that entry's candidates. " +
	"A pin that is not in the list is rejected and no file is changed. " +
	"Does not draw wires, move symbols, or edit a .kicad_pcb. Call verify for the pass."

type proposeArgs struct {
	ProjectPath string `json:"project_path" jsonschema:"directory or design file, the same path rv connections propose accepts"`
	Write       bool   `json:"write,omitempty" jsonschema:"when true, write .architon/connections.proposal.yaml. Omit or false to leave the project unchanged"`
	Force       bool   `json:"force,omitempty" jsonschema:"when write is true, overwrite an existing proposal file. Same rule as rv connections propose --force"`
}

type proposeOutput struct {
	Decided     []proposeEntry    `json:"decided" jsonschema:"entries whose status is decided. A decided entry is not a scan result and is not accepted"`
	NeedsChoice []proposeEntry    `json:"needs_choice" jsonschema:"entries whose status is needs_choice. Each lists every candidate pin and does not mark a GPIO accepted"`
	Conflict    []proposeEntry    `json:"conflict" jsonschema:"entries whose status is conflict. A conflict does not propose a join"`
	Citations   []proposeCitation `json:"citations" jsonschema:"datasheet citations copied from the proposal"`
	Path        string            `json:"path,omitempty" jsonschema:"absolute path of the proposal file written when write is true"`
	Error       string            `json:"error,omitempty" jsonschema:"set when the proposal could not be built or the file was not written"`
}

type proposeEntry struct {
	ID         string             `json:"id" yaml:"id"`
	Status     string             `json:"status" yaml:"status"`
	Signal     string             `json:"signal" yaml:"signal"`
	Net        string             `json:"net,omitempty" yaml:"net,omitempty"`
	Ref        string             `json:"ref,omitempty" yaml:"ref,omitempty"`
	MPN        string             `json:"mpn,omitempty" yaml:"mpn,omitempty"`
	Parts      []proposePart      `json:"parts,omitempty" yaml:"parts,omitempty"`
	Candidates []proposeCandidate `json:"candidates,omitempty" yaml:"candidates,omitempty"`
}

type proposePart struct {
	Ref      string          `json:"ref" yaml:"ref"`
	MPN      string          `json:"mpn,omitempty" yaml:"mpn,omitempty"`
	Pin      string          `json:"pin" yaml:"pin"`
	Net      string          `json:"net,omitempty" yaml:"net,omitempty"`
	Nets     []string        `json:"nets,omitempty" yaml:"nets,omitempty"`
	Citation proposeCitation `json:"citation,omitempty" yaml:"citation,omitempty"`
}

type proposeCandidate struct {
	Pins     []string        `json:"pins" yaml:"pins"`
	Number   string          `json:"number,omitempty" yaml:"number,omitempty"`
	Citation proposeCitation `json:"citation,omitempty" yaml:"citation,omitempty"`
}

type proposeCitation struct {
	Datasheet string `json:"datasheet" yaml:"datasheet"`
	Revision  string `json:"revision" yaml:"revision"`
	Table     string `json:"table,omitempty" yaml:"table,omitempty"`
	Section   string `json:"section,omitempty" yaml:"section,omitempty"`
}

type applyArgs struct {
	ProposalPath string `json:"proposal_path" jsonschema:"path to .architon/connections.proposal.yaml, or the project directory that contains it"`
}

type applyOutput struct {
	Files []string `json:"files_changed" jsonschema:"schematic files whose bytes changed"`
	IDs   []string `json:"ids_applied" jsonschema:"ids of accepted entries that were applied"`
	Error string   `json:"error,omitempty" jsonschema:"set when apply rejects the proposal and writes nothing"`
}

func proposeTool(ctx context.Context, _ *mcp.CallToolRequest, args proposeArgs) (*mcp.CallToolResult, proposeOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, emptyProposeOutput(), err
	}
	return nil, runProposeTool(args), nil
}

func runProposeTool(args proposeArgs) proposeOutput {
	out := emptyProposeOutput()
	if strings.TrimSpace(args.ProjectPath) == "" {
		out.Error = "project_path is required"
		return out
	}
	document, proposalPath, err := buildConnectionProposal(args.ProjectPath)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	parsed, err := proposeOutputFromYAML(document.YAML)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out = parsed
	if !args.Write {
		return out
	}
	if err := guardConnectionProposal(proposalPath, args.Force); err != nil {
		out.Error = err.Error()
		return out
	}
	if err := writeConnectionsProposal(proposalPath, []byte(document.YAML), args.Force); err != nil {
		out.Error = err.Error()
		return out
	}
	out.Path = proposalPath
	return out
}

func emptyProposeOutput() proposeOutput {
	return proposeOutput{
		Decided:     []proposeEntry{},
		NeedsChoice: []proposeEntry{},
		Conflict:    []proposeEntry{},
		Citations:   []proposeCitation{},
	}
}

func proposeOutputFromYAML(raw string) (proposeOutput, error) {
	out := emptyProposeOutput()
	var doc struct {
		Connections []proposeEntry `yaml:"connections"`
	}
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return out, err
	}
	seen := map[string]struct{}{}
	for _, entry := range doc.Connections {
		switch entry.Status {
		case "decided":
			out.Decided = append(out.Decided, entry)
		case "needs_choice":
			out.NeedsChoice = append(out.NeedsChoice, entry)
		case "conflict":
			out.Conflict = append(out.Conflict, entry)
		}
		for _, citation := range proposeEntryCitations(entry) {
			key := citation.Datasheet + "\n" + citation.Revision + "\n" + citation.Table + "\n" + citation.Section
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out.Citations = append(out.Citations, citation)
		}
	}
	return out, nil
}

func proposeEntryCitations(entry proposeEntry) []proposeCitation {
	out := make([]proposeCitation, 0, len(entry.Parts)+len(entry.Candidates))
	for _, part := range entry.Parts {
		if part.Citation.Datasheet != "" {
			out = append(out, part.Citation)
		}
	}
	for _, candidate := range entry.Candidates {
		if candidate.Citation.Datasheet != "" {
			out = append(out, candidate.Citation)
		}
	}
	return out
}

func applyTool(ctx context.Context, _ *mcp.CallToolRequest, args applyArgs) (*mcp.CallToolResult, applyOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, emptyApplyOutput(), err
	}
	return nil, runApplyTool(args), nil
}

func runApplyTool(args applyArgs) applyOutput {
	out := emptyApplyOutput()
	if strings.TrimSpace(args.ProposalPath) == "" {
		out.Error = "proposal_path is required"
		return out
	}
	files, ids, err := applyConnectionLabels(args.ProposalPath)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if files != nil {
		out.Files = files
	}
	if ids != nil {
		out.IDs = ids
	}
	return out
}

func emptyApplyOutput() applyOutput {
	return applyOutput{
		Files: []string{},
		IDs:   []string{},
	}
}
