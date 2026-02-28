package output

import "github.com/badimirzai/architon-cli/internal/validate"

type jsonLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type jsonFinding struct {
	ID       string                 `json:"id"`
	Severity string                 `json:"severity"`
	Message  string                 `json:"message"`
	Path     *string                `json:"path"`
	Location *jsonLocation          `json:"location"`
	Meta     map[string]interface{} `json:"meta"`
}

type jsonSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
	ExitCode int `json:"exit_code"`
}

type Debug struct {
	InternalError string `json:"internal_error"`
	Stacktrace    string `json:"stacktrace"`
}

type jsonReport struct {
	SpecFile string        `json:"spec_file"`
	Summary  jsonSummary   `json:"summary"`
	Findings []jsonFinding `json:"findings"`
	Debug    *Debug        `json:"debug,omitempty"`
}

// RenderJSONReport renders a report to JSON with the required schema.
func RenderJSONReport(specFile string, report validate.Report, exitCode int, debug *Debug) (any, jsonSummary, error) {
	findings := make([]jsonFinding, 0, len(report.Findings))
	summary := jsonSummary{ExitCode: exitCode}

	for _, f := range report.Findings {
		switch f.Severity {
		case validate.SevError:
			summary.Errors++
		case validate.SevWarn:
			summary.Warnings++
		case validate.SevInfo:
			summary.Infos++
		}

		var path *string
		if f.Path != "" {
			path = &f.Path
		}

		var location *jsonLocation
		if f.Location != nil && f.Location.Line > 0 {
			column := f.Location.Column
			if column == 0 {
				column = 1
			}
			location = &jsonLocation{Line: f.Location.Line, Column: column}
		}

		findings = append(findings, jsonFinding{
			ID:       f.Code,
			Severity: string(f.Severity),
			Message:  f.Message,
			Path:     path,
			Location: location,
			Meta:     map[string]interface{}{},
		})
	}

	payload := jsonReport{
		SpecFile: specFile,
		Summary:  summary,
		Findings: findings,
		Debug:    debug,
	}
	return payload, summary, nil
}

// RenderJSONError builds a JSON error response when findings cannot be produced.
func RenderJSONError(specFile string, exitCode int, message string, debug *Debug) (any, jsonSummary, error) {
	report := jsonReport{
		SpecFile: specFile,
		Summary: jsonSummary{
			Errors:   1,
			Warnings: 0,
			Infos:    0,
			ExitCode: exitCode,
		},
		Findings: []jsonFinding{
			{
				ID:       "PARSER_ERROR",
				Severity: string(validate.SevError),
				Message:  message,
				Path:     nil,
				Location: nil,
				Meta:     map[string]interface{}{},
			},
		},
		Debug: debug,
	}
	return report, report.Summary, nil
}
