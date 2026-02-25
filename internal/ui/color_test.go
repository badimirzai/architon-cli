package ui

import (
	"strings"
	"testing"
)

func TestColorizeDisabledReturnsOriginal(t *testing.T) {
	EnableColors(false)
	defer EnableColors(DefaultColorEnabled())

	msg := "plain"
	if got := Colorize("ERROR", msg); got != msg {
		t.Fatalf("expected %q, got %q", msg, got)
	}
}

func TestColorizeInfoUsesBlue(t *testing.T) {
	EnableColors(true)
	defer EnableColors(DefaultColorEnabled())

	got := Colorize("INFO", "note")
	if !strings.HasPrefix(got, "\x1b[38;2;144;213;255m") {
		t.Fatalf("expected INFO to use rgb(144,213,255) color prefix, got %q", got)
	}
}
