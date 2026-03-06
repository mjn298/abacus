package scanner

import (
	"testing"
)

func validOutput() *ScanOutput {
	return &ScanOutput{
		Version: 1,
		Scanner: ScannerInfo{
			ID:      "test-scanner",
			Name:    "Test Scanner",
			Version: "1.0.0",
		},
		Nodes: []ScanNode{
			{
				ID:         "route:/api/users",
				Kind:       "route",
				Name:       "/api/users",
				Label:      "GET /api/users",
				Properties: map[string]any{},
				Source:     "scan",
			},
			{
				ID:         "entity:User",
				Kind:       "entity",
				Name:       "User",
				Label:      "User Entity",
				Properties: map[string]any{},
				Source:     "scan",
			},
		},
		Edges: []ScanEdge{
			{
				ID:    "edge:1",
				SrcID: "route:/api/users",
				DstID: "entity:User",
				Kind:  "touches_entity",
			},
		},
		Warnings: []ScanWarning{},
		Stats:    ScanStats{FilesScanned: 2, NodesFound: 2, EdgesFound: 1},
	}
}

func TestValidateOutput_Valid(t *testing.T) {
	errs := ValidateOutput(validOutput())
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateOutput_WrongVersion(t *testing.T) {
	out := validOutput()
	out.Version = 2
	errs := ValidateOutput(out)
	assertContains(t, errs, "version must be 1")
}

func TestValidateOutput_EmptyScannerID(t *testing.T) {
	out := validOutput()
	out.Scanner.ID = ""
	errs := ValidateOutput(out)
	assertContains(t, errs, "scanner.id must be non-empty")
}

func TestValidateOutput_NodeMissingID(t *testing.T) {
	out := validOutput()
	out.Nodes[0].ID = ""
	errs := ValidateOutput(out)
	assertContains(t, errs, "node[0]: id must be non-empty")
}

func TestValidateOutput_NodeMissingKind(t *testing.T) {
	out := validOutput()
	out.Nodes[0].Kind = ""
	errs := ValidateOutput(out)
	assertContains(t, errs, "node[0]: kind must be non-empty")
}

func TestValidateOutput_NodeMissingName(t *testing.T) {
	out := validOutput()
	out.Nodes[0].Name = ""
	errs := ValidateOutput(out)
	assertContains(t, errs, "node[0]: name must be non-empty")
}

func TestValidateOutput_NodeMissingLabel(t *testing.T) {
	out := validOutput()
	out.Nodes[0].Label = ""
	errs := ValidateOutput(out)
	assertContains(t, errs, "node[0]: label must be non-empty")
}

func TestValidateOutput_NodeInvalidKind(t *testing.T) {
	out := validOutput()
	out.Nodes[0].Kind = "widget"
	errs := ValidateOutput(out)
	assertContains(t, errs, "node[0]: invalid kind \"widget\"")
}

func TestValidateOutput_DuplicateNodeID(t *testing.T) {
	out := validOutput()
	out.Nodes[1].ID = out.Nodes[0].ID
	errs := ValidateOutput(out)
	assertContains(t, errs, "duplicate node id")
}

func TestValidateOutput_EdgeInvalidKind(t *testing.T) {
	out := validOutput()
	out.Edges[0].Kind = "bad_kind"
	errs := ValidateOutput(out)
	assertContains(t, errs, "edge[0]: invalid kind \"bad_kind\"")
}

func TestValidateOutput_EdgeBrokenSrcRef(t *testing.T) {
	out := validOutput()
	out.Edges[0].SrcID = "nonexistent"
	errs := ValidateOutput(out)
	assertContains(t, errs, "edge[0]: srcId \"nonexistent\" does not reference a known node")
}

func TestValidateOutput_EdgeBrokenDstRef(t *testing.T) {
	out := validOutput()
	out.Edges[0].DstID = "nonexistent"
	errs := ValidateOutput(out)
	assertContains(t, errs, "edge[0]: dstId \"nonexistent\" does not reference a known node")
}

func TestValidateOutput_AllNodeKinds(t *testing.T) {
	for _, kind := range []string{"route", "entity", "page", "action", "permission"} {
		out := &ScanOutput{
			Version: 1,
			Scanner: ScannerInfo{ID: "t", Name: "t", Version: "1"},
			Nodes: []ScanNode{
				{ID: "n1", Kind: kind, Name: "n", Label: "l", Properties: map[string]any{}, Source: "scan"},
			},
		}
		errs := ValidateOutput(out)
		if len(errs) > 0 {
			t.Errorf("kind %q should be valid, got errors: %v", kind, errs)
		}
	}
}

func TestValidateOutput_AllEdgeKinds(t *testing.T) {
	validEdgeKinds := []string{
		"uses_route", "touches_entity", "on_page",
		"requires_permission", "relates_to", "field_relation",
	}
	for _, kind := range validEdgeKinds {
		out := &ScanOutput{
			Version: 1,
			Scanner: ScannerInfo{ID: "t", Name: "t", Version: "1"},
			Nodes: []ScanNode{
				{ID: "a", Kind: "route", Name: "a", Label: "a", Properties: map[string]any{}, Source: "scan"},
				{ID: "b", Kind: "entity", Name: "b", Label: "b", Properties: map[string]any{}, Source: "scan"},
			},
			Edges: []ScanEdge{
				{ID: "e1", SrcID: "a", DstID: "b", Kind: kind},
			},
		}
		errs := ValidateOutput(out)
		if len(errs) > 0 {
			t.Errorf("edge kind %q should be valid, got errors: %v", kind, errs)
		}
	}
}

func assertContains(t *testing.T, errs []string, substr string) {
	t.Helper()
	for _, e := range errs {
		if contains(e, substr) {
			return
		}
	}
	t.Errorf("expected error containing %q, got %v", substr, errs)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
