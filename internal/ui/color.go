package ui

import (
	"os"
	"strings"
)

const (
	colorRed    = "\x1b[31m"
	colorYellow = "\x1b[33m"
	colorBlue   = "\x1b[38;2;144;213;255m"
	colorGreen  = "\x1b[32m"
	colorReset  = "\x1b[0m"
)

var colorsEnabled = DefaultColorEnabled()

func DefaultColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func EnableColors(enabled bool) {
	colorsEnabled = enabled
}

func Colorize(severity string, msg string) string {
	if !colorsEnabled {
		return msg
	}
	color := colorForSeverity(severity)
	if color == "" {
		return msg
	}
	return color + msg + colorReset
}

func colorForSeverity(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "ERROR":
		return colorRed
	case "WARN":
		return colorYellow
	case "INFO":
		return colorBlue
	case "OK", "HEADER":
		return colorGreen
	default:
		return ""
	}
}
