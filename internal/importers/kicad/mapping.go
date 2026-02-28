package kicad

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ColumnMapping allows explicit CSV header mapping.
type ColumnMapping struct {
	Ref          string `yaml:"ref"`
	Value        string `yaml:"value"`
	Footprint    string `yaml:"footprint"`
	MPN          string `yaml:"mpn"`
	Manufacturer string `yaml:"manufacturer"`
}

// LoadColumnMapping reads a YAML mapping file for BOM header mapping.
func LoadColumnMapping(path string) (ColumnMapping, error) {
	var mapping ColumnMapping
	data, err := os.ReadFile(path)
	if err != nil {
		return mapping, fmt.Errorf("read mapping file: %w", err)
	}
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return mapping, fmt.Errorf("parse mapping YAML: %w", err)
	}
	return mapping, nil
}
