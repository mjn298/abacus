package db

import "strings"

// NodeKind enumerates the types of nodes in the application graph.
type NodeKind string

const (
	NodeRoute      NodeKind = "route"
	NodeEntity     NodeKind = "entity"
	NodePage       NodeKind = "page"
	NodeAction     NodeKind = "action"
	NodePermission NodeKind = "permission"
	NodeModule     NodeKind = "module"
)

// AllNodeKinds is the single source of truth for all valid node kinds.
var AllNodeKinds = []NodeKind{NodeRoute, NodeEntity, NodePage, NodeAction, NodePermission, NodeModule}

// NodeSource indicates how a node was created.
type NodeSource string

const (
	SourceScan   NodeSource = "scan"
	SourceAgent  NodeSource = "agent"
	SourceManual NodeSource = "manual"
)

// EdgeKind enumerates the types of edges in the application graph.
type EdgeKind string

const (
	EdgeUsesRoute          EdgeKind = "uses_route"
	EdgeTouchesEntity      EdgeKind = "touches_entity"
	EdgeOnPage             EdgeKind = "on_page"
	EdgeRequiresPermission EdgeKind = "requires_permission"
	EdgeRelatesTo          EdgeKind = "relates_to"
	EdgeFieldRelation      EdgeKind = "field_relation"
	EdgeDelegatesTo        EdgeKind = "delegates_to"
)

// AllEdgeKinds is the single source of truth for all valid edge kinds.
var AllEdgeKinds = []EdgeKind{EdgeUsesRoute, EdgeTouchesEntity, EdgeOnPage, EdgeRequiresPermission, EdgeRelatesTo, EdgeFieldRelation, EdgeDelegatesTo}

// GraphNode represents a node in the application graph.
type GraphNode struct {
	ID         string         `json:"id"`
	Kind       NodeKind       `json:"kind"`
	Name       string         `json:"name"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Source     NodeSource     `json:"source"`
	SourceFile *string        `json:"source_file"`
	CreatedAt  int64          `json:"created_at"`
	UpdatedAt  int64          `json:"updated_at"`
	ScanHash   *string        `json:"scan_hash"`
}

// GherkinPatterns extracts gherkin_patterns from the node's properties.
func (n GraphNode) GherkinPatterns() []string {
	raw, ok := n.Properties["gherkin_patterns"]
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

// GraphEdge represents an edge in the application graph.
type GraphEdge struct {
	ID            string         `json:"id"`
	SrcID         string         `json:"src_id"`
	DstID         string         `json:"dst_id"`
	Kind          EdgeKind       `json:"kind"`
	Properties    map[string]any `json:"properties"`
	SourceScanner *string        `json:"source_scanner,omitempty"`
	CreatedAt     int64          `json:"created_at"`
}

// SanitizeFTS5Query wraps input in double quotes to treat it as a literal
// phrase, neutralizing any FTS5 operators or special syntax. Double quotes
// within the input are escaped by doubling them.
func SanitizeFTS5Query(input string) string {
	if input == "" {
		return ""
	}
	escaped := strings.ReplaceAll(input, `"`, `""`)
	return `"` + escaped + `"`
}
