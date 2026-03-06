package match

import (
	"testing"
)

func TestCucumberMatcher_CompilePattern_Valid(t *testing.T) {
	m := NewCucumberMatcher()
	expr, err := m.CompilePattern("I register a new {word}")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if expr == nil {
		t.Fatal("expected non-nil expression")
	}
}

func TestCucumberMatcher_CompilePattern_Caching(t *testing.T) {
	m := NewCucumberMatcher()
	expr1, err := m.CompilePattern("I register a new {word}")
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	expr2, err := m.CompilePattern("I register a new {word}")
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	// Both should be the same cached instance
	if expr1 != expr2 {
		t.Error("expected same cached expression instance")
	}
}

func TestCucumberMatcher_Match_Word(t *testing.T) {
	m := NewCucumberMatcher()
	params, matched, err := m.Match("I register a new {word}", "I register a new user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected match")
	}
	if len(params) == 0 {
		t.Fatal("expected parameters")
	}
	// The {word} parameter should have extracted "user"
	found := false
	for _, v := range params {
		if v == "user" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected param value 'user', got params: %v", params)
	}
}

func TestCucumberMatcher_Match_Int(t *testing.T) {
	m := NewCucumberMatcher()
	params, matched, err := m.Match("I have {int} items in my cart", "I have 5 items in my cart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected match")
	}
	found := false
	for _, v := range params {
		if v == "5" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected param value '5', got params: %v", params)
	}
}

func TestCucumberMatcher_Match_String(t *testing.T) {
	m := NewCucumberMatcher()
	params, matched, err := m.Match("I see the message {string}", `I see the message "hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected match")
	}
	found := false
	for _, v := range params {
		if v == "hello world" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected param value 'hello world', got params: %v", params)
	}
}

func TestCucumberMatcher_Match_NoMatch(t *testing.T) {
	m := NewCucumberMatcher()
	_, matched, err := m.Match("I register a new {word}", "I delete an existing user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched {
		t.Error("expected no match")
	}
}

func TestCucumberMatcher_MatchAny_Found(t *testing.T) {
	m := NewCucumberMatcher()
	patterns := []string{
		"I delete a {word}",
		"I register a new {word}",
		"I update the {word}",
	}
	matchedPattern, params, matched := m.MatchAny(patterns, "I register a new user")
	if !matched {
		t.Fatal("expected match")
	}
	if matchedPattern != "I register a new {word}" {
		t.Errorf("expected pattern 'I register a new {word}', got %q", matchedPattern)
	}
	found := false
	for _, v := range params {
		if v == "user" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected param 'user', got %v", params)
	}
}

func TestCucumberMatcher_MatchAny_NotFound(t *testing.T) {
	m := NewCucumberMatcher()
	patterns := []string{
		"I delete a {word}",
		"I register a new {word}",
	}
	_, _, matched := m.MatchAny(patterns, "I navigate to the dashboard")
	if matched {
		t.Error("expected no match")
	}
}
