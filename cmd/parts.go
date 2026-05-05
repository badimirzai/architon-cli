package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/spf13/cobra"
)

var partsCmd = newPartsCmd()

func init() {
	rootCmd.AddCommand(partsCmd)
}

func newPartsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "parts",
		Short: "Inspect built-in deterministic contract parts",
	}
	cmd.AddCommand(newPartsListCmd())
	cmd.AddCommand(newPartsShowCmd())
	return cmd
}

func newPartsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Args:  cobra.NoArgs,
		Short: "List built-in contract parts",
		Run: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			parts := contracts.BuiltinContracts()
			sort.Slice(parts, func(i, j int) bool { return parts[i].MPN < parts[j].MPN })
			if isJSONFormat(format) {
				writeJSON(cmd, parts)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Built-in contract parts:")
			for _, part := range parts {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s", part.MPN)
				if part.Manufacturer != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " (%s)", part.Manufacturer)
				}
				fmt.Fprintf(cmd.OutOrStdout(), " - %s\n", requirementTypes(part.Requirements))
			}
		},
	}
	cmd.Flags().String("format", "text", "Output format: text or json")
	return cmd
}

func newPartsShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <mpn>",
		Args:  cobra.ExactArgs(1),
		Short: "Show one built-in contract part",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			match := contracts.MatchPart(ir.Part{MPN: args[0], Value: args[0]}, contracts.BuiltinContracts())
			if match.Ambiguous {
				return userError(fmt.Errorf("ambiguous built-in part %q: %s", args[0], strings.Join(match.Candidates, ", ")))
			}
			if !match.Matched {
				return userError(fmt.Errorf("built-in part %q not found", args[0]))
			}
			part := match.Contract
			if isJSONFormat(format) {
				writeJSON(cmd, part)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "MPN: %s\n", part.MPN)
			if part.Manufacturer != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Manufacturer: %s\n", part.Manufacturer)
			}
			if part.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", part.Description)
			}
			if len(part.Aliases) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Aliases: %s\n", strings.Join(part.Aliases, ", "))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Contracts:")
			for _, req := range part.Requirements {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s", req.Type)
				if len(req.Scope.Pins) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), " pins=%s", strings.Join(req.Scope.Pins, ","))
				}
				if req.MinVoltage != nil || req.MaxVoltage != nil {
					fmt.Fprintf(cmd.OutOrStdout(), " voltage=%s", voltageRange(req.MinVoltage, req.MaxVoltage))
				}
				if req.MaxCurrent != nil {
					fmt.Fprintf(cmd.OutOrStdout(), " max_current=%.2fA", *req.MaxCurrent)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().String("format", "text", "Output format: text or json")
	return cmd
}

func isJSONFormat(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), "json")
}

func writeJSON(cmd *cobra.Command, value any) {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func requirementTypes(reqs []contracts.Requirement) string {
	types := make([]string, 0, len(reqs))
	seen := map[string]struct{}{}
	for _, req := range reqs {
		key := string(req.Type)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		types = append(types, key)
	}
	sort.Strings(types)
	return strings.Join(types, ", ")
}

func voltageRange(minV *float64, maxV *float64) string {
	switch {
	case minV != nil && maxV != nil:
		return fmt.Sprintf("%.2f..%.2fV", *minV, *maxV)
	case minV != nil:
		return fmt.Sprintf(">=%.2fV", *minV)
	case maxV != nil:
		return fmt.Sprintf("<=%.2fV", *maxV)
	default:
		return ""
	}
}
