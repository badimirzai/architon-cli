package meta

import (
	"os"

	"gopkg.in/yaml.v3"
)

func Parse(path string) (*Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Version == "" {
		m.Version = "0"
	}
	return &m, nil
}
