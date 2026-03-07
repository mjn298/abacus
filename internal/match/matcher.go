package match

import (
	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

// MatchService orchestrates the three-tier matching algorithm:
// Tier 1 (exact) → Tier 2 (fuzzy) → Tier 3 (suggest).
type MatchService struct {
	cucumber *CucumberMatcher
	fuzzy    *FuzzyMatcher
	suggest  *SuggestionBuilder
	actions  *graph.ActionService
	repo     *graph.GraphRepository
	opts     MatchOptions
}

// MatchOptions configures the matching behavior.
type MatchOptions struct {
	FuzzyThreshold float64
	FuzzyLimit     int
	SuggestLimit   int
}

// MatchResult holds the outcome of matching a step text.
type MatchResult struct {
	Tier       string            `json:"tier"`                 // "exact", "fuzzy", "suggest"
	Action     *graph.ActionNode `json:"action,omitempty"`     // Tier 1: matched action with resolved refs
	Parameters map[string]string `json:"parameters,omitempty"` // Tier 1: extracted parameters
	Candidates []FuzzyCandidate  `json:"candidates,omitempty"` // Tier 2: fuzzy matches
	Suggestion *Suggestion       `json:"suggestion,omitempty"` // Tier 3: creation suggestion
}

// Step represents a single Gherkin step.
type Step struct {
	Keyword string `json:"keyword"` // "Given", "When", "Then", etc.
	Text    string `json:"text"`    // The step text without keyword
}

// NewMatchService creates a new MatchService with the given dependencies.
func NewMatchService(repo *graph.GraphRepository, actions *graph.ActionService, opts MatchOptions) *MatchService {
	if opts.FuzzyLimit <= 0 {
		opts.FuzzyLimit = 10
	}
	if opts.SuggestLimit <= 0 {
		opts.SuggestLimit = 5
	}

	fm := NewFuzzyMatcher(repo)
	if opts.FuzzyThreshold != 0 {
		fm.threshold = opts.FuzzyThreshold
	}

	return &MatchService{
		cucumber: NewCucumberMatcher(),
		fuzzy:    fm,
		suggest:  NewSuggestionBuilder(repo),
		actions:  actions,
		repo:     repo,
		opts:     opts,
	}
}

// Match runs the tiered match algorithm against a step text.
// It tries exact cucumber matching first, then fuzzy FTS5, then suggestion.
func (m *MatchService) Match(stepText string) (*MatchResult, error) {
	// Tier 1: Exact cucumber expression match
	result, err := m.tryExactMatch(stepText)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	// Tier 2: Fuzzy FTS5 match
	result, err = m.tryFuzzyMatch(stepText)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	// Tier 3: Suggestion
	return m.buildSuggestion(stepText)
}

// MatchScenario runs Match for each step in a scenario.
func (m *MatchService) MatchScenario(steps []Step) ([]MatchResult, error) {
	results := make([]MatchResult, len(steps))
	for i, step := range steps {
		result, err := m.Match(step.Text)
		if err != nil {
			return nil, err
		}
		results[i] = *result
	}
	return results, nil
}

// tryExactMatch attempts Tier 1 matching against all action nodes.
func (m *MatchService) tryExactMatch(stepText string) (*MatchResult, error) {
	actionNodes, err := m.actions.List(graph.ListActionOpts{Limit: 10000})
	if err != nil {
		return nil, err
	}

	for _, an := range actionNodes {
		patterns := extractGherkinPatterns(an.Node)
		if len(patterns) == 0 {
			continue
		}

		matchedPattern, params, matched := m.cucumber.MatchAny(patterns, stepText)
		if matched {
			_ = matchedPattern
			// Resolve full action with references
			resolved, err := m.actions.Get(an.Node.ID)
			if err != nil {
				return nil, err
			}
			return &MatchResult{
				Tier:       "exact",
				Action:     resolved,
				Parameters: params,
			}, nil
		}
	}

	return nil, nil
}

// tryFuzzyMatch attempts Tier 2 matching via FTS5.
func (m *MatchService) tryFuzzyMatch(stepText string) (*MatchResult, error) {
	candidates, err := m.fuzzy.Match(stepText, m.opts.FuzzyLimit)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	return &MatchResult{
		Tier:       "fuzzy",
		Candidates: candidates,
	}, nil
}

// buildSuggestion generates a Tier 3 suggestion.
func (m *MatchService) buildSuggestion(stepText string) (*MatchResult, error) {
	suggestion, err := m.suggest.Build(stepText)
	if err != nil {
		return nil, err
	}

	return &MatchResult{
		Tier:       "suggest",
		Suggestion: suggestion,
	}, nil
}

// extractGherkinPatterns extracts gherkin_patterns from a node's properties.
func extractGherkinPatterns(node db.GraphNode) []string {
	raw, ok := node.Properties["gherkin_patterns"]
	if !ok {
		return nil
	}

	switch v := raw.(type) {
	case []any:
		patterns := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				patterns = append(patterns, s)
			}
		}
		return patterns
	case []string:
		return v
	default:
		return nil
	}
}
