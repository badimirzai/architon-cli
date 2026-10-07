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

const contractsDraftFileName = "contracts.draft.yaml"

func newContractsDraftCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "draft <path>",
		Args:          cobra.ExactArgs(1),
		Short:         "Write a reviewable contracts draft from a netlist",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Write .architon/contracts.draft.yaml from a netlist.

The draft is not a verification result. The command does not scan the design,
does not evaluate the draft, and does not fill pin tokens or power_budget currents.

It never overwrites .architon/contracts.yaml. If the draft file already exists,
the command exits 3 unless --force is set.

Net names match with or without a leading slash. When none of the recognized
bus nets exist, the command writes an empty contracts list and exits 0.
It prints the draft path and the contract ids.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return runContractsDraft(cmd, args[0], force)
		},
	}
	cmd.Flags().Bool("force", false, "Overwrite an existing .architon/contracts.draft.yaml")
	return cmd
}

// runContractsDraft resolves a scan input, imports it, and writes a contracts draft.
// It does not call the scan pipeline and does not evaluate the draft.
func runContractsDraft(cmd *cobra.Command, inputPath string, force bool) error {
	resolved, err := resolveScanInput(inputPath, "", "")
	if err != nil {
		return fatalError(err)
	}
	design, err := importResolvedScanInput(resolved, "")
	if err != nil {
		return userError(err)
	}
	draftPath, err := contractsDraftPath(resolved)
	if err != nil {
		return fatalError(err)
	}
	if filepath.Base(draftPath) != contractsDraftFileName {
		return fatalError(fmt.Errorf("refusing to write %s", draftPath))
	}
	if !force {
		info, statErr := os.Stat(draftPath)
		switch {
		case statErr == nil:
			if info.IsDir() {
				return fatalError(fmt.Errorf("%s is a directory", draftPath))
			}
			return &ExitError{
				Code: 3,
				Err:  fmt.Errorf("contracts draft already exists: %s", draftPath),
			}
		case os.IsNotExist(statErr):
		default:
			return fatalError(fmt.Errorf("stat contracts draft: %w", statErr))
		}
	}

	document := contractspkg.DraftFromDesign(design)
	if err := writeContractsDraft(draftPath, []byte(document.YAML), force); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), draftPath)
	for _, id := range document.IDs {
		fmt.Fprintln(cmd.OutOrStdout(), id)
	}
	return nil
}

// contractsDraftPath is .architon/contracts.draft.yaml under the resolved project.
// It is never .architon/contracts.yaml.
func contractsDraftPath(input resolvedScanInput) (string, error) {
	root := contractsProjectRoot(input)
	if strings.TrimSpace(root) == "" {
		return "", errors.New("contracts draft: project path is empty")
	}
	draft := filepath.Join(root, ".architon", contractsDraftFileName)
	live := filepath.Join(root, ".architon", "contracts.yaml")
	if filepath.Clean(draft) == filepath.Clean(live) {
		return "", errors.New("refusing to write .architon/contracts.yaml")
	}
	return draft, nil
}

// contractsProjectRoot is the directory whose .architon folder receives the draft.
// Directory scans use the resolved project path. A netlist file uses its directory,
// or the parent of .architon when the file already lives there.
func contractsProjectRoot(input resolvedScanInput) string {
	if input.Directory && strings.TrimSpace(input.ProjectPath) != "" {
		return filepath.Clean(input.ProjectPath)
	}
	direct := filepath.Clean(strings.TrimSpace(input.DirectPath))
	if direct == "" || direct == "." {
		return "."
	}
	dir := filepath.Dir(direct)
	if filepath.Base(dir) == ".architon" {
		root := filepath.Dir(dir)
		if root == "" {
			return "."
		}
		return root
	}
	if dir == "" {
		return "."
	}
	return dir
}

func writeContractsDraft(path string, data []byte, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fatalError(fmt.Errorf("create contracts draft directory: %w", err))
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
				Err:  fmt.Errorf("contracts draft already exists: %s", path),
			}
		}
		return fatalError(fmt.Errorf("write contracts draft: %w", err))
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fatalError(fmt.Errorf("write contracts draft: %w", err))
	}
	return nil
}
