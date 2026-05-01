package kicad

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/badimirzai/architon-cli/internal/ir"
)

const importerName = "kicad"

// Importer adapts KiCad BOM CSV and netlist files into DesignIR.
// All KiCad-specific parsing is kept behind this type.
type Importer struct {
	Mapping ColumnMapping
}

// NewImporter creates a KiCad adapter with optional BOM column mapping.
func NewImporter(mapping ColumnMapping) Importer {
	return Importer{Mapping: mapping}
}

func (i Importer) Name() string {
	return importerName
}

// Detect recognizes supported KiCad files and project directories.
func (i Importer) Detect(path string) bool {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return detectKiCadFile(cleanPath)
	}
	if info.IsDir() {
		bom, _ := findProjectBOM(cleanPath)
		netlist, _ := findProjectNetlist(cleanPath)
		return bom != "" || netlist != ""
	}
	return detectKiCadFile(cleanPath)
}

// Import converts a KiCad file or project directory into normalized DesignIR.
func (i Importer) Import(path string) (*ir.DesignIR, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) && detectKiCadFile(cleanPath) {
			return i.importFile(cleanPath)
		}
		return nil, fmt.Errorf("stat KiCad input: %w", err)
	}
	if info.IsDir() {
		return i.importProject(cleanPath)
	}
	return i.importFile(cleanPath)
}

// importFile keeps single-file dispatch local to the adapter.
func (i Importer) importFile(path string) (*ir.DesignIR, error) {
	switch {
	case isCSV(path):
		design, err := ImportKiCadBOM(path, i.Mapping)
		if err != nil {
			return nil, err
		}
		ensureKiCadSourceInfo(design, "bom_csv", path)
		return design, nil
	case isNetlist(path):
		design, err := ImportKiCadNetlist(path)
		if err != nil {
			return nil, err
		}
		ensureKiCadSourceInfo(design, "netlist_sexpr", path)
		return design, nil
	default:
		return nil, fmt.Errorf("unsupported KiCad input format for %q", path)
	}
}

// importProject merges a discovered BOM and netlist into one project DesignIR.
func (i Importer) importProject(path string) (*ir.DesignIR, error) {
	bomPath, err := findProjectBOM(path)
	if err != nil {
		return nil, err
	}
	netlistPath, err := findProjectNetlist(path)
	if err != nil {
		return nil, err
	}
	if bomPath == "" && netlistPath == "" {
		return nil, errors.New("no KiCad BOM or netlist found")
	}

	var bomDesign *ir.DesignIR
	if bomPath != "" {
		bomDesign, err = ImportKiCadBOM(bomPath, i.Mapping)
		if err != nil {
			return nil, err
		}
	}

	var netlistDesign *ir.DesignIR
	if netlistPath != "" {
		netlistDesign, err = ImportKiCadNetlist(netlistPath)
		if err != nil {
			return nil, err
		}
	}

	switch {
	case bomDesign != nil && netlistDesign != nil:
		return ir.MergeProjectIR(bomDesign, netlistDesign, path, time.Now()), nil
	case bomDesign != nil:
		ensureKiCadSourceInfo(bomDesign, "bom_csv", bomPath)
		return bomDesign, nil
	default:
		ensureKiCadSourceInfo(netlistDesign, "netlist_sexpr", netlistPath)
		return netlistDesign, nil
	}
}

// detectKiCadFile is intentionally conservative: extension detection only.
func detectKiCadFile(path string) bool {
	return isCSV(path) || isNetlist(path)
}

func isCSV(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".csv")
}

func isNetlist(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".net")
}

// ensureKiCadSourceInfo records provenance without exposing KiCad details to rules.
func ensureKiCadSourceInfo(design *ir.DesignIR, format string, input string) {
	if design == nil {
		return
	}
	design.SourceInfo = ir.SourceInfo{
		Importer: importerName,
		Format:   format,
		Input:    input,
		Imported: design.Metadata.ParsedAt,
	}
}

// findProjectBOM mirrors scan discovery so the adapter can also import a
// project directory directly through the generic importer interface.
func findProjectBOM(projectPath string) (string, error) {
	for _, relPath := range [][]string{
		{"bom", "bom.csv"},
		{"bom.csv"},
		{"exports", "bom.csv"},
	} {
		candidate := filepath.Clean(filepath.Join(append([]string{projectPath}, relPath...)...))
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				continue
			}
			return candidate, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat BOM candidate: %w", err)
		}
	}

	for _, dir := range []string{
		filepath.Join(projectPath, "bom"),
		filepath.Join(projectPath, "exports"),
		projectPath,
	} {
		candidates, err := findFiles(dir, func(name string) bool {
			lowerName := strings.ToLower(name)
			return strings.EqualFold(filepath.Ext(name), ".csv") &&
				(strings.HasSuffix(lowerName, ".bom.csv") || strings.Contains(lowerName, "bom"))
		})
		if err != nil {
			return "", err
		}
		if len(candidates) > 0 {
			return candidates[0], nil
		}
	}

	return "", nil
}

// findProjectNetlist discovers the first deterministic KiCad netlist candidate.
func findProjectNetlist(projectPath string) (string, error) {
	for _, dir := range []string{
		filepath.Join(projectPath, "exports"),
		projectPath,
	} {
		candidates, err := findFiles(dir, func(name string) bool {
			return strings.EqualFold(filepath.Ext(name), ".net")
		})
		if err != nil {
			return "", err
		}
		if len(candidates) > 0 {
			return candidates[0], nil
		}
	}
	return "", nil
}

// findFiles returns sorted matches so directory imports are reproducible.
func findFiles(dir string, match func(string) bool) ([]string, error) {
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read KiCad project directory: %w", err)
	}

	matches := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !match(entry.Name()) {
			continue
		}
		matches = append(matches, filepath.Join(dir, entry.Name()))
	}

	sort.Slice(matches, func(i, j int) bool {
		return filepath.ToSlash(matches[i]) < filepath.ToSlash(matches[j])
	})
	return matches, nil
}
