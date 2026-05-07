package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildKiCadNetlistCommand(t *testing.T) {
	binary, args, err := buildKiCadNetlistCommand("/opt/kicad-cli", "/tmp/out.net", "/work/demo.kicad_sch")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if binary != "/opt/kicad-cli" {
		t.Fatalf("expected binary override, got %q", binary)
	}
	wantArgs := []string{
		"sch",
		"export",
		"netlist",
		"--format",
		"kicadsexpr",
		"--output",
		"/tmp/out.net",
		"/work/demo.kicad_sch",
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("unexpected args:\nwant: %#v\n got: %#v", wantArgs, args)
	}
}

func TestResolveKiCadCLIPathUsesCommonMacAppPathWhenNotInPath(t *testing.T) {
	tmpDir := t.TempDir()
	fakeCLI := writeFakeKiCadCLI(t, tmpDir, kicadFixtureData(t, "netlist_simple.net"))

	got, err := resolveKiCadCLIPathWithLookPath(defaultKiCadCLI, []string{fakeCLI}, func(string) (string, error) {
		return "", errors.New("not in path")
	})
	if err != nil {
		t.Fatalf("expected fallback path, got %v", err)
	}
	if got != fakeCLI {
		t.Fatalf("expected %q, got %q", fakeCLI, got)
	}
}

func TestScan_DirectoryInputGeneratesNetlistFromRootSchematic(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "robot.kicad_sch"), "(kicad_sch)")
	fakeCLI := writeFakeKiCadCLI(t, tmpDir, kicadFixtureData(t, "netlist_simple.net"))

	stdout, err := runScanCommand(t, tmpDir, ".", "--kicad-cli", fakeCLI)
	if err != nil {
		t.Fatalf("expected generated netlist scan to succeed, got %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "Generated Netlist: .architon/generated.net\n") {
		t.Fatalf("expected generated netlist line, got %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".architon", "generated.net")); err != nil {
		t.Fatalf("expected generated netlist in project .architon directory, got %v", err)
	}
	if !strings.Contains(stdout, "Parts: 3\n") || !strings.Contains(stdout, "Nets: 2\n") {
		t.Fatalf("expected generated netlist import summary, got %q", stdout)
	}
	if strings.Contains(stdout, "Metadata: inferred\n") {
		t.Fatalf("expected metadata mode to be hidden by default, got %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".architon", "meta.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scan must not write meta.yaml, stat err=%v", err)
	}

	report := readScanReport(t, filepath.Join(tmpDir, defaultScanReportPath))
	if report.Summary.Nets != 2 {
		t.Fatalf("expected generated netlist nets in report, got %d", report.Summary.Nets)
	}
}

func TestScan_DirectoryInputGeneratedNetlistWriteFailureReturnsExitCodeThree(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "robot.kicad_sch"), "(kicad_sch)")
	writeScanTestFile(t, filepath.Join(tmpDir, ".architon"), "not a directory")
	fakeCLI := writeFakeKiCadCLI(t, tmpDir, kicadFixtureData(t, "netlist_simple.net"))

	_, err := runScanCommand(t, tmpDir, ".", "--kicad-cli", fakeCLI)
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}
	if exitErr.Err == nil || !strings.Contains(exitErr.Err.Error(), "generate KiCad netlist .architon/generated.net: create output directory") {
		t.Fatalf("expected generated netlist write error, got %v", exitErr.Err)
	}
}

func TestScan_DirectoryInputNoNetlistNoSchematicReturnsExitCodeThree(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := runScanCommand(t, tmpDir, ".")
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}
	if exitErr.Err == nil || exitErr.Err.Error() != noScanInputsFoundInProjectDirMessage {
		t.Fatalf("expected clear no-input error %q, got %v", noScanInputsFoundInProjectDirMessage, exitErr.Err)
	}
}

func TestScan_NoKiCadCLIDisablesSchematicNetlistGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "robot.kicad_sch"), "(kicad_sch)")

	_, err := runScanCommand(t, tmpDir, ".", "--no-kicad-cli")
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}
	if exitErr.Err == nil || !strings.Contains(exitErr.Err.Error(), "root KiCad schematic found but no netlist is available") {
		t.Fatalf("expected no-kicad-cli tool error, got %v", exitErr.Err)
	}
}

func TestResolveScanInput_ExportsNetlistWinsOverRoot(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "exports", "board.net"), "(export)")
	writeScanTestFile(t, filepath.Join(tmpDir, "root.net"), "(export)")

	got, err := resolveScanInput(tmpDir, "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := filepath.Join(tmpDir, "exports", "board.net")
	if got.NetlistPath != expected {
		t.Fatalf("expected %q, got %q", expected, got.NetlistPath)
	}
	if !got.NetlistDiscovered {
		t.Fatalf("expected netlist discovery")
	}
}

func TestResolveScanInput_RootNetlistPicksLexicalFirst(t *testing.T) {
	tmpDir := t.TempDir()
	writeScanTestFile(t, filepath.Join(tmpDir, "zeta.net"), "(export)")
	writeScanTestFile(t, filepath.Join(tmpDir, "alpha.net"), "(export)")

	got, err := resolveScanInput(tmpDir, "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := filepath.Join(tmpDir, "alpha.net")
	if got.NetlistPath != expected {
		t.Fatalf("expected %q, got %q", expected, got.NetlistPath)
	}
}

func TestResolveScanInput_NetlistOverrideWins(t *testing.T) {
	tmpDir := t.TempDir()
	override := filepath.Join(tmpDir, "manual", "chosen.net")
	writeScanTestFile(t, filepath.Join(tmpDir, "exports", "board.net"), "(export)")
	writeScanTestFile(t, override, "(export)")

	got, err := resolveScanInput(tmpDir, "", override)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.NetlistPath != override {
		t.Fatalf("expected %q, got %q", override, got.NetlistPath)
	}
	if got.NetlistDiscovered {
		t.Fatalf("expected override to suppress discovery flag")
	}
}

func writeFakeKiCadCLI(t *testing.T, dir string, netlist string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-kicad-cli")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      shift
      out="$1"
      ;;
  esac
  shift
done
if [ -z "$out" ]; then
  echo "missing --output" >&2
  exit 64
fi
cat > "$out" <<'ARCHITON_NETLIST'
` + netlist + `
ARCHITON_NETLIST
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kicad-cli: %v", err)
	}
	return path
}
