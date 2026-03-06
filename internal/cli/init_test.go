package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesAbacusDir(t *testing.T) {
	dir := t.TempDir()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	abacusDir := filepath.Join(dir, ".abacus")
	info, err := os.Stat(abacusDir)
	if err != nil {
		t.Fatalf(".abacus directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".abacus is not a directory")
	}
}

func TestInitCreatesConfigYAML(t *testing.T) {
	dir := t.TempDir()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	configPath := filepath.Join(dir, ".abacus", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("config.yaml is empty")
	}
	// Should contain version field
	if !bytes.Contains(data, []byte("version:")) {
		t.Error("config.yaml missing version field")
	}
	if !bytes.Contains(data, []byte("project:")) {
		t.Error("config.yaml missing project field")
	}
}

func TestInitCreatesDatabase(t *testing.T) {
	dir := t.TempDir()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	dbPath := filepath.Join(dir, ".abacus", "abacus.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database not created: %v", err)
	}
}

func TestInitAlreadyInitializedWarns(t *testing.T) {
	dir := t.TempDir()

	// First init
	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init should error
	buf.Reset()
	cmd.SetArgs([]string{"init", "--dir", dir})
	err := cmd.Execute()
	if err == nil {
		// Check output for warning
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("already initialized")) {
			t.Error("expected 'already initialized' warning on second init")
		}
	}
	// Either error or warning output is acceptable
}

func TestInitWithJSONFlag(t *testing.T) {
	dir := t.TempDir()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --json failed: %v", err)
	}

	output := buf.Bytes()
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, string(output))
	}

	if _, ok := result["project"]; !ok {
		t.Error("JSON output missing 'project' field")
	}
	if _, ok := result["config_path"]; !ok {
		t.Error("JSON output missing 'config_path' field")
	}
	if _, ok := result["db_path"]; !ok {
		t.Error("JSON output missing 'db_path' field")
	}
}

func TestInitAutoDetectsGoProject(t *testing.T) {
	dir := t.TempDir()
	// Create a go.mod file
	gomod := []byte("module example.com/myproject\n\ngo 1.21\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), gomod, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	projectType, ok := result["project_type"]
	if !ok {
		t.Fatal("JSON output missing 'project_type' field")
	}
	if projectType != "go" {
		t.Errorf("expected project_type 'go', got %v", projectType)
	}
}

func TestInitAutoDetectsNodeProject(t *testing.T) {
	dir := t.TempDir()
	// Create a package.json file
	pkg := []byte(`{"name": "my-node-project", "version": "1.0.0"}`)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), pkg, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["project_type"] != "node" {
		t.Errorf("expected project_type 'node', got %v", result["project_type"])
	}
}

func TestInitAddsToGitignore(t *testing.T) {
	dir := t.TempDir()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf(".gitignore not created/updated: %v", err)
	}

	if !bytes.Contains(data, []byte(".abacus/abacus.db")) {
		t.Error(".gitignore does not contain .abacus/abacus.db")
	}
}

func TestInitPreservesExistingGitignore(t *testing.T) {
	dir := t.TempDir()

	// Create existing .gitignore
	existing := []byte("node_modules/\n.env\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), existing, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--dir", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !bytes.Contains([]byte(content), []byte("node_modules/")) {
		t.Error("existing .gitignore content was lost")
	}
	if !bytes.Contains([]byte(content), []byte(".abacus/abacus.db")) {
		t.Error(".abacus/abacus.db not added to .gitignore")
	}
}
