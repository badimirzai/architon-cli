package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	contractspkg "github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/spf13/cobra"
)

const connectionsProposalFileName = "connections.proposal.yaml"

func init() {
	rootCmd.AddCommand(newConnectionsCmd())
}

func newConnectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connections",
		Short: "Propose and apply reviewable connections",
		Long: `Propose reviewable connections from built-in pin functions, then apply accepted entries.

  rv connections propose <path>  Write .architon/connections.proposal.yaml. This is not a scan result.
  rv connections apply <path>    Add net labels for accepted entries. This does not scan.`,
	}
	cmd.AddCommand(newConnectionsProposeCmd())
	cmd.AddCommand(newConnectionsApplyCmd())
	return cmd
}

func newConnectionsProposeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "propose <path>",
		Args:          cobra.ExactArgs(1),
		Short:         "Write a reviewable connection proposal from pin functions",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Write .architon/connections.proposal.yaml from built-in pin functions and a netlist.

The proposal is not a scan result. The command does not verify the design, does not
call a model, does not read a PDF, and does not evaluate the proposal with the rule
engine. It does not edit a KiCad file or .architon/contracts.yaml.

A decided entry requires two different matched parts with a dedicated SDA or SCL pin.
gpio_candidate pins cannot decide an entry. One of those pins is already on a net, and
the other is unconnected or already on that same net.

An MCU with only gpio_candidate pins produces needs_choice. Every candidate pin is
listed. No candidate is picked or ranked. A dedicated pin on a different net produces
conflict and does not propose a join.

If the proposal file already exists, the command exits 3 unless --force is set.
If nothing matches, the command exits 0 and writes an empty connections list.
It prints the proposal path, the decided count, the needs_choice count, and the
conflict count.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return runConnectionsPropose(cmd, args[0], force)
		},
	}
	cmd.Flags().Bool("force", false, "Overwrite an existing .architon/connections.proposal.yaml")
	return cmd
}

// runConnectionsPropose resolves a scan input, imports it, and writes a connection proposal.
// Import stops at DesignIR. The rule engine is not called.
func runConnectionsPropose(cmd *cobra.Command, inputPath string, force bool) error {
	proposalPath, document, err := saveConnectionProposal(inputPath, force)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), proposalPath)
	fmt.Fprintf(cmd.OutOrStdout(), "decided: %d\n", document.Decided)
	fmt.Fprintf(cmd.OutOrStdout(), "needs_choice: %d\n", document.NeedsChoice)
	fmt.Fprintf(cmd.OutOrStdout(), "conflict: %d\n", document.Conflict)
	return nil
}

// buildConnectionProposal imports a scan input and builds a connection proposal.
// It does not write a file and does not call the rule engine.
func buildConnectionProposal(inputPath string) (contractspkg.ConnectionProposal, string, error) {
	resolved, err := resolveScanInput(inputPath, "", "")
	if err != nil {
		return contractspkg.ConnectionProposal{}, "", fatalError(err)
	}
	design, err := importResolvedScanInput(resolved, "")
	if err != nil {
		return contractspkg.ConnectionProposal{}, "", userError(err)
	}
	proposalPath, err := connectionsProposalPath(resolved)
	if err != nil {
		return contractspkg.ConnectionProposal{}, "", fatalError(err)
	}
	if filepath.Base(proposalPath) != connectionsProposalFileName {
		return contractspkg.ConnectionProposal{}, "", fatalError(fmt.Errorf("refusing to write %s", proposalPath))
	}
	return contractspkg.ProposeConnections(design), proposalPath, nil
}

func saveConnectionProposal(inputPath string, force bool) (string, contractspkg.ConnectionProposal, error) {
	document, proposalPath, err := buildConnectionProposal(inputPath)
	if err != nil {
		return "", contractspkg.ConnectionProposal{}, err
	}
	if err := guardConnectionProposal(proposalPath, force); err != nil {
		return "", contractspkg.ConnectionProposal{}, err
	}
	if err := writeConnectionsProposal(proposalPath, []byte(document.YAML), force); err != nil {
		return "", contractspkg.ConnectionProposal{}, err
	}
	return proposalPath, document, nil
}

func guardConnectionProposal(proposalPath string, force bool) error {
	if force {
		return nil
	}
	info, statErr := os.Stat(proposalPath)
	switch {
	case statErr == nil:
		if info.IsDir() {
			return fatalError(fmt.Errorf("%s is a directory", proposalPath))
		}
		return &ExitError{
			Code: 3,
			Err:  fmt.Errorf("connections proposal already exists: %s", proposalPath),
		}
	case os.IsNotExist(statErr):
		return nil
	default:
		return fatalError(fmt.Errorf("stat connections proposal: %w", statErr))
	}
}

// connectionsProposalPath is .architon/connections.proposal.yaml under the resolved project.
// It is never .architon/contracts.yaml.
func connectionsProposalPath(input resolvedScanInput) (string, error) {
	root := contractsProjectRoot(input)
	if strings.TrimSpace(root) == "" {
		return "", errors.New("connections proposal: project path is empty")
	}
	proposal := filepath.Join(root, ".architon", connectionsProposalFileName)
	live := filepath.Join(root, ".architon", "contracts.yaml")
	draft := filepath.Join(root, ".architon", contractsDraftFileName)
	cleanProposal := filepath.Clean(proposal)
	if cleanProposal == filepath.Clean(live) || cleanProposal == filepath.Clean(draft) {
		return "", errors.New("refusing to write .architon/contracts.yaml")
	}
	return proposal, nil
}

func writeConnectionsProposal(path string, data []byte, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fatalError(fmt.Errorf("create connections proposal directory: %w", err))
	}
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		if !force && errors.Is(err, os.ErrExist) {
			return &ExitError{
				Code: 3,
				Err:  fmt.Errorf("connections proposal already exists: %s", path),
			}
		}
		return fatalError(fmt.Errorf("write connections proposal: %w", err))
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fatalError(fmt.Errorf("write connections proposal: %w", err))
	}
	return nil
}
