package meta

import (
	"errors"
	"fmt"
	"strings"
)

func ValidateStrict(m *Meta) error {
	if m == nil {
		return errors.New("meta: config must not be nil")
	}
	for _, s := range m.Sources {
		if strings.TrimSpace(s.Net) == "" {
			return errors.New("meta: source net must not be empty")
		}
		if s.Voltage <= 0 {
			return fmt.Errorf("meta: source voltage must be > 0 for net %s", s.Net)
		}
	}

	for _, r := range m.Regulators {
		if strings.TrimSpace(r.Ref) == "" {
			return errors.New("meta: regulator ref must not be empty")
		}
		if strings.TrimSpace(r.InPin) == "" || strings.TrimSpace(r.OutPin) == "" {
			return fmt.Errorf("meta: regulator %s in_pin/out_pin must not be empty", r.Ref)
		}
		if r.OutVoltage <= 0 {
			return fmt.Errorf("meta: regulator %s out_voltage must be > 0", r.Ref)
		}
	}

	for _, c := range m.Components {
		if strings.TrimSpace(c.Ref) == "" {
			return errors.New("meta: component ref must not be empty")
		}
		if c.MaxVoltage <= 0 {
			return fmt.Errorf("meta: component %s max_voltage must be > 0", c.Ref)
		}
	}

	return nil
}
