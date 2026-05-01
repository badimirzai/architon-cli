package importers

import (
	"fmt"

	"github.com/badimirzai/architon-cli/internal/ir"
)

// Importer adapts an EDA/project format into the stable DesignIR schema.
// Importers are allowed to parse vendor-specific files; rule packages are not.
type Importer interface {
	Name() string
	Detect(path string) bool
	Import(path string) (*ir.DesignIR, error)
}

// Detect returns the first importer that recognizes path.
func Detect(path string, candidates []Importer) (Importer, bool) {
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.Detect(path) {
			return candidate, true
		}
	}
	return nil, false
}

// Import detects and imports path using one of candidates.
func Import(path string, candidates []Importer) (*ir.DesignIR, Importer, error) {
	importer, ok := Detect(path, candidates)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported input format for %q", path)
	}
	design, err := importer.Import(path)
	if err != nil {
		return nil, importer, err
	}
	return design, importer, nil
}
