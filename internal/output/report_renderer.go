package output

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/badimirzai/robotics-verifier-cli/internal/ui"
	"github.com/badimirzai/robotics-verifier-cli/internal/validate"
)

const defaultReportWidth = 92

type Verdict string

const (
	verdictFail Verdict = "FAIL"
	verdictWarn Verdict = "WARN"
	verdictOK   Verdict = "OK"
)

// ReportRenderer renders the structured interactive report format.
type ReportRenderer struct{}

// Render renders a report in structured report mode.
func (ReportRenderer) Render(result CheckResult, opts RenderOptions) string {
	width := opts.Width
	if width <= 0 {
		width = defaultReportWidth
	}

	summary := summarizeFindings(result.Report)
	verdict := verdictForSummary(summary)

	var b strings.Builder
	b.WriteString("Analyzing system architecture...\n\n")
	b.WriteString("ARCHITON CHECK\n")
	b.WriteString(fmt.Sprintf("Target: %s\n", result.Target))
	b.WriteString(
		fmt.Sprintf(
			"Result: %s — %s\n",
			colorizeVerdict(verdict),
			resultExplanation(verdict),
		),
	)
	b.WriteString(
		fmt.Sprintf(
			"(errors: %d, warnings: %d, notes: %d)\n",
			summary.Errors,
			summary.Warnings,
			summary.Notes,
		),
	)

	sections := []struct {
		Title    string
		Severity validate.Severity
	}{
		{Title: "HARD STOPS (must fix)", Severity: validate.SevError},
		{Title: "RISKS (recommended fixes)", Severity: validate.SevWarn},
		{Title: "NOTES (informational)", Severity: validate.SevInfo},
	}

	grouped := groupBySeverity(result.Report.Findings)
	for _, section := range sections {
		findings := grouped[section.Severity]
		if len(findings) == 0 {
			continue
		}
		b.WriteString("\n")
		b.WriteString(ui.Colorize(severityToken(section.Severity), section.Title))
		b.WriteString("\n")
		for i, f := range findings {
			heading := fmt.Sprintf("  %s %s", iconForSeverity(f.Severity), f.Code)
			b.WriteString(ui.Colorize(severityToken(f.Severity), heading))
			b.WriteString("\n")
			writeWrappedLine(&b, stripDerivationSuffix(f.Message), width, "    ")
			if hint, ok := FixHint(f.Code); ok {
				writeWrappedLine(&b, "Fix: "+hint, width, "    ")
			}
			if i < len(findings)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("exit code: %d\n", result.ExitCode))
	return b.String()
}

type findingSummary struct {
	Errors   int
	Warnings int
	Notes    int
}

func summarizeFindings(report validate.Report) findingSummary {
	var summary findingSummary
	for _, f := range report.Findings {
		switch f.Severity {
		case validate.SevError:
			summary.Errors++
		case validate.SevWarn:
			summary.Warnings++
		case validate.SevInfo:
			summary.Notes++
		}
	}
	return summary
}

func verdictForSummary(summary findingSummary) Verdict {
	if summary.Errors > 0 {
		return verdictFail
	}
	if summary.Warnings > 0 {
		return verdictWarn
	}
	return verdictOK
}

func groupBySeverity(findings []validate.Finding) map[validate.Severity][]validate.Finding {
	grouped := map[validate.Severity][]validate.Finding{
		validate.SevError: {},
		validate.SevWarn:  {},
		validate.SevInfo:  {},
	}
	for _, f := range findings {
		grouped[f.Severity] = append(grouped[f.Severity], f)
	}
	return grouped
}

func iconForSeverity(severity validate.Severity) string {
	switch severity {
	case validate.SevError:
		return "[X]"
	case validate.SevWarn:
		return "[!]"
	default:
		return "[i]"
	}
}

func writeWrappedLine(b *strings.Builder, text string, width int, indent string) {
	lineWidth := width - len(indent)
	if lineWidth < 1 {
		lineWidth = 1
	}
	lines := wrapText(text, lineWidth)
	for _, line := range lines {
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func severityToken(severity validate.Severity) string {
	switch severity {
	case validate.SevError:
		return "ERROR"
	case validate.SevWarn:
		return "WARN"
	default:
		return "INFO"
	}
}

func colorizeVerdict(verdict Verdict) string {
	switch verdict {
	case verdictFail:
		return ui.Colorize("ERROR", string(verdict))
	case verdictWarn:
		return ui.Colorize("WARN", string(verdict))
	default:
		return ui.Colorize("OK", string(verdict))
	}
}

func resultExplanation(verdict Verdict) string {
	switch verdict {
	case verdictFail:
		return "architecture violations detected"
	case verdictWarn:
		return "architecture risks detected"
	default:
		return "no architecture violations detected"
	}
}

func stripDerivationSuffix(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || !strings.HasSuffix(trimmed, ")") {
		return message
	}

	closing := len(trimmed) - 1
	opening := strings.LastIndex(trimmed, "(")
	if opening <= 0 || trimmed[opening-1] != ' ' {
		return message
	}

	content := strings.TrimSpace(trimmed[opening+1 : closing])
	if !looksLikeDerivation(content) {
		return message
	}
	return strings.TrimSpace(trimmed[:opening])
}

func looksLikeDerivation(content string) bool {
	hasDigit := false
	hasOperator := false
	for _, r := range content {
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		switch r {
		case '*', 'x', 'X', '+', '/', '=':
			hasOperator = true
		}
	}
	return hasDigit && hasOperator
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if strings.TrimSpace(text) == "" {
		return []string{""}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, len(words))
	current := ""
	for _, word := range words {
		for len(word) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, word[:width])
			word = word[width:]
		}
		if current == "" {
			current = word
			continue
		}
		if len(current)+1+len(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
