package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/badimirzai/architon-cli/internal/version"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check local rv and KiCad CLI setup",
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Architon doctor\n")
		fmt.Fprintf(out, "Version: %s\n", version.Line())
		fmt.Fprintf(out, "Go runtime: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

		if executable, err := os.Executable(); err == nil {
			fmt.Fprintf(out, "Executable: %s\n", executable)
		}
		if rvPath, err := exec.LookPath("rv"); err == nil {
			fmt.Fprintf(out, "rv on PATH: %s\n", rvPath)
		} else {
			fmt.Fprintf(out, "rv on PATH: not found\n")
		}

		if kicadCLI, err := resolveKiCadCLIPath(defaultKiCadCLI); err == nil {
			fmt.Fprintf(out, "KiCad CLI: %s\n", kicadCLI)
			fmt.Fprintf(out, "KiCad netlist export: available\n")
		} else {
			fmt.Fprintf(out, "KiCad CLI: not found\n")
			fmt.Fprintf(out, "KiCad netlist export: unavailable (%v)\n", err)
			fmt.Fprintf(out, "Install KiCad, add kicad-cli to PATH, or use rv scan . --kicad-cli /full/path/to/kicad-cli\n")
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
