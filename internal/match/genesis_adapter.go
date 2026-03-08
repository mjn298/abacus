package match

import "github.com/mjn/abacus/internal/graph"

// GenesisAdapter wraps MatchService to implement graph.StepMatcher,
// breaking the import cycle between match and graph.
type GenesisAdapter struct {
	svc *MatchService
}

// NewGenesisAdapter creates a new GenesisAdapter.
func NewGenesisAdapter(svc *MatchService) *GenesisAdapter {
	return &GenesisAdapter{svc: svc}
}

// MatchForGenesis implements graph.StepMatcher by delegating to MatchService.Match
// and converting the result into the graph-facing StepMatchResult type.
func (a *GenesisAdapter) MatchForGenesis(stepText string) (*graph.StepMatchResult, error) {
	result, err := a.svc.Match(stepText)
	if err != nil {
		return nil, err
	}

	out := &graph.StepMatchResult{
		Tier:   result.Tier,
		Action: result.Action, // *graph.ActionNode — same type
	}

	// For suggest tier, extract context routes and entities
	if result.Suggestion != nil {
		out.SuggestRoutes = result.Suggestion.Context.Routes
		out.SuggestEntities = result.Suggestion.Context.Entities
	}

	return out, nil
}
