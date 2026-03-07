package match

import (
	"strings"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

// FuzzyCandidate holds an action node and its FTS5 BM25 score.
type FuzzyCandidate struct {
	Action db.GraphNode `json:"action"`
	Score  float64      `json:"score"`
}

// FuzzyMatcher searches for action nodes matching step text via FTS5
// full-text search with BM25 ranking.
type FuzzyMatcher struct {
	repo      *graph.GraphRepository
	threshold float64
}

// NewFuzzyMatcher creates a new FuzzyMatcher with default threshold.
// The threshold is the maximum (least negative) BM25 rank to accept.
// BM25 ranks are negative; more negative = better match.
func NewFuzzyMatcher(repo *graph.GraphRepository) *FuzzyMatcher {
	return &FuzzyMatcher{
		repo:      repo,
		threshold: 0, // Accept all FTS5 results (all ranks are negative)
	}
}

// gherkinKeywords are stripped from step text before tokenization.
var gherkinKeywords = map[string]bool{
	"given": true, "when": true, "then": true, "and": true, "but": true,
}

// stopWords are removed during tokenization to improve search quality.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "i": true,
	"is": true, "am": true, "are": true, "to": true,
	"in": true, "on": true, "at": true, "of": true,
	"as": true, "by": true, "my": true, "me": true,
}

// Tokenize extracts meaningful words from step text by stripping Gherkin
// keywords, stop words, and non-alphanumeric tokens.
func Tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var tokens []string
	for _, w := range words {
		// Strip all non-alphanumeric characters (including FTS5 special chars like / * + - ^ ~)
		var cleaned []byte
		for i := 0; i < len(w); i++ {
			c := w[i]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				cleaned = append(cleaned, c)
			}
		}
		w = string(cleaned)
		if w == "" {
			continue
		}
		if gherkinKeywords[w] || stopWords[w] {
			continue
		}
		tokens = append(tokens, w)
	}
	return tokens
}

// Match searches for action nodes matching the step text via FTS5.
// Results are sorted by BM25 rank (lower = better match).
func (m *FuzzyMatcher) Match(stepText string, limit int) ([]FuzzyCandidate, error) {
	tokens := Tokenize(stepText)
	if len(tokens) == 0 {
		return nil, nil
	}

	// Build FTS5 query: join tokens with OR for broad matching
	query := strings.Join(tokens, " OR ")

	kind := db.NodeAction
	results, err := m.repo.Search(query, &kind, limit)
	if err != nil {
		return nil, err
	}

	var candidates []FuzzyCandidate
	for _, r := range results {
		if r.Rank > m.threshold {
			continue // Filter out low-quality matches (rank > threshold means worse)
		}
		candidates = append(candidates, FuzzyCandidate{
			Action: r.Node,
			Score:  r.Rank,
		})
	}

	return candidates, nil
}
