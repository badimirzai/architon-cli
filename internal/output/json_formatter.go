package output

import (
	"encoding/json"
	"strings"

	"github.com/badimirzai/architon-cli/internal/ui"
)

// FormatJSON renders the payload as JSON, optionally pretty printed.
func FormatJSON(payload any, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(payload, "", "    ")
	}
	return json.Marshal(payload)
}

// ColorizeJSON adds ANSI colors to severity values for terminal display.
func ColorizeJSON(raw []byte) []byte {
	s := string(raw)
	s = strings.ReplaceAll(s, `"severity":"ERROR"`, `"severity":"`+ui.Colorize("ERROR", "ERROR")+`"`)
	s = strings.ReplaceAll(s, `"severity":"WARN"`, `"severity":"`+ui.Colorize("WARN", "WARN")+`"`)
	s = strings.ReplaceAll(s, `"severity":"INFO"`, `"severity":"`+ui.Colorize("INFO", "INFO")+`"`)
	s = strings.ReplaceAll(s, `"severity": "ERROR"`, `"severity": "`+ui.Colorize("ERROR", "ERROR")+`"`)
	s = strings.ReplaceAll(s, `"severity": "WARN"`, `"severity": "`+ui.Colorize("WARN", "WARN")+`"`)
	s = strings.ReplaceAll(s, `"severity": "INFO"`, `"severity": "`+ui.Colorize("INFO", "INFO")+`"`)
	return []byte(s)
}
