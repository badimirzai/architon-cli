package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/badimirzai/architon-cli/internal/parts"
)

func buildPartsStore(cliDirs []string, envVar string) (*parts.Store, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	searchDirs := partsSearchDirs(cwd, cliDirs, envVar)
	return parts.NewStoreWithDirs(searchDirs), nil
}

func partsSearchDirs(cwd string, cliDirs []string, envVar string) []string {
	dirs := []string{
		filepath.Join(cwd, "rv_parts"),
		filepath.Join(cwd, "parts"),
	}

	for _, dir := range cliDirs {
		if dir == "" {
			continue
		}
		dirs = append(dirs, filepath.Clean(dir))
	}

	dirs = append(dirs, parsePartsEnvDirs(envVar)...)
	return dirs
}

func parsePartsEnvDirs(envVar string) []string {
	if envVar == "" {
		return nil
	}
	rawDirs := strings.Split(envVar, string(filepath.ListSeparator))
	dirs := make([]string, 0, len(rawDirs))
	for _, dir := range rawDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dirs = append(dirs, filepath.Clean(dir))
	}
	return dirs
}
