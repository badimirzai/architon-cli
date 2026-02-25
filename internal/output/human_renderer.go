package output

import "github.com/badimirzai/robotics-verifier-cli/internal/validate"

// HumanRenderer renders human-readable output.
type HumanRenderer interface {
	Render(result CheckResult, opts RenderOptions) string
}

// CheckResult holds all data required by human renderers.
type CheckResult struct {
	Target   string
	Report   validate.Report
	ExitCode int
}

// RenderOptions controls human output formatting.
type RenderOptions struct {
	Width int
}
