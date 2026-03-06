package match

import (
	"fmt"

	cucumberexpressions "github.com/cucumber/cucumber-expressions/go/v16"
)

// CucumberMatcher compiles and caches cucumber expressions, and matches
// step text against them with parameter extraction.
type CucumberMatcher struct {
	registry *cucumberexpressions.ParameterTypeRegistry
	cache    map[string]cucumberexpressions.Expression
}

// NewCucumberMatcher creates a new CucumberMatcher with an empty cache.
func NewCucumberMatcher() *CucumberMatcher {
	return &CucumberMatcher{
		registry: cucumberexpressions.NewParameterTypeRegistry(),
		cache:    make(map[string]cucumberexpressions.Expression),
	}
}

// CompilePattern compiles a cucumber expression pattern and caches it.
// Subsequent calls with the same pattern return the cached expression.
func (m *CucumberMatcher) CompilePattern(pattern string) (cucumberexpressions.Expression, error) {
	if expr, ok := m.cache[pattern]; ok {
		return expr, nil
	}
	expr, err := cucumberexpressions.NewCucumberExpression(pattern, m.registry)
	if err != nil {
		return nil, fmt.Errorf("compile cucumber expression %q: %w", pattern, err)
	}
	m.cache[pattern] = expr
	return expr, nil
}

// Match tries to match stepText against a cucumber expression pattern.
// Returns extracted parameters as a map (param index → string value), whether
// a match was found, and any error.
func (m *CucumberMatcher) Match(pattern string, stepText string) (map[string]string, bool, error) {
	expr, err := m.CompilePattern(pattern)
	if err != nil {
		return nil, false, err
	}

	args, err := expr.Match(stepText)
	if err != nil {
		return nil, false, fmt.Errorf("match %q against %q: %w", stepText, pattern, err)
	}
	if args == nil {
		return nil, false, nil
	}

	params := make(map[string]string, len(args))
	for i, arg := range args {
		params[fmt.Sprintf("p%d", i)] = fmt.Sprintf("%v", arg.GetValue())
	}
	return params, true, nil
}

// MatchAny tries to match stepText against all given patterns in order.
// Returns the first matching pattern, extracted parameters, and whether any match was found.
func (m *CucumberMatcher) MatchAny(patterns []string, stepText string) (matchedPattern string, params map[string]string, matched bool) {
	for _, pattern := range patterns {
		p, ok, err := m.Match(pattern, stepText)
		if err != nil {
			continue
		}
		if ok {
			return pattern, p, true
		}
	}
	return "", nil, false
}
