package graph

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mjn/abacus/internal/db"
)

// ActionService wraps GraphRepository with validation, reference resolution,
// and the enrichment protocol for action nodes.
type ActionService struct {
	repo *GraphRepository
}

// NewActionService creates a new ActionService backed by the given repository.
func NewActionService(repo *GraphRepository) *ActionService {
	return &ActionService{repo: repo}
}

// CreateActionInput holds the data needed to create a new action node.
type CreateActionInput struct {
	Name            string
	Label           string
	GherkinPatterns []string
	RouteRefs       []string
	EntityRefs      []string
	PageRefs        []string
	PermissionRefs  []string
	Properties      map[string]any
}

// UpdateActionInput holds optional fields for updating an action node.
type UpdateActionInput struct {
	Label           *string
	GherkinPatterns []string
	RouteRefs       []string
	EntityRefs      []string
	PageRefs        []string
	PermissionRefs  []string
	Properties      map[string]any
}

// ActionResult holds the result of a create or update operation.
type ActionResult struct {
	Action   *db.GraphNode
	Edges    []db.GraphEdge
	Warnings []string
}

// ActionNode holds an action node with its resolved references.
type ActionNode struct {
	Node     db.GraphNode
	Routes   []db.GraphNode
	Entities []db.GraphNode
	Pages    []db.GraphNode
}

// ListActionOpts holds pagination options for listing actions.
type ListActionOpts struct {
	Limit  int
	Offset int
}

// SuggestContext holds parameters for suggesting related actions.
type SuggestContext struct {
	Query string
	Limit int
}

// ActionAudit holds the results of an action audit.
type ActionAudit struct {
	Total         int
	StaleActions  []StaleAction
	OrphanActions []string
}

// StaleAction holds information about an action with missing references.
type StaleAction struct {
	ActionID    string
	MissingRefs []string
}

// slugRe matches non-alphanumeric characters for slug generation.
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a name to a URL-friendly slug.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// Create creates a new action node with edges to referenced nodes.
func (s *ActionService) Create(input CreateActionInput) (*ActionResult, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("action name is required")
	}

	// Validate gherkin patterns
	for i, p := range input.GherkinPatterns {
		if strings.TrimSpace(p) == "" {
			return nil, fmt.Errorf("gherkin pattern at index %d is empty", i)
		}
		if len(p) > 1000 {
			return nil, fmt.Errorf("gherkin pattern at index %d exceeds 1000 characters", i)
		}
	}

	actionID := "action:" + slugify(input.Name)

	// Check uniqueness
	existing, err := s.repo.GetNode(actionID)
	if err != nil {
		return nil, fmt.Errorf("check uniqueness: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("action with name %q already exists (id=%s)", input.Name, actionID)
	}

	// Build properties
	props := make(map[string]any)
	for k, v := range input.Properties {
		props[k] = v
	}
	if len(input.GherkinPatterns) > 0 {
		props["gherkin_patterns"] = input.GherkinPatterns
	}

	node := &db.GraphNode{
		ID:         actionID,
		Kind:       db.NodeAction,
		Name:       input.Name,
		Label:      input.Label,
		Properties: props,
		Source:     db.SourceAgent,
	}

	if err := s.repo.InsertNode(node); err != nil {
		return nil, fmt.Errorf("insert action node: %w", err)
	}

	// Create edges for references
	var edges []db.GraphEdge
	var warnings []string

	refSets := []struct {
		refs []string
		kind db.EdgeKind
	}{
		{input.RouteRefs, db.EdgeUsesRoute},
		{input.EntityRefs, db.EdgeTouchesEntity},
		{input.PageRefs, db.EdgeOnPage},
		{input.PermissionRefs, db.EdgeRequiresPermission},
	}

	for _, rs := range refSets {
		for _, ref := range rs.refs {
			edgeID := fmt.Sprintf("edge:%s-%s-%s", actionID, rs.kind, ref)
			edge := &db.GraphEdge{
				ID:    edgeID,
				SrcID: actionID,
				DstID: ref,
				Kind:  rs.kind,
			}
			if err := s.repo.InsertEdge(edge); err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to create edge to %s: %v", ref, err))
			} else {
				edges = append(edges, *edge)
			}
		}
	}

	// Re-read node to get timestamps set by DB
	created, err := s.repo.GetNode(actionID)
	if err != nil {
		return nil, fmt.Errorf("re-read action: %w", err)
	}

	return &ActionResult{
		Action:   created,
		Edges:    edges,
		Warnings: warnings,
	}, nil
}

// Update updates an existing action node.
func (s *ActionService) Update(id string, input UpdateActionInput) (*ActionResult, error) {
	existing, err := s.repo.GetNode(id)
	if err != nil {
		return nil, fmt.Errorf("get action: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("action %q not found", id)
	}
	if existing.Kind != db.NodeAction {
		return nil, fmt.Errorf("node %q is not an action (kind=%s)", id, existing.Kind)
	}

	// Update label if provided
	if input.Label != nil {
		existing.Label = *input.Label
	}

	// Update gherkin patterns if provided
	if input.GherkinPatterns != nil {
		if existing.Properties == nil {
			existing.Properties = make(map[string]any)
		}
		existing.Properties["gherkin_patterns"] = input.GherkinPatterns
	}

	// Merge properties if provided
	if input.Properties != nil {
		if existing.Properties == nil {
			existing.Properties = make(map[string]any)
		}
		for k, v := range input.Properties {
			existing.Properties[k] = v
		}
	}

	// Upsert the node with updated fields
	if err := s.repo.UpsertNode(existing); err != nil {
		return nil, fmt.Errorf("upsert action: %w", err)
	}

	var warnings []string

	// Update edges for each reference type if provided
	refSets := []struct {
		refs *[]string
		kind db.EdgeKind
	}{
		{nilIfEmpty(input.RouteRefs), db.EdgeUsesRoute},
		{nilIfEmpty(input.EntityRefs), db.EdgeTouchesEntity},
		{nilIfEmpty(input.PageRefs), db.EdgeOnPage},
		{nilIfEmpty(input.PermissionRefs), db.EdgeRequiresPermission},
	}

	for _, rs := range refSets {
		if rs.refs == nil {
			continue
		}
		// Delete existing edges of this kind
		if err := s.deleteEdgesFromByKind(id, rs.kind); err != nil {
			return nil, fmt.Errorf("delete edges: %w", err)
		}
		// Create new edges
		for _, ref := range *rs.refs {
			edgeID := fmt.Sprintf("edge:%s-%s-%s", id, rs.kind, ref)
			edge := &db.GraphEdge{
				ID:    edgeID,
				SrcID: id,
				DstID: ref,
				Kind:  rs.kind,
			}
			if err := s.repo.InsertEdge(edge); err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to create edge to %s: %v", ref, err))
			}
		}
	}

	// Re-read to get updated timestamps
	updated, err := s.repo.GetNode(id)
	if err != nil {
		return nil, fmt.Errorf("re-read action: %w", err)
	}

	// Get all edges for the result
	allEdges, err := s.repo.GetEdgesFrom(id, nil)
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}

	return &ActionResult{
		Action:   updated,
		Edges:    allEdges,
		Warnings: warnings,
	}, nil
}

// Get retrieves an action node with resolved references.
func (s *ActionService) Get(id string) (*ActionNode, error) {
	node, err := s.repo.GetNode(id)
	if err != nil {
		return nil, fmt.Errorf("get action: %w", err)
	}
	if node == nil {
		return nil, nil
	}
	if node.Kind != db.NodeAction {
		return nil, fmt.Errorf("node %q is not an action (kind=%s)", id, node.Kind)
	}

	result := &ActionNode{Node: *node}

	// Get all outgoing edges
	edges, err := s.repo.GetEdgesFrom(id, nil)
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}

	// Resolve references by edge kind
	for _, edge := range edges {
		target, err := s.repo.GetNode(edge.DstID)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %s: %w", edge.DstID, err)
		}
		if target == nil {
			continue // Target was deleted
		}
		switch edge.Kind {
		case db.EdgeUsesRoute:
			result.Routes = append(result.Routes, *target)
		case db.EdgeTouchesEntity:
			result.Entities = append(result.Entities, *target)
		case db.EdgeOnPage:
			result.Pages = append(result.Pages, *target)
		}
	}

	return result, nil
}

// List returns action nodes with pagination. References are not resolved.
func (s *ActionService) List(opts ListActionOpts) ([]ActionNode, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	nodes, err := s.repo.GetNodesByKind(db.NodeAction, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}

	result := make([]ActionNode, len(nodes))
	for i, n := range nodes {
		result[i] = ActionNode{Node: n}
	}
	return result, nil
}

// Delete removes an action node. Edges are cascade-deleted by the DB.
func (s *ActionService) Delete(id string) error {
	return s.repo.DeleteNode(id)
}

// Suggest finds actions matching the query text via FTS5 search.
func (s *ActionService) Suggest(ctx SuggestContext) ([]ActionNode, error) {
	if ctx.Limit <= 0 {
		ctx.Limit = 10
	}
	kind := db.NodeAction
	results, err := s.repo.Search(ctx.Query, &kind, ctx.Limit)
	if err != nil {
		return nil, fmt.Errorf("suggest: %w", err)
	}

	nodes := make([]ActionNode, len(results))
	for i, r := range results {
		nodes[i] = ActionNode{Node: r.Node}
	}
	return nodes, nil
}

// Audit inspects all action nodes for stale references and orphans.
func (s *ActionService) Audit() (*ActionAudit, error) {
	actions, err := s.repo.GetNodesByKind(db.NodeAction, 10000, 0)
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}

	audit := &ActionAudit{Total: len(actions)}

	for _, action := range actions {
		edges, err := s.repo.GetEdgesFrom(action.ID, nil)
		if err != nil {
			return nil, fmt.Errorf("get edges for %s: %w", action.ID, err)
		}

		if len(edges) == 0 {
			audit.OrphanActions = append(audit.OrphanActions, action.ID)
			continue
		}

		// Check for stale refs (edges pointing to deleted nodes)
		var missingRefs []string
		for _, edge := range edges {
			target, err := s.repo.GetNode(edge.DstID)
			if err != nil {
				return nil, fmt.Errorf("check ref %s: %w", edge.DstID, err)
			}
			if target == nil {
				missingRefs = append(missingRefs, edge.DstID)
			}
		}
		if len(missingRefs) > 0 {
			audit.StaleActions = append(audit.StaleActions, StaleAction{
				ActionID:    action.ID,
				MissingRefs: missingRefs,
			})
		}
	}

	return audit, nil
}

// deleteEdgesFromByKind deletes all edges from a node with the given kind.
func (s *ActionService) deleteEdgesFromByKind(nodeID string, kind db.EdgeKind) error {
	_, err := s.repo.database.Exec(
		"DELETE FROM edges WHERE src_id = ? AND kind = ?",
		nodeID, string(kind),
	)
	if err != nil {
		return fmt.Errorf("delete edges from %s of kind %s: %w", nodeID, kind, err)
	}
	return nil
}

// nilIfEmpty returns nil if the slice is nil, otherwise returns a pointer to the slice.
// This distinguishes "not provided" (nil) from "set to empty" (empty slice).
func nilIfEmpty(s []string) *[]string {
	if s == nil {
		return nil
	}
	return &s
}
