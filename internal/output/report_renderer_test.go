package output

import (
	"strings"
	"testing"

	"github.com/badimirzai/robotics-verifier-cli/internal/ui"
	"github.com/badimirzai/robotics-verifier-cli/internal/validate"
)

func TestReportRenderer_VerdictLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		findings []validate.Finding
		want     string
	}{
		{
			name: "fail_when_errors_exist",
			findings: []validate.Finding{
				{Severity: validate.SevError, Code: "DRV_SUPPLY_RANGE", Message: "error"},
				{Severity: validate.SevWarn, Code: "DRV_CONT_LOW_MARGIN", Message: "warn"},
			},
			want: "Result: FAIL — architecture violations detected",
		},
		{
			name: "warn_when_only_warnings_exist",
			findings: []validate.Finding{
				{Severity: validate.SevWarn, Code: "DRV_CONT_LOW_MARGIN", Message: "warn"},
				{Severity: validate.SevInfo, Code: "DRV_CHANNELS_OK", Message: "info"},
			},
			want: "Result: WARN — architecture risks detected",
		},
		{
			name: "ok_when_no_errors_or_warnings",
			findings: []validate.Finding{
				{Severity: validate.SevInfo, Code: "DRV_CHANNELS_OK", Message: "info"},
			},
			want: "Result: OK — no architecture violations detected",
		},
	}

	renderer := ReportRenderer{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := renderer.Render(CheckResult{
				Target: "robot.yaml",
				Report: validate.Report{Findings: tt.findings},
			}, RenderOptions{Width: 92})

			if !strings.HasPrefix(out, "Analyzing system architecture...\n\nARCHITON CHECK\n") {
				t.Fatalf("expected analysis intro and header prefix, got:\n%s", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("expected result line %q, got:\n%s", tt.want, out)
			}
			if !strings.Contains(out, "(errors:") {
				t.Fatalf("expected counts line, got:\n%s", out)
			}
		})
	}
}

func TestReportRenderer_GroupingOrder(t *testing.T) {
	t.Parallel()

	report := validate.Report{
		Findings: []validate.Finding{
			{Severity: validate.SevInfo, Code: "DRV_CHANNELS_OK", Message: "info message"},
			{Severity: validate.SevError, Code: "DRV_SUPPLY_RANGE", Message: "error message"},
			{Severity: validate.SevWarn, Code: "DRV_CONT_LOW_MARGIN", Message: "warn message"},
		},
	}

	out := ReportRenderer{}.Render(CheckResult{
		Target: "robot.yaml",
		Report: report,
	}, RenderOptions{Width: 92})

	hardStops := strings.Index(out, "HARD STOPS")
	risks := strings.Index(out, "RISKS")
	notes := strings.Index(out, "NOTES")
	if hardStops == -1 || risks == -1 || notes == -1 {
		t.Fatalf("missing expected sections:\n%s", out)
	}
	if !(hardStops < risks && risks < notes) {
		t.Fatalf("expected HARD STOPS -> RISKS -> NOTES ordering, got:\n%s", out)
	}

	errFinding := strings.Index(out, "[X] DRV_SUPPLY_RANGE")
	warnFinding := strings.Index(out, "[!] DRV_CONT_LOW_MARGIN")
	infoFinding := strings.Index(out, "[i] DRV_CHANNELS_OK")
	if !(errFinding < warnFinding && warnFinding < infoFinding) {
		t.Fatalf("expected ERROR -> WARN -> INFO finding order, got:\n%s", out)
	}
}

func TestReportRenderer_FixHintIncluded(t *testing.T) {
	t.Parallel()

	out := ReportRenderer{}.Render(CheckResult{
		Target: "robot.yaml",
		Report: validate.Report{
			Findings: []validate.Finding{
				{Severity: validate.SevError, Code: "DRV_SUPPLY_RANGE", Message: "battery mismatch"},
			},
		},
	}, RenderOptions{Width: 92})

	if !strings.Contains(out, "Fix: Align battery voltage with driver supply range, or use compatible driver.") {
		t.Fatalf("expected DRV_SUPPLY_RANGE fix hint, got:\n%s", out)
	}
}

func TestReportRenderer_WrapWidthMax92(t *testing.T) {
	t.Parallel()

	out := ReportRenderer{}.Render(CheckResult{
		Target: "robot.yaml",
		Report: validate.Report{
			Findings: []validate.Finding{
				{
					Severity: validate.SevWarn,
					Code:     "DRV_CONT_LOW_MARGIN",
					Message:  "driver continuous rating 1.00A is below recommended 1.25A for motor 12V DC gearmotor nominal 1.00A under sustained high duty cycle with limited airflow near the driver board",
				},
			},
		},
	}, RenderOptions{Width: 92})

	for _, line := range strings.Split(out, "\n") {
		if len(line) > 92 {
			t.Fatalf("line exceeds width 92 (%d): %q", len(line), line)
		}
	}
}

func TestReportRenderer_ColorScopeOnlySectionAndCode(t *testing.T) {
	originalColors := ui.DefaultColorEnabled()
	ui.EnableColors(true)
	defer ui.EnableColors(originalColors)

	out := ReportRenderer{}.Render(CheckResult{
		Target: "robot.yaml",
		Report: validate.Report{
			Findings: []validate.Finding{
				{
					Severity: validate.SevError,
					Code:     "DRV_SUPPLY_RANGE",
					Message:  "battery 12.00V outside motor_driver motor supply range [18.00, 24.00]V",
				},
			},
		},
	}, RenderOptions{Width: 92})

	if !strings.Contains(out, "\x1b[31mHARD STOPS (must fix)\x1b[0m") {
		t.Fatalf("expected HARD STOPS section token to be red, got:\n%s", out)
	}
	if !strings.Contains(out, "Result: \x1b[31mFAIL\x1b[0m — architecture violations detected") {
		t.Fatalf("expected FAIL verdict token to be red, got:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[31m  [X] DRV_SUPPLY_RANGE\x1b[0m") {
		t.Fatalf("expected code heading to be red, got:\n%s", out)
	}
	if strings.Contains(out, "\x1b[31m    battery 12.00V") {
		t.Fatalf("did not expect message text to be colorized, got:\n%s", out)
	}
	if strings.Contains(out, "\x1b[31m    Fix:") {
		t.Fatalf("did not expect fix text to be colorized, got:\n%s", out)
	}
}

func TestReportRenderer_StripsDerivationSuffixInMessage(t *testing.T) {
	t.Parallel()

	out := ReportRenderer{}.Render(CheckResult{
		Target: "robot.yaml",
		Report: validate.Report{
			Findings: []validate.Finding{
				{
					Severity: validate.SevError,
					Code:     "BATT_PEAK_OVER_C",
					Message:  "Peak current 16.00A exceeds battery max 10.00A (2.00Ah * 5.00C)",
				},
			},
		},
	}, RenderOptions{Width: 92})

	if !strings.Contains(out, "Peak current 16.00A exceeds battery max 10.00A") {
		t.Fatalf("expected simplified battery message, got:\n%s", out)
	}
	if strings.Contains(out, "(2.00Ah * 5.00C)") {
		t.Fatalf("did not expect derivation suffix in report output, got:\n%s", out)
	}
}
