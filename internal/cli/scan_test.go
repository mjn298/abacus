package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestScanNoConfig(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Remove config.yaml if it exists (setupTestDB doesn't create one)
	os.Remove(filepath.Join(dir, ".abacus", "config.yaml"))

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"scan", "--db", dbFile})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no config.yaml exists, got nil")
	}
}

func TestScanNoScanners(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Create a valid config.yaml with empty scanners map
	configContent := `version: 1
project:
  name: test-project
  root: .
scanners: {}
`
	configPath := filepath.Join(dir, ".abacus", "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"scan", "--db", dbFile})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no scanners configured, got nil")
	}
}
