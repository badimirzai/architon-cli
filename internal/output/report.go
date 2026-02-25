package output

import (
	"fmt"
	"strings"

	"github.com/badimirzai/robotics-verifier-cli/internal/ui"
	"github.com/badimirzai/robotics-verifier-cli/internal/validate"
)

// ClassicRenderer preserves the original human-readable output format.
type ClassicRenderer struct{}

// Render renders a report in classic mode.
func (ClassicRenderer) Render(result CheckResult, _ RenderOptions) string {
	return renderClassicReport(result.Report)
}

func renderClassicReport(r validate.Report) string {
	var b strings.Builder
	b.WriteString(ui.Colorize("HEADER", "rv check"))
	b.WriteString("\n")
	b.WriteString(ui.Colorize("HEADER", "--------------"))
	b.WriteString("\n")
	for _, f := range r.Findings {
		severity := string(f.Severity)
		b.WriteString(ui.Colorize(severity, severity))
		b.WriteString(" ")
		b.WriteString(f.Code)
		b.WriteString(": ")
		if f.Location != nil {
			b.WriteString(f.Location.File)
			if f.Location.Line > 0 {
				b.WriteString(fmt.Sprintf(":%d", f.Location.Line))
			}
			b.WriteString(" ")
		}
		b.WriteString(f.Message)
		b.WriteString("\n")
	}
	return b.String()
}

// RenderReport is kept for compatibility with existing callers.
func RenderReport(r validate.Report) string {
	return renderClassicReport(r)
}
