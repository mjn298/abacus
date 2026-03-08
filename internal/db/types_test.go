package db

import (
	"testing"
)

func TestSanitizeFTS5Query(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple word", input: "User", want: `"User"`},
		{name: "multiple words", input: "User Account", want: `"User Account"`},
		{name: "FTS5 operators", input: "User OR Admin", want: `"User OR Admin"`},
		{name: "special chars wildcard", input: "User*", want: `"User*"`},
		{name: "double quotes in input", input: `User "admin"`, want: `"User ""admin"""`},
		{name: "empty string", input: "", want: ""},
		{name: "NEAR operator", input: "NEAR(User, Admin)", want: `"NEAR(User, Admin)"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFTS5Query(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFTS5Query(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGraphNode_GherkinPatterns(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]any
		want       []string
		wantNil    bool
	}{
		{
			name:       "[]any with strings",
			properties: map[string]any{"gherkin_patterns": []any{"pattern1", "pattern2"}},
			want:       []string{"pattern1", "pattern2"},
		},
		{
			name:       "[]string",
			properties: map[string]any{"gherkin_patterns": []string{"pattern1"}},
			want:       []string{"pattern1"},
		},
		{
			name:       "missing key",
			properties: map[string]any{"other": "value"},
			wantNil:    true,
		},
		{
			name:       "unexpected type int",
			properties: map[string]any{"gherkin_patterns": 42},
			wantNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := GraphNode{Properties: tt.properties}
			got := node.GherkinPatterns()
			if tt.wantNil {
				if got != nil {
					t.Errorf("GherkinPatterns() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("GherkinPatterns() len = %d, want %d", len(got), len(tt.want))
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("GherkinPatterns()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}
