package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestCheckInvalidYAMLProcessExitCodeIsThree(t *testing.T) {
	specPath := writeCheckSpec(t, "name: [\n")

	stdout, stderr, code := runCLIProcess(t, "check", specPath)
	if code != 3 {
		t.Fatalf("expected process exit code 3, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !bytes.Contains([]byte(stdout), []byte("exit code: 3")) {
		t.Fatalf("expected stdout to include exit code 3, got %q", stdout)
	}
	if !bytes.Contains([]byte(stderr), []byte("parse yaml")) {
		t.Fatalf("expected stderr to include parse yaml error, got %q", stderr)
	}
}

func runCLIProcess(t *testing.T, args ...string) (string, string, int) {
	t.Helper()

	cmdArgs := append([]string{"-test.run=TestCLIHelperProcess", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "NO_COLOR=1")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run helper process: %v", err)
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	sep := -1
	for i, arg := range os.Args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == -1 {
		t.Fatal("missing argument separator")
	}

	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	rootCmd.SetArgs(os.Args[sep+1:])
	Execute()
	os.Exit(0)
}
