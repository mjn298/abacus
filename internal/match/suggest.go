package match

import (
	"regexp"
	"strings"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

// Suggestion holds a generated CreateActionInput and the graph context
// that informed it.
type Suggestion struct {
	Input   graph.CreateActionInput `json:"input"`
	Context RelatedContext          `json:"context"`
}

// RelatedContext holds graph nodes related to a step text, discovered
// via keyword search.
type RelatedContext struct {
	Routes   []db.GraphNode `json:"routes"`
	Entities []db.GraphNode `json:"entities"`
	Pages    []db.GraphNode `json:"pages"`
}

// SuggestionBuilder generates action creation suggestions from step text
// by searching the graph for related nodes and generating cucumber patterns.
type SuggestionBuilder struct {
	repo *graph.GraphRepository
}

// NewSuggestionBuilder creates a new SuggestionBuilder.
func NewSuggestionBuilder(repo *graph.GraphRepository) *SuggestionBuilder {
	return &SuggestionBuilder{repo: repo}
}

// quotedStringRe matches double-quoted strings in step text.
var quotedStringRe = regexp.MustCompile(`"[^"]*"`)

// numberRe matches standalone integers in step text.
var numberRe = regexp.MustCompile(`\b\d+\b`)

// Build generates a CreateActionInput suggestion from step text and graph context.
// It always returns a non-nil suggestion.
func (b *SuggestionBuilder) Build(stepText string) (*Suggestion, error) {
	tokens := Tokenize(stepText)

	// Search for related nodes by keyword
	routes := b.searchByKind(tokens, db.NodeRoute)
	entities := b.searchByKind(tokens, db.NodeEntity)
	pages := b.searchByKind(tokens, db.NodePage)

	// Generate action name from step text
	name := generateActionName(stepText)

	// Generate cucumber expression pattern
	pattern := generatePattern(stepText)

	// Collect reference IDs
	var routeRefs, entityRefs, pageRefs []string
	for _, r := range routes {
		routeRefs = append(routeRefs, r.ID)
	}
	for _, e := range entities {
		entityRefs = append(entityRefs, e.ID)
	}
	for _, p := range pages {
		pageRefs = append(pageRefs, p.ID)
	}

	return &Suggestion{
		Input: graph.CreateActionInput{
			Name:            name,
			Label:           stepText,
			GherkinPatterns: []string{pattern},
			RouteRefs:       routeRefs,
			EntityRefs:      entityRefs,
			PageRefs:        pageRefs,
		},
		Context: RelatedContext{
			Routes:   routes,
			Entities: entities,
			Pages:    pages,
		},
	}, nil
}

// searchByKind searches the graph for nodes of a given kind matching the tokens.
func (b *SuggestionBuilder) searchByKind(tokens []string, kind db.NodeKind) []db.GraphNode {
	if len(tokens) == 0 {
		return nil
	}
	query := strings.Join(tokens, " OR ")
	results, err := b.repo.Search(query, &kind, 5)
	if err != nil {
		return nil
	}
	nodes := make([]db.GraphNode, len(results))
	for i, r := range results {
		nodes[i] = r.Node
	}
	return nodes
}

// generateActionName creates an action name from step text by cleaning it up.
func generateActionName(stepText string) string {
	// Strip Gherkin keywords from the front
	text := strings.ToLower(strings.TrimSpace(stepText))
	for _, kw := range []string{"given ", "when ", "then ", "and ", "but "} {
		if strings.HasPrefix(text, kw) {
			text = strings.TrimPrefix(text, kw)
			break
		}
	}

	// Remove quoted strings and numbers for the name
	text = quotedStringRe.ReplaceAllString(text, "")
	text = numberRe.ReplaceAllString(text, "")

	// Clean up whitespace
	text = strings.Join(strings.Fields(text), " ")
	text = strings.TrimSpace(text)

	if text == "" {
		text = "unnamed action"
	}

	return text
}

// generatePattern creates a cucumber expression pattern from step text by
// replacing quoted strings with {string} and numbers with {int}.
func generatePattern(stepText string) string {
	// Strip Gherkin keywords from the front
	text := strings.TrimSpace(stepText)
	lower := strings.ToLower(text)
	for _, kw := range []string{"Given ", "When ", "Then ", "And ", "But "} {
		if strings.HasPrefix(lower, strings.ToLower(kw)) {
			text = text[len(kw):]
			break
		}
	}

	// Replace quoted strings with {string}
	pattern := quotedStringRe.ReplaceAllString(text, "{string}")

	// Replace standalone numbers with {int}
	pattern = numberRe.ReplaceAllString(pattern, "{int}")

	return pattern
}
