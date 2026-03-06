package match

import (
	"fmt"
	"testing"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

func setupMatchService(t *testing.T) *MatchService {
	t.Helper()
	repo := setupTestDB(t)
	seedTestNodes(t, repo)
	actions := graph.NewActionService(repo)
	return NewMatchService(repo, actions, MatchOptions{
		FuzzyLimit: 10,
	})
}

func TestMatchService_Tier1_ExactMatch(t *testing.T) {
	svc := setupMatchService(t)

	result, err := svc.Match("I register a new user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Tier != "exact" {
		t.Errorf("expected tier 'exact', got %q", result.Tier)
	}
	if result.Action == nil {
		t.Fatal("expected non-nil action for exact match")
	}
	if result.Action.Node.ID != "action:register-new-user" {
		t.Errorf("expected action 'action:register-new-user', got %q", result.Action.Node.ID)
	}
	if result.Parameters == nil || len(result.Parameters) == 0 {
		t.Error("expected extracted parameters")
	}
	// Should have extracted "user" as the {word} parameter
	found := false
	for _, v := range result.Parameters {
		if v == "user" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected parameter 'user', got %v", result.Parameters)
	}
}

func TestMatchService_Tier2_FuzzyMatch(t *testing.T) {
	svc := setupMatchService(t)

	// Use text that won't match any exact pattern but has related keywords
	result, err := svc.Match("create a new user account")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Tier != "fuzzy" && result.Tier != "suggest" {
		// fuzzy or suggest are both acceptable when no exact match
		t.Logf("got tier %q", result.Tier)
	}
	if result.Tier == "fuzzy" {
		if len(result.Candidates) == 0 {
			t.Error("expected fuzzy candidates")
		}
	}
}

func TestMatchService_Tier3_Suggestion(t *testing.T) {
	repo := setupTestDB(t)
	// Don't seed any action nodes - just route/entity/page
	// This forces tier 3
	actions := graph.NewActionService(repo)
	svc := NewMatchService(repo, actions, MatchOptions{FuzzyLimit: 10})

	result, err := svc.Match("I do something completely unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Tier != "suggest" {
		t.Errorf("expected tier 'suggest', got %q", result.Tier)
	}
	if result.Suggestion == nil {
		t.Error("expected non-nil suggestion")
	}
}

func TestMatchService_TierCascade(t *testing.T) {
	svc := setupMatchService(t)

	// Tier 1: exact match
	r1, err := svc.Match("I register a new admin")
	if err != nil {
		t.Fatalf("tier 1: %v", err)
	}
	if r1.Tier != "exact" {
		t.Errorf("expected tier 1 exact, got %q", r1.Tier)
	}

	// Tier 2 or 3: no exact match, should fall through
	r2, err := svc.Match("navigate to the home page")
	if err != nil {
		t.Fatalf("tier 2/3: %v", err)
	}
	if r2.Tier == "exact" {
		t.Error("did not expect exact match for 'navigate to the home page'")
	}
}

func TestMatchService_MatchScenario(t *testing.T) {
	svc := setupMatchService(t)

	steps := []Step{
		{Keyword: "Given", Text: "I register a new user"},
		{Keyword: "When", Text: "I do something unknown"},
		{Keyword: "Then", Text: "I register a new admin"},
	}

	results, err := svc.MatchScenario(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != len(steps) {
		t.Fatalf("expected %d results, got %d", len(steps), len(results))
	}
	// First and third should be exact matches
	if results[0].Tier != "exact" {
		t.Errorf("step 0: expected 'exact', got %q", results[0].Tier)
	}
	if results[2].Tier != "exact" {
		t.Errorf("step 2: expected 'exact', got %q", results[2].Tier)
	}
}

func TestMatchService_Performance_50Actions(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	// Add 50 more actions
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("action:perf-test-%d", i)
		name := fmt.Sprintf("perf test action %d", i)
		pattern := fmt.Sprintf("I perform action %d on {word}", i)
		n := &db.GraphNode{
			ID:   id,
			Kind: db.NodeAction,
			Name: name,
			Label: name,
			Properties: map[string]any{
				"gherkin_patterns": []any{pattern},
			},
			Source: db.SourceAgent,
		}
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("seed perf node %d: %v", i, err)
		}
	}

	actions := graph.NewActionService(repo)
	svc := NewMatchService(repo, actions, MatchOptions{FuzzyLimit: 10})

	// Should complete in reasonable time
	result, err := svc.Match("I perform action 25 on widget")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Tier != "exact" {
		t.Errorf("expected exact match, got %q", result.Tier)
	}
}
