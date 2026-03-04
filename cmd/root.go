package cmd

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rv",
	Short: "Architon CLI (rv)",
	Long: `Architon CLI (rv) - early-stage electrical architecture checks for robotics projects.

Quick help:
  rv check <file.yaml>       Run analysis
  rv scan <path>             Import KiCad BOM/netlist and emit DesignIR report JSON
  rv init                    Initialize Architon metadata or write a starter robot spec
  rv check --output json     Emit JSON findings
  rv version                 Show installed version
  rv --help                  Show all commands and flags`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	defer func() {
		if recovered := recover(); recovered != nil {
			if debugEnabled {
				fmt.Fprintf(os.Stderr, "panic: %v\n", recovered)
				fmt.Fprintln(os.Stderr, string(debug.Stack()))
			} else {
				fmt.Fprintln(os.Stderr, "internal error: unexpected panic (run with --debug or RV_DEBUG=1)")
			}
			os.Exit(3)
		}
	}()
	if err := rootCmd.Execute(); err != nil {
		handleError(err)
	}
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "Print internal error details")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if cmd.Flags().Changed("debug") {
			debugEnabled, _ = cmd.Flags().GetBool("debug")
		} else {
			debugEnabled = envBool("RV_DEBUG")
		}

		noColor, _ := cmd.Flags().GetBool("no-color")
		colorsEnabled := ui.DefaultColorEnabled()
		if noColor {
			colorsEnabled = false
		}
		ui.EnableColors(colorsEnabled)
	}
	// subcommands register themselves in init()
}

var debugEnabled bool

func handleError(err error) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Internal {
			if debugEnabled && exitErr.Err != nil {
				fmt.Fprintln(os.Stderr, exitErr.Err)
			} else {
				fmt.Fprintln(os.Stderr, "internal error: please report or re-run with --debug or RV_DEBUG=1")
			}
		} else if !exitErr.Silent && exitErr.Err != nil {
			fmt.Fprintln(os.Stderr, exitErr.Err)
		}
		os.Exit(exitErr.Code)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}

func envBool(name string) bool {
	val := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return val == "1" || val == "true" || val == "yes"
}
