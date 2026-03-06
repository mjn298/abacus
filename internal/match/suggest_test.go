package match

import (
	"testing"
)

func TestSuggestionBuilder_Build_GeneratesInput(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	sb := NewSuggestionBuilder(repo)
	suggestion, err := sb.Build("I register a new user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected non-nil suggestion")
	}
	if suggestion.Input.Name == "" {
		t.Error("expected non-empty action name")
	}
	if len(suggestion.Input.GherkinPatterns) == 0 {
		t.Error("expected at least one gherkin pattern")
	}
}

func TestSuggestionBuilder_Build_FindsRelatedRoutes(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	sb := NewSuggestionBuilder(repo)
	suggestion, err := sb.Build("I register a new user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected non-nil suggestion")
	}
	// Should find the register route
	if len(suggestion.Context.Routes) == 0 {
		t.Error("expected related routes")
	}
}

func TestSuggestionBuilder_Build_FindsRelatedEntities(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	sb := NewSuggestionBuilder(repo)
	suggestion, err := sb.Build("I register a new user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected non-nil suggestion")
	}
	// Should find the User entity
	if len(suggestion.Context.Entities) == 0 {
		t.Error("expected related entities")
	}
}

func TestSuggestionBuilder_Build_PatternWithStringPlaceholder(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	sb := NewSuggestionBuilder(repo)
	suggestion, err := sb.Build(`I see the message "hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected non-nil suggestion")
	}
	// Pattern should have {string} for quoted strings
	pattern := suggestion.Input.GherkinPatterns[0]
	if !containsSubstring(pattern, "{string}") {
		t.Errorf("expected pattern to contain {string}, got %q", pattern)
	}
}

func TestSuggestionBuilder_Build_PatternWithIntPlaceholder(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	sb := NewSuggestionBuilder(repo)
	suggestion, err := sb.Build("I have 5 items in my cart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected non-nil suggestion")
	}
	pattern := suggestion.Input.GherkinPatterns[0]
	if !containsSubstring(pattern, "{int}") {
		t.Errorf("expected pattern to contain {int}, got %q", pattern)
	}
}

func TestSuggestionBuilder_Build_NeverReturnsNil(t *testing.T) {
	repo := setupTestDB(t)
	// No seed data - empty graph

	sb := NewSuggestionBuilder(repo)
	suggestion, err := sb.Build("some completely unknown step")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("Build should never return nil")
	}
	if suggestion.Input.Name == "" {
		t.Error("expected non-empty name even with empty graph")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsIdx(s, sub))
}

func containsIdx(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
