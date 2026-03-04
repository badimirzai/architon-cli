package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/badimirzai/architon-cli/templates"
)

func runInitCommand(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	cmd := newInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return buf.String(), err
}

func readArchitonFile(t *testing.T, cwd string, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(cwd, architonDirName, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func writeArchitonFile(t *testing.T, cwd string, name string, contents string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(cwd, architonDirName, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestInitFreshDirectoryCreatesArchitonProject(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runInitCommand(t, tmpDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Initialized Architon project in .architon/") {
		t.Fatalf("expected initialization message, got %q", stdout)
	}

	info, err := os.Stat(filepath.Join(tmpDir, architonDirName))
	if err != nil {
		t.Fatalf("stat .architon: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected .architon to be a directory")
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected .architon permissions 0755, got %o", info.Mode().Perm())
	}
	if got := readArchitonFile(t, tmpDir, "meta.yaml"); got != architonMetaYAML {
		t.Fatalf("unexpected meta.yaml contents:\n%s", got)
	}
	if got := readArchitonFile(t, tmpDir, "README.md"); got != architonReadme {
		t.Fatalf("unexpected README.md contents:\n%s", got)
	}
}

func TestInitRerunWithoutForceLeavesFilesUnchanged(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := runInitCommand(t, tmpDir); err != nil {
		t.Fatalf("initial init failed: %v", err)
	}

	writeArchitonFile(t, tmpDir, "meta.yaml", "custom meta\n")
	writeArchitonFile(t, tmpDir, "README.md", "custom readme\n")

	stdout, err := runInitCommand(t, tmpDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Architon project already initialized.") {
		t.Fatalf("expected already-initialized message, got %q", stdout)
	}
	if got := readArchitonFile(t, tmpDir, "meta.yaml"); got != "custom meta\n" {
		t.Fatalf("expected meta.yaml to remain unchanged, got %q", got)
	}
	if got := readArchitonFile(t, tmpDir, "README.md"); got != "custom readme\n" {
		t.Fatalf("expected README.md to remain unchanged, got %q", got)
	}
}

func TestInitRerunWithForceOverwritesFiles(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := runInitCommand(t, tmpDir); err != nil {
		t.Fatalf("initial init failed: %v", err)
	}

	writeArchitonFile(t, tmpDir, "meta.yaml", "custom meta\n")
	writeArchitonFile(t, tmpDir, "README.md", "custom readme\n")

	stdout, err := runInitCommand(t, tmpDir, "--force")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Reinitialized Architon project in .architon/") {
		t.Fatalf("expected reinitialization message, got %q", stdout)
	}
	if got := readArchitonFile(t, tmpDir, "meta.yaml"); got != architonMetaYAML {
		t.Fatalf("expected meta.yaml to be overwritten, got %q", got)
	}
	if got := readArchitonFile(t, tmpDir, "README.md"); got != architonReadme {
		t.Fatalf("expected README.md to be overwritten, got %q", got)
	}
}

func TestInitDirectoryAlreadyExistsBeforeInit(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.Mkdir(filepath.Join(tmpDir, architonDirName), 0o755); err != nil {
		t.Fatalf("mkdir .architon: %v", err)
	}

	stdout, err := runInitCommand(t, tmpDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Architon project already initialized.") {
		t.Fatalf("expected already-initialized message, got %q", stdout)
	}
	if got := readArchitonFile(t, tmpDir, "meta.yaml"); got != architonMetaYAML {
		t.Fatalf("unexpected meta.yaml contents:\n%s", got)
	}
	if got := readArchitonFile(t, tmpDir, "README.md"); got != architonReadme {
		t.Fatalf("unexpected README.md contents:\n%s", got)
	}
}

func TestInitListTemplates(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runInitCommand(t, tmpDir, "--list")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for _, name := range templates.Names() {
		if !strings.Contains(stdout, name+"\n") {
			t.Fatalf("expected template %q in output, got %q", name, stdout)
		}
	}
}

func TestInitTemplateWritesDefaultOutput(t *testing.T) {
	tmpDir := t.TempDir()

	stdout, err := runInitCommand(t, tmpDir, "--template", "4wd-problem")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Wrote robot.yaml (template: 4wd-problem)") {
		t.Fatalf("unexpected output: %q", stdout)
	}

	want, err := templates.Load("4wd-problem")
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	got := readFile(t, filepath.Join(tmpDir, "robot.yaml"))
	if got != string(want) {
		t.Fatalf("unexpected robot.yaml contents:\n%s", got)
	}
}

func TestInitTemplateRefusesOverwriteWithoutForce(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "robot.yaml")

	if err := os.WriteFile(target, []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("seed robot.yaml: %v", err)
	}

	_, err := runInitCommand(t, tmpDir, "--template", "4wd-clean")
	if err == nil {
		t.Fatal("expected overwrite error, got nil")
	}
	if !strings.Contains(err.Error(), "output file exists") {
		t.Fatalf("expected output file exists error, got %v", err)
	}
	if got := readFile(t, target); got != "keep me\n" {
		t.Fatalf("expected existing contents to remain, got %q", got)
	}
}

func TestInitTemplateForceOverwritesOutput(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "robot.yaml")

	if err := os.WriteFile(target, []byte("old contents\n"), 0o644); err != nil {
		t.Fatalf("seed robot.yaml: %v", err)
	}

	stdout, err := runInitCommand(t, tmpDir, "--template", "4wd-clean", "--force")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Wrote robot.yaml (template: 4wd-clean)") {
		t.Fatalf("unexpected output: %q", stdout)
	}

	want, err := templates.Load("4wd-clean")
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	if got := readFile(t, target); got != string(want) {
		t.Fatalf("expected robot.yaml to be overwritten, got:\n%s", got)
	}
}
