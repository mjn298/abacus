package match

import (
	"testing"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

func setupTestDB(t *testing.T) *graph.GraphRepository {
	t.Helper()
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	err = db.InitSchema(database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return graph.NewGraphRepository(database)
}

func seedTestNodes(t *testing.T, repo *graph.GraphRepository) {
	t.Helper()

	nodes := []db.GraphNode{
		{
			ID: "route:POST-/api/auth/register", Kind: db.NodeRoute,
			Name: "POST /api/auth/register", Label: "Register endpoint",
			Properties: map[string]any{}, Source: db.SourceScan,
		},
		{
			ID: "entity:User", Kind: db.NodeEntity,
			Name: "User", Label: "User entity",
			Properties: map[string]any{}, Source: db.SourceScan,
		},
		{
			ID: "page:/register", Kind: db.NodePage,
			Name: "/register", Label: "Registration page",
			Properties: map[string]any{}, Source: db.SourceScan,
		},
		{
			ID: "action:register-new-user", Kind: db.NodeAction,
			Name: "register new user", Label: "Register a new user",
			Properties: map[string]any{
				"gherkin_patterns": []any{"I register a new {word}"},
			},
			Source: db.SourceAgent,
		},
		{
			ID: "action:login-user", Kind: db.NodeAction,
			Name: "login user", Label: "Login an existing user",
			Properties: map[string]any{
				"gherkin_patterns": []any{"I login as {string}"},
			},
			Source: db.SourceAgent,
		},
		{
			ID: "action:delete-user", Kind: db.NodeAction,
			Name: "delete user", Label: "Delete a user account",
			Properties: map[string]any{
				"gherkin_patterns": []any{"I delete the user {string}"},
			},
			Source: db.SourceAgent,
		},
	}

	for _, n := range nodes {
		if err := repo.InsertNode(&n); err != nil {
			t.Fatalf("seed node %s: %v", n.ID, err)
		}
	}
}

func TestTokenize_Basic(t *testing.T) {
	tokens := Tokenize("Given I register a new user")
	// "Given", "I", "a" are stripped (keywords/stopwords)
	expected := []string{"register", "new", "user"}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("token[%d] = %q, want %q", i, tok, expected[i])
		}
	}
}

func TestTokenize_StripsFTS5SpecialChars(t *testing.T) {
	// FTS5 special chars: / * + - ^ ~ should be stripped
	tokens := Tokenize(`I navigate to "/dashboard"`)
	// "I", "to" are stopwords; "/dashboard" → "dashboard"; quotes stripped
	for _, tok := range tokens {
		for _, c := range tok {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("token %q contains non-alphanumeric char %q", tok, string(c))
			}
		}
	}
	// Should contain "navigate" and "dashboard"
	found := map[string]bool{}
	for _, tok := range tokens {
		found[tok] = true
	}
	if !found["navigate"] {
		t.Error("expected 'navigate' token")
	}
	if !found["dashboard"] {
		t.Error("expected 'dashboard' token")
	}
}

func TestTokenize_EmptyAfterStripping(t *testing.T) {
	// Pure punctuation/special chars should produce no tokens
	tokens := Tokenize(`"/" "*" "+" "-"`)
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens from pure special chars, got %d: %v", len(tokens), tokens)
	}
}

func TestFuzzyMatcher_Match_FindsSimilar(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	fm := NewFuzzyMatcher(repo)
	candidates, err := fm.Match("register user", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}
	// The "register new user" action should be among results
	found := false
	for _, c := range candidates {
		if c.Action.ID == "action:register-new-user" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'register-new-user' action in results")
	}
}

func TestFuzzyMatcher_Match_StripsKeywords(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	fm := NewFuzzyMatcher(repo)
	// "Given" should be stripped, "register" should still match
	candidates, err := fm.Match("Given I register a new user", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate after keyword stripping")
	}
}

func TestFuzzyMatcher_Match_NoResults(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	fm := NewFuzzyMatcher(repo)
	candidates, err := fm.Match("xylophone quantum entanglement", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates, got %d", len(candidates))
	}
}

func TestFuzzyMatcher_Match_SortedByRank(t *testing.T) {
	repo := setupTestDB(t)
	seedTestNodes(t, repo)

	fm := NewFuzzyMatcher(repo)
	candidates, err := fm.Match("user", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) < 2 {
		t.Skip("need at least 2 candidates to test sort order")
	}
	// BM25 ranks are negative; lower (more negative) = better match
	// Results should be sorted ascending (best first)
	for i := 1; i < len(candidates); i++ {
		if candidates[i].Score < candidates[i-1].Score {
			t.Errorf("results not sorted by rank: [%d]=%f < [%d]=%f",
				i, candidates[i].Score, i-1, candidates[i-1].Score)
		}
	}
}
