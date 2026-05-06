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
		Short: "Validate project-defined system contracts",
	}
	cmd.AddCommand(newContractsValidateCmd())
	return cmd
}

func newContractsValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Validate a contracts.yaml schema",
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
