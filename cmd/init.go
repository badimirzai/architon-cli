package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/badimirzai/architon-cli/templates"
	"github.com/spf13/cobra"
)

const (
	architonDirName  = ".architon"
	architonMetaYAML = `# Architon metadata file
# Defines electrical semantics for topology verification

version: "0"

sources:
  # - net: "VBAT"
  #   voltage: 24.0

regulators:
  # - ref: "U3"
  #   in_pin: "VIN"
  #   out_pin: "VOUT"
  #   out_voltage: 5.0

components:
  # - ref: "U1"
  #   max_voltage: 5.5
`
	architonReadme = `This directory contains Architon project metadata.

Edit meta.yaml to define voltage sources and component limits.
`
)

var architonFiles = []architonFileTemplate{
	{name: "meta.yaml", contents: architonMetaYAML},
	{name: "README.md", contents: architonReadme},
}

var initCmd = newInitCmd()

type architonFileTemplate struct {
	name     string
	contents string
}

type initProjectResult struct {
	createdDir bool
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Architon metadata or write a starter robot spec",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, _ := cmd.Flags().GetBool("list")
			templateName, _ := cmd.Flags().GetString("template")
			templateName = strings.TrimSpace(templateName)
			outPathChanged := cmd.Flags().Changed("out")

			if list || templateName != "" || outPathChanged {
				return runTemplateInit(cmd, list, templateName)
			}

			force, _ := cmd.Flags().GetBool("force")

			result, err := initializeArchitonProject(force)
			if err != nil {
				return fatalError(err)
			}

			switch {
			case force && !result.createdDir:
				fmt.Fprintln(cmd.OutOrStdout(), "Reinitialized Architon project in .architon/")
			case result.createdDir:
				fmt.Fprintln(cmd.OutOrStdout(), "Initialized Architon project in .architon/")
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Architon project already initialized.")
			}
			return nil
		},
	}

	cmd.Flags().String("template", "", "Template name")
	cmd.Flags().String("out", "robot.yaml", "Output path for template mode")
	cmd.Flags().Bool("list", false, "List available templates")
	cmd.Flags().Bool("force", false, "Overwrite output files if they already exist")

	return cmd
}

func runTemplateInit(cmd *cobra.Command, list bool, templateName string) error {
	if list {
		for _, name := range templates.Names() {
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		return nil
	}

	if templateName == "" {
		if err := cmd.Help(); err != nil {
			return err
		}
		return userError(fmt.Errorf("missing --template (use --list to see available templates)"))
	}

	outPath, _ := cmd.Flags().GetString("out")
	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		outPath = "robot.yaml"
	}

	if info, err := os.Stat(outPath); err == nil && info != nil {
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			return userError(fmt.Errorf("output file exists: %s (use --force to overwrite)", outPath))
		}
	} else if err != nil && !os.IsNotExist(err) {
		return userError(fmt.Errorf("check output file: %w", err))
	}

	data, err := templates.Load(templateName)
	if err != nil {
		return userError(err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return userError(fmt.Errorf("write template: %w", err))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s (template: %s)\n", outPath, templateName)
	return nil
}

func initializeArchitonProject(force bool) (initProjectResult, error) {
	targetDir := filepath.Clean(architonDirName)

	info, err := os.Stat(targetDir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return initProjectResult{}, fmt.Errorf("%s exists and is not a directory", targetDir)
		}
		return initializeExistingArchitonProject(targetDir, force)
	case os.IsNotExist(err):
		if err := initializeNewArchitonProject(targetDir); err != nil {
			return initProjectResult{}, err
		}
		return initProjectResult{createdDir: true}, nil
	default:
		return initProjectResult{}, fmt.Errorf("stat %s: %w", targetDir, err)
	}
}

func initializeNewArchitonProject(targetDir string) error {
	parentDir := filepath.Dir(targetDir)
	stagingDir, err := os.MkdirTemp(parentDir, ".architon.tmp-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	if err := os.Chmod(stagingDir, 0o755); err != nil {
		return fmt.Errorf("set staging directory permissions: %w", err)
	}
	if err := writeArchitonFiles(stagingDir, architonFiles); err != nil {
		return err
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		return fmt.Errorf("finalize %s: %w", targetDir, err)
	}
	return nil
}

func initializeExistingArchitonProject(targetDir string, force bool) (initProjectResult, error) {
	filesToWrite, err := architonFilesToWrite(targetDir, force)
	if err != nil {
		return initProjectResult{}, err
	}
	if len(filesToWrite) == 0 {
		return initProjectResult{}, nil
	}

	stagingDir, err := os.MkdirTemp(targetDir, ".architon.tmp-*")
	if err != nil {
		return initProjectResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	if err := writeArchitonFiles(stagingDir, filesToWrite); err != nil {
		return initProjectResult{}, err
	}

	backups, err := backupArchitonTargets(targetDir, stagingDir, filesToWrite)
	if err != nil {
		return initProjectResult{}, err
	}

	if err := applyArchitonFiles(targetDir, stagingDir, filesToWrite); err != nil {
		rollbackErr := rollbackArchitonFiles(targetDir, filesToWrite, backups)
		if rollbackErr != nil {
			return initProjectResult{}, fmt.Errorf("%v (rollback failed: %v)", err, rollbackErr)
		}
		return initProjectResult{}, err
	}

	return initProjectResult{}, nil
}

func architonFilesToWrite(targetDir string, force bool) ([]architonFileTemplate, error) {
	filesToWrite := make([]architonFileTemplate, 0, len(architonFiles))
	for _, file := range architonFiles {
		targetPath := filepath.Join(targetDir, file.name)
		info, err := os.Stat(targetPath)
		switch {
		case err == nil:
			if info.IsDir() {
				return nil, fmt.Errorf("%s is a directory", targetPath)
			}
			if force {
				filesToWrite = append(filesToWrite, file)
			}
		case os.IsNotExist(err):
			filesToWrite = append(filesToWrite, file)
		default:
			return nil, fmt.Errorf("stat %s: %w", targetPath, err)
		}
	}
	return filesToWrite, nil
}

func writeArchitonFiles(root string, files []architonFileTemplate) error {
	for _, file := range files {
		targetPath := filepath.Join(root, file.name)
		if err := os.WriteFile(targetPath, []byte(file.contents), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}
	}
	return nil
}

func backupArchitonTargets(targetDir string, stagingDir string, files []architonFileTemplate) (map[string]string, error) {
	backups := make(map[string]string, len(files))
	for _, file := range files {
		targetPath := filepath.Join(targetDir, file.name)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			if rollbackErr := restoreArchitonBackups(targetDir, files, backups); rollbackErr != nil {
				return nil, fmt.Errorf("stat %s: %v (rollback failed: %v)", targetPath, err, rollbackErr)
			}
			return nil, fmt.Errorf("stat %s: %w", targetPath, err)
		}

		backupPath := filepath.Join(stagingDir, file.name+".bak")
		if err := os.Rename(targetPath, backupPath); err != nil {
			if rollbackErr := restoreArchitonBackups(targetDir, files, backups); rollbackErr != nil {
				return nil, fmt.Errorf("backup %s: %v (rollback failed: %v)", targetPath, err, rollbackErr)
			}
			return nil, fmt.Errorf("backup %s: %w", targetPath, err)
		}
		backups[file.name] = backupPath
	}
	return backups, nil
}

func applyArchitonFiles(targetDir string, stagingDir string, files []architonFileTemplate) error {
	for _, file := range files {
		sourcePath := filepath.Join(stagingDir, file.name)
		targetPath := filepath.Join(targetDir, file.name)
		if err := os.Rename(sourcePath, targetPath); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}
	}
	return nil
}

func rollbackArchitonFiles(targetDir string, files []architonFileTemplate, backups map[string]string) error {
	for i := len(files) - 1; i >= 0; i-- {
		targetPath := filepath.Join(targetDir, files[i].name)
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s during rollback: %w", targetPath, err)
		}
	}
	return restoreArchitonBackups(targetDir, architonFiles, backups)
}

func restoreArchitonBackups(targetDir string, files []architonFileTemplate, backups map[string]string) error {
	for i := len(files) - 1; i >= 0; i-- {
		backupPath, ok := backups[files[i].name]
		if !ok {
			continue
		}
		targetPath := filepath.Join(targetDir, files[i].name)
		if err := os.Rename(backupPath, targetPath); err != nil {
			return fmt.Errorf("restore %s: %w", targetPath, err)
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(initCmd)
}
