package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/importers/kicad"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const connectionsApplyNextStep = "Run rv scan or the verify tool."

func newConnectionsApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "apply <path>",
		Args:          cobra.ExactArgs(1),
		Short:         "Add net labels for accepted connection proposal entries",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Add one net label for each unconnected pin named by an accepted entry in .architon/connections.proposal.yaml.

The command reads that proposal and the KiCad schematics. It does not scan, does not call a model,
does not draw wires, and does not edit a .kicad_pcb.

Only an entry whose status is accepted is applied. decided, needs_choice, and conflict are left
untouched. A decided entry is not accepted until a person or a later tool changes that status.

For each accepted entry, both pins must already exist. A missing pin or a pin already on a different
net exits 3 and writes nothing. A pin already on the proposal net is left as it is. An unconnected
pin gets one net label at that pin. The symbol is not moved.

Every accepted entry is applied, or none are. On failure the schematic bytes are restored.

The command prints the schematic paths that changed and the entry ids applied. It does not print a
pass or fail. Run rv scan or the verify tool to see the verdict.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectionsApply(cmd, args[0])
		},
	}
}

// runConnectionsApply adds net labels for accepted proposal entries.
// It does not import a netlist, call the rule engine, or invoke a model.
func runConnectionsApply(cmd *cobra.Command, inputPath string) error {
	root, err := connectionsApplyRoot(inputPath)
	if err != nil {
		return fatalError(err)
	}
	proposalPath := filepath.Join(root, ".architon", connectionsProposalFileName)
	data, err := readConnectionsProposal(proposalPath)
	if err != nil {
		return err
	}
	entries, err := acceptedNetLabelEntries(data)
	if err != nil {
		return fatalError(err)
	}
	out := cmd.OutOrStdout()
	if len(entries) == 0 {
		fmt.Fprintln(out, connectionsApplyNextStep)
		return nil
	}

	files, err := readProjectSchematics(root)
	if err != nil {
		return fatalError(err)
	}
	changed, err := kicad.AddNetLabels(files, entries)
	if err != nil {
		return fatalError(err)
	}
	if err := writeChangedSchematics(changed, files); err != nil {
		return fatalError(err)
	}

	paths := make([]string, 0, len(changed))
	for path := range changed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintln(out, path)
	}
	for _, entry := range entries {
		fmt.Fprintln(out, entry.ID)
	}
	fmt.Fprintln(out, connectionsApplyNextStep)
	return nil
}

func connectionsApplyRoot(inputPath string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(inputPath))
	if clean == "" {
		clean = "."
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("stat input path: %w", err)
	}
	root := clean
	if !info.IsDir() {
		root = contractsProjectRoot(resolvedScanInput{DirectPath: clean})
		if strings.TrimSpace(root) == "" {
			return "", errors.New("connections apply: project path is empty")
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	return filepath.Clean(abs), nil
}

func readConnectionsProposal(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fatalError(fmt.Errorf("connections proposal not found: %s", path))
		}
		return nil, fatalError(fmt.Errorf("stat connections proposal: %w", err))
	}
	if info.IsDir() {
		return nil, fatalError(fmt.Errorf("%s is a directory", path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fatalError(fmt.Errorf("read connections proposal: %w", err))
	}
	return data, nil
}

type yamlText string

func (text *yamlText) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a string")
	}
	*text = yamlText(node.Value)
	return nil
}

type connectionProposalApplyDoc struct {
	Connections []connectionProposalApplyEntry `yaml:"connections"`
}

type connectionProposalApplyEntry struct {
	ID     yamlText                      `yaml:"id"`
	Status yamlText                      `yaml:"status"`
	Net    yamlText                      `yaml:"net"`
	Parts  []connectionProposalApplyPart `yaml:"parts"`
}

type connectionProposalApplyPart struct {
	Ref yamlText `yaml:"ref"`
	Pin yamlText `yaml:"pin"`
}

func acceptedNetLabelEntries(data []byte) ([]kicad.NetLabelEntry, error) {
	var doc connectionProposalApplyDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("connections proposal: %w", err)
	}
	out := make([]kicad.NetLabelEntry, 0)
	for _, entry := range doc.Connections {
		if strings.TrimSpace(string(entry.Status)) != "accepted" {
			continue
		}
		id := strings.TrimSpace(string(entry.ID))
		net := strings.TrimSpace(string(entry.Net))
		if id == "" {
			return nil, errors.New("accepted connection is missing an id")
		}
		if net == "" || strings.Trim(net, "/") == "" {
			return nil, fmt.Errorf("accepted entry %s has no net", id)
		}
		if len(entry.Parts) != 2 {
			return nil, fmt.Errorf("accepted entry %s needs two parts", id)
		}
		var pins [2]kicad.NetLabelPin
		for i, part := range entry.Parts {
			pins[i] = kicad.NetLabelPin{
				Ref: strings.TrimSpace(string(part.Ref)),
				Pin: strings.TrimSpace(string(part.Pin)),
			}
			if pins[i].Ref == "" || pins[i].Pin == "" {
				return nil, fmt.Errorf("accepted entry %s needs two parts", id)
			}
		}
		if pins[0].Ref == pins[1].Ref {
			return nil, fmt.Errorf("accepted entry %s needs two parts", id)
		}
		out = append(out, kicad.NetLabelEntry{ID: id, Net: net, Pins: pins})
	}
	return out, nil
}

func readProjectSchematics(root string) (map[string][]byte, error) {
	paths, err := findProjectSchematics(root)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schematic: %w", err)
		}
		files[path] = data
	}
	return files, nil
}

func findProjectSchematics(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != root && skipSchematicDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSchematicName(name) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list schematics: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func skipSchematicDir(name string) bool {
	if name == ".git" || name == ".architon" {
		return true
	}
	return strings.Contains(strings.ToLower(name), "backup")
}

func isSchematicName(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~") {
		return false
	}
	lower := strings.ToLower(name)
	if strings.Contains(lower, "autosave") || strings.HasSuffix(lower, ".bak") {
		return false
	}
	return strings.EqualFold(filepath.Ext(name), ".kicad_sch")
}

func writeChangedSchematics(changed map[string][]byte, original map[string][]byte) error {
	paths := make([]string, 0, len(changed))
	for path := range changed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	written := make([]string, 0, len(paths))
	for _, path := range paths {
		next := changed[path]
		prev, ok := original[path]
		if ok && string(prev) == string(next) {
			continue
		}
		if err := writeSchematicAtomic(path, next); err != nil {
			if restoreErr := restoreSchematics(written, original); restoreErr != nil {
				return fmt.Errorf("write schematic %s: %w (restore: %v)", path, err, restoreErr)
			}
			return fmt.Errorf("write schematic %s: %w", path, err)
		}
		written = append(written, path)
	}
	return nil
}

func restoreSchematics(paths []string, original map[string][]byte) error {
	var first error
	for _, path := range paths {
		data, ok := original[path]
		if !ok {
			continue
		}
		if err := writeSchematicAtomic(path, data); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func writeSchematicAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".architon-sch-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
