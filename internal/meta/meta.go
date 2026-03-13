package meta

type Meta struct {
	Version    string      `yaml:"version"`
	Sources    []Source    `yaml:"sources"`
	Regulators []Regulator `yaml:"regulators,omitempty"`
	Components []Component `yaml:"components,omitempty"`
}

type Source struct {
	Net     string  `yaml:"net"`
	Voltage float64 `yaml:"voltage"`
}

type Regulator struct {
	Ref        string  `yaml:"ref"`
	InPin      string  `yaml:"in_pin"`
	OutPin     string  `yaml:"out_pin"`
	OutVoltage float64 `yaml:"out_voltage"`
}

type Component struct {
	Ref        string  `yaml:"ref"`
	MaxVoltage float64 `yaml:"max_voltage"`
}

func Load(path string) (*Meta, error) {
	m, err := Parse(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateStrict(m); err != nil {
		return nil, err
	}
	return m, nil
}
