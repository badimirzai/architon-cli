package meta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteSkeleton(path string, urefs []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if len(urefs) > 10 {
		urefs = urefs[:10]
	}

	var b strings.Builder
	b.WriteString(`version: "0"

# Minimal edit:
# 1) Set at least one source voltage (match a net in the netlist)
# 2) Set max_voltage for at least one component
#
# Then run:
#   rv scan .

sources:
  - net: VBAT
    voltage: 0

regulators: []

components:
`)

	if len(urefs) == 0 {
		b.WriteString("  - ref: U1\n    max_voltage: 0\n")
	} else {
		for _, r := range urefs {
			b.WriteString(fmt.Sprintf("  - ref: %s\n    max_voltage: 0\n", r))
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
