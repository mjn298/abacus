package db

// NodeKind enumerates the types of nodes in the application graph.
type NodeKind string

const (
	NodeRoute      NodeKind = "route"
	NodeEntity     NodeKind = "entity"
	NodePage       NodeKind = "page"
	NodeAction     NodeKind = "action"
	NodePermission NodeKind = "permission"
)

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
)

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
