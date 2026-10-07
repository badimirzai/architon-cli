package cmd

import (
	"fmt"

	contractspkg "github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/spf13/cobra"
)

var contractsCmd = newContractsCmd()

func init() {
	rootCmd.AddCommand(contractsCmd)
}

func newContractsCmd() *cobra.Command {
	// Keep contract tooling under one namespace so scan behavior and schema
	// validation can evolve independently.
	cmd := &cobra.Command{
		Use:   "contracts",
		Short: "Draft or validate project contracts",
		Long: `Draft a reviewable contracts file from a netlist, or validate contracts YAML.

  rv contracts draft <path>     Write .architon/contracts.draft.yaml. This is not a verification result.
  rv contracts validate <path>  Validate a contracts YAML schema only.`,
	}
	cmd.AddCommand(newContractsDraftCmd())
	cmd.AddCommand(newContractsValidateCmd())
	return cmd
}

func newContractsValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Validate a contracts.yaml schema only",
		Long: `Validate a contracts.yaml schema only.

This checks contract YAML syntax and schema. It does not verify a design.
Use rv scan --contracts <path> to enforce contracts against a project.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validation is intentionally schema-only. It does not need a design
			// file because contracts must be deterministic before scan time.
			if _, err := contractspkg.LoadYAMLFile(args[0]); err != nil {
				return &ExitError{
					Code: 3,
					Err:  fmt.Errorf("contracts invalid: %w", err),
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "contracts valid")
			return nil
		},
	}
}
