package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

// setupTestDB creates a temp dir with an initialized DB and seeds an action node.
// Returns the temp dir path and a cleanup function.
func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	abacusDir := filepath.Join(dir, ".abacus")
	if err := os.MkdirAll(abacusDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbFilePath := filepath.Join(abacusDir, "abacus.db")
	database, err := db.OpenDB(dbFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.InitSchema(database); err != nil {
		t.Fatal(err)
	}

	// Seed an action node with a gherkin pattern
	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)
	_, err = actions.Create(graph.CreateActionInput{
		Name:            "register user",
		Label:           "Register a new user",
		GherkinPatterns: []string{"I register a new user"},
	})
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestMatchSingleStep(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"match", "--db", dbFile, "I register a new user"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("match command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "exact") {
		t.Errorf("expected exact match tier in output, got: %s", output)
	}
	if !strings.Contains(output, "register") {
		t.Errorf("expected 'register' in output, got: %s", output)
	}
}

func TestMatchSingleStepJSON(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"match", "--db", dbFile, "--json", "I register a new user"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("match --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	tier, ok := result["tier"].(string)
	if !ok || tier != "exact" {
		t.Errorf("expected tier 'exact', got %v", result["tier"])
	}
}

func TestMatchSuggestTier(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"match", "--db", dbFile, "I do something completely unknown"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("match command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "suggest") {
		t.Errorf("expected suggest tier in output, got: %s", output)
	}
}

func TestMatchFileMode(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Create a temp feature file
	featureContent := `Feature: Auth
  Scenario: Register
    Given I am on the register page
    When I register a new user
    Then I should see the dashboard
`
	featureFile := filepath.Join(dir, "auth.feature")
	if err := os.WriteFile(featureFile, []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"match", "--db", dbFile, "--file", featureFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("match --file failed: %v", err)
	}

	output := buf.String()
	// Should have processed multiple steps
	if !strings.Contains(output, "register") {
		t.Errorf("expected 'register' in file mode output, got: %s", output)
	}
}

func TestMatchFileModeJSON(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	featureContent := `Feature: Auth
  Scenario: Register
    Given I am on the register page
    When I register a new user
    Then I should see the dashboard
`
	featureFile := filepath.Join(dir, "auth.feature")
	if err := os.WriteFile(featureFile, []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"match", "--db", dbFile, "--json", "--file", featureFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("match --file --json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON array: %v\noutput: %s", err, buf.String())
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestCoverageCommand(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Create a feature file with one matching and two non-matching steps
	featureContent := `Feature: Auth
  Scenario: Register
    Given I am on the register page
    When I register a new user
    Then I should see the dashboard
`
	featDir := filepath.Join(dir, "features")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "auth.feature"), []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"coverage", "--db", dbFile, filepath.Join(featDir, "*.feature")})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("coverage command failed: %v", err)
	}

	output := buf.String()
	// Should show coverage data (either text with "%" or JSON with "coverage_pct")
	if !strings.Contains(output, "%") && !strings.Contains(output, "coverage_pct") {
		t.Errorf("expected coverage data in output, got: %s", output)
	}
}

func TestCoverageJSON(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	featureContent := `Feature: Auth
  Scenario: Register
    Given I am on the register page
    When I register a new user
    Then I should see the dashboard
`
	featDir := filepath.Join(dir, "features")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "auth.feature"), []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"coverage", "--db", dbFile, "--json", filepath.Join(featDir, "*.feature")})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("coverage --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if _, ok := result["total_steps"]; !ok {
		t.Error("JSON output missing 'total_steps' field")
	}
	if _, ok := result["exact_matches"]; !ok {
		t.Error("JSON output missing 'exact_matches' field")
	}
	if _, ok := result["coverage_pct"]; !ok {
		t.Error("JSON output missing 'coverage_pct' field")
	}
}

func TestParseFeatureFile(t *testing.T) {
	content := `Feature: Login
  Scenario: Valid login
    Given I am on the login page
    When I enter valid credentials
    And I click submit
    Then I should see the homepage
    But I should not see the login form
`
	tmpFile := filepath.Join(t.TempDir(), "test.feature")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	steps, err := parseFeatureFile(tmpFile)
	if err != nil {
		t.Fatalf("parseFeatureFile failed: %v", err)
	}

	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	expected := []struct {
		keyword string
		text    string
	}{
		{"Given", "I am on the login page"},
		{"When", "I enter valid credentials"},
		{"And", "I click submit"},
		{"Then", "I should see the homepage"},
		{"But", "I should not see the login form"},
	}

	for i, exp := range expected {
		if steps[i].Keyword != exp.keyword {
			t.Errorf("step %d keyword: got %q, want %q", i, steps[i].Keyword, exp.keyword)
		}
		if steps[i].Text != exp.text {
			t.Errorf("step %d text: got %q, want %q", i, steps[i].Text, exp.text)
		}
	}
}
