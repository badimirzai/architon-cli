package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHardwareWorkflowFile(t *testing.T) {
	root := repoRoot(t)
	workflowPath := filepath.Join(root, "dist", "github", "architon.yaml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse workflow YAML: %v", err)
	}

	body := string(data)
	for _, want := range []string{
		"ghcr.io/badimirzai/architon:v0.17.0",
		"rv export",
		".architon/studio/report.json",
		".architon/studio/graph.json",
		"https://studio.architon.io/?github=",
		"RV_SOURCE_REVISION",
		"architon-studio",
		"if: always()",
		"pull-requests: write",
		"contents: read",
		"github.event_name == 'pull_request'",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("workflow %s missing %q", workflowPath, want)
		}
	}

	example, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "architon-example.yml"))
	if err != nil {
		t.Fatalf("read example workflow: %v", err)
	}
	exampleBody := string(example)
	if !strings.Contains(exampleBody, "go install ./cmd/rv") {
		t.Fatal("example workflow should keep compiling rv from this repository")
	}
	if strings.Contains(exampleBody, "ghcr.io/badimirzai/architon:v0.17.0") {
		t.Fatal("example workflow should stay separate from the hardware image pin")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file path")
	}
	return filepath.Join(filepath.Dir(file), "..")
}
