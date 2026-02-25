package cmd

import "testing"

func TestSelectRenderMode_DefaultsByTTY(t *testing.T) {
	t.Parallel()

	modeTTY, err := selectRenderMode(renderModeOptions{
		OutputFormat: "text",
		StdoutIsTTY:  true,
	})
	if err != nil {
		t.Fatalf("selectRenderMode tty: %v", err)
	}
	if modeTTY != renderModeReport {
		t.Fatalf("expected report mode for tty, got %q", modeTTY)
	}

	modePipe, err := selectRenderMode(renderModeOptions{
		OutputFormat: "text",
		StdoutIsTTY:  false,
	})
	if err != nil {
		t.Fatalf("selectRenderMode non-tty: %v", err)
	}
	if modePipe != renderModeClassic {
		t.Fatalf("expected classic mode for non-tty, got %q", modePipe)
	}
}

func TestSelectRenderMode_StyleOverridesTTY(t *testing.T) {
	t.Parallel()

	mode, err := selectRenderMode(renderModeOptions{
		OutputFormat:  "text",
		Style:         "classic",
		StyleProvided: true,
		StdoutIsTTY:   true,
	})
	if err != nil {
		t.Fatalf("selectRenderMode: %v", err)
	}
	if mode != renderModeClassic {
		t.Fatalf("expected classic mode override, got %q", mode)
	}

	mode, err = selectRenderMode(renderModeOptions{
		OutputFormat:  "text",
		Style:         "report",
		StyleProvided: true,
		StdoutIsTTY:   false,
	})
	if err != nil {
		t.Fatalf("selectRenderMode: %v", err)
	}
	if mode != renderModeReport {
		t.Fatalf("expected report mode override, got %q", mode)
	}
}

func TestSelectRenderMode_JSONOverridesStyle(t *testing.T) {
	t.Parallel()

	mode, err := selectRenderMode(renderModeOptions{
		OutputFormat:  "json",
		Style:         "classic",
		StyleProvided: true,
		StdoutIsTTY:   true,
	})
	if err != nil {
		t.Fatalf("selectRenderMode: %v", err)
	}
	if mode != renderModeJSON {
		t.Fatalf("expected json mode, got %q", mode)
	}
}

func TestSelectRenderMode_InvalidStyle(t *testing.T) {
	t.Parallel()

	_, err := selectRenderMode(renderModeOptions{
		OutputFormat:  "text",
		Style:         "wide",
		StyleProvided: true,
		StdoutIsTTY:   true,
	})
	if err == nil {
		t.Fatal("expected invalid style error")
	}
}
