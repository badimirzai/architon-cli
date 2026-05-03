package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--clean" {
		if err := os.RemoveAll("bin"); err != nil {
			fmt.Fprintf(os.Stderr, "clean bin: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := os.MkdirAll("bin", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create bin: %v\n", err)
		os.Exit(1)
	}

	name := "rv"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	args := append([]string{"build"}, os.Args[1:]...)
	args = append(args, "-o", filepath.Join("bin", name), "./cmd/rv")
	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n", err)
		os.Exit(1)
	}
}
