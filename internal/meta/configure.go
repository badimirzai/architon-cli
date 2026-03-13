package meta

import "strings"

func IsConfigured(m *Meta) bool {
	hasSource := false
	for _, s := range m.Sources {
		if strings.TrimSpace(s.Net) != "" && s.Voltage > 0 {
			hasSource = true
			break
		}
	}
	hasComponent := false
	for _, c := range m.Components {
		if strings.TrimSpace(c.Ref) != "" && c.MaxVoltage > 0 {
			hasComponent = true
			break
		}
	}
	return hasSource && hasComponent
}
