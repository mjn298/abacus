package scanner

import (
	"encoding/json"
	"testing"
)

func TestScanInput_ExistingNodes_JSON(t *testing.T) {
	input := ScanInput{
		Version:     1,
		ProjectRoot: "/app",
		Options:     map[string]any{"key": "val"},
		ExistingNodes: []ScanNodeRef{
			{ID: "n1", Kind: "route", Name: "/api/users", SourceFile: "src/routes.ts"},
			{ID: "n2", Kind: "model", Name: "User"},
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ScanInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.ExistingNodes) != 2 {
		t.Fatalf("expected 2 existing nodes, got %d", len(decoded.ExistingNodes))
	}

	n := decoded.ExistingNodes[0]
	if n.ID != "n1" || n.Kind != "route" || n.Name != "/api/users" || n.SourceFile != "src/routes.ts" {
		t.Errorf("first node mismatch: %+v", n)
	}

	n2 := decoded.ExistingNodes[1]
	if n2.ID != "n2" || n2.Kind != "model" || n2.Name != "User" || n2.SourceFile != "" {
		t.Errorf("second node mismatch: %+v", n2)
	}
}

func TestScanInput_ExistingNodes_Omitempty(t *testing.T) {
	input := ScanInput{
		Version:     1,
		ProjectRoot: "/app",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	raw := string(data)
	// existingNodes should not appear in JSON when nil/empty
	if containsKey(raw, "existingNodes") {
		t.Errorf("expected existingNodes to be omitted, got: %s", raw)
	}
}

func TestScanNodeRef_NoProperties(t *testing.T) {
	// Compile-time check: ScanNodeRef should have exactly these fields.
	// If someone adds a Properties field, this explicit construction will still compile,
	// but the JSON round-trip below ensures no extra fields leak through.
	ref := ScanNodeRef{
		ID:         "n1",
		Kind:       "route",
		Name:       "/users",
		SourceFile: "src/routes.ts",
	}

	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal into a generic map to verify no "properties" key exists
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if _, ok := m["properties"]; ok {
		t.Error("ScanNodeRef should not have a 'properties' field")
	}

	// Verify expected fields are present
	if m["id"] != ref.ID || m["kind"] != ref.Kind || m["name"] != ref.Name {
		t.Errorf("unexpected field values: %v", m)
	}
}

func containsKey(jsonStr, key string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
