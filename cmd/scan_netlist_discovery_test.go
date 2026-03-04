package cmd

import (
	"path/filepath"
	"testing"
)

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
