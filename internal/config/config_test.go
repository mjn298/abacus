package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `version: 1
project:
  name: my-app
  root: .
scanners:
  orpc:
    command: abacus-scanner-orpc
    options:
      routerFile: src/router.ts
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Project.Name != "my-app" {
		t.Errorf("expected project name 'my-app', got %q", cfg.Project.Name)
	}
	if cfg.Project.Root != "." {
		t.Errorf("expected project root '.', got %q", cfg.Project.Root)
	}
	if len(cfg.Scanners) != 1 {
		t.Errorf("expected 1 scanner, got %d", len(cfg.Scanners))
	}
	sc, ok := cfg.Scanners["orpc"]
	if !ok {
		t.Fatal("expected scanner 'orpc' to exist")
	}
	if sc.Command != "abacus-scanner-orpc" {
		t.Errorf("expected command 'abacus-scanner-orpc', got %q", sc.Command)
	}
}

func TestLoadRejectsInvalidVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `version: 2
project:
  name: my-app
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestLoadRejectsZeroVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `project:
  name: my-app
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for zero version, got nil")
	}
}

func TestLoadRejectsMissingProjectName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `version: 1
project:
  root: .
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing project name, got nil")
	}
}

func TestLoadRejectsNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestScannerConfig_PhaseLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `version: 1
project:
  name: my-app
  root: .
scanners:
  linker:
    command: abacus-scanner-linker
    phase: link
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sc := cfg.Scanners["linker"]
	if sc.Phase != "link" {
		t.Errorf("expected phase 'link', got %q", sc.Phase)
	}
}

func TestScannerConfig_PhaseScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `version: 1
project:
  name: my-app
  root: .
scanners:
  scanner:
    command: abacus-scanner-foo
    phase: scan
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sc := cfg.Scanners["scanner"]
	if sc.Phase != "scan" {
		t.Errorf("expected phase 'scan', got %q", sc.Phase)
	}
}

func TestScannerConfig_PhaseDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `version: 1
project:
  name: my-app
  root: .
scanners:
  basic:
    command: abacus-scanner-basic
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sc := cfg.Scanners["basic"]
	if sc.Phase != "" {
		t.Errorf("expected phase '' (empty default), got %q", sc.Phase)
	}
}
