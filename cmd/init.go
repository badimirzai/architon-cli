package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/badimirzai/architon-cli/templates"
	"github.com/spf13/cobra"
)

var initCmd = newInitCmd()

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a starter robot spec from a template",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetBool("list")
			if list {
				for _, name := range templates.Names() {
					fmt.Fprintln(cmd.OutOrStdout(), name)
				}
				return nil
			}

			templateName, _ := cmd.Flags().GetString("template")
			templateName = strings.TrimSpace(templateName)
			if templateName == "" {
				if err := cmd.Help(); err != nil {
					return err
				}
				return userError(fmt.Errorf("missing --template (use --list to see available templates)"))
			}

			outPath, _ := cmd.Flags().GetString("out")
			outPath = strings.TrimSpace(outPath)
			if outPath == "" {
				outPath = "robot.yaml"
			}

			if info, err := os.Stat(outPath); err == nil && info != nil {
				force, _ := cmd.Flags().GetBool("force")
				if !force {
					return userError(fmt.Errorf("output file exists: %s (use --force to overwrite)", outPath))
				}
			} else if err != nil && !os.IsNotExist(err) {
				return userError(fmt.Errorf("check output file: %w", err))
			}

			data, err := templates.Load(templateName)
			if err != nil {
				return userError(err)
			}
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				return userError(fmt.Errorf("write template: %w", err))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s (template: %s)\n", outPath, templateName)
			return nil
		},
	}

	cmd.Flags().String("template", "", "Template name")
	cmd.Flags().String("out", "robot.yaml", "Output path")
	cmd.Flags().Bool("force", false, "Overwrite output file if it exists")
	cmd.Flags().Bool("list", false, "List available templates")

	return cmd
}

func init() {
	rootCmd.AddCommand(initCmd)
}
