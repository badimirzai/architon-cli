package templates

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

var (
	//go:embed *.yaml
	templateFS embed.FS

	templateFiles = map[string]string{
		"4wd-clean":          "4wd-clean.yaml",
		"4wd-problem":        "4wd-problem.yaml",
		"robot-over_voltage": "robot-over_voltage.yaml",
	}
)

// Names returns the sorted list of available template names.
func Names() []string {
	names := make([]string, 0, len(templateFiles))
	for name := range templateFiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Load returns the template contents for the given name.
func Load(name string) ([]byte, error) {
	path, ok := templateFiles[name]
	if !ok && strings.HasSuffix(name, ".yaml") {
		path, ok = templateFiles[strings.TrimSuffix(name, ".yaml")]
	}
	if !ok {
		return nil, fmt.Errorf("unknown template %q (use --list to see available templates)", name)
	}
	data, err := templateFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return data, nil
}
