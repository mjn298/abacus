package graph

import (
	"fmt"

	"github.com/mjn/abacus/internal/db"
)

// GenesisInput holds the input for a genesis query. Exactly one of Step,
// Entity, or NodeID must be provided.
type GenesisInput struct {
	Step   string // Gherkin step text to classify and resolve
	Entity string // Entity name to show implementation pattern for
	NodeID string // Node ID to traverse from directly
	Depth  int    // Max traversal depth (default 5)
}

// NodeRef is a lightweight reference to a graph node for output.
type NodeRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SourceFile string `json:"source_file,omitempty"`
}

// GenesisResult holds the output of a genesis query.
type GenesisResult struct {
	Classification   string     `json:"classification,omitempty"`   // "covered", "wirable", "new" (only for step input)
	MatchTier        string     `json:"match_tier,omitempty"`       // "exact", "fuzzy", "suggest" (only for step input)
	Action           *ActionRef `json:"action,omitempty"`           // matched action (only for exact tier)
	Routes           []NodeRef  `json:"routes"`
	Entities         []NodeRef  `json:"entities"`
	Modules          []NodeRef  `json:"modules"`
	Pages            []NodeRef  `json:"pages"`
	DelegationChains [][]string `json:"delegation_chains"` // ordered node ID paths
}

// ActionRef holds a lightweight reference to an action for genesis output.
type ActionRef struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	GherkinPatterns []string `json:"gherkin_patterns,omitempty"`
}

// StepMatchResult is the genesis-facing view of a match result, decoupled
// from the match package to avoid an import cycle.
type StepMatchResult struct {
	Tier   string      // "exact", "fuzzy", "suggest"
	Action *ActionNode // resolved action (exact tier only)
	// SuggestRoutes and SuggestEntities hold the related-context nodes
	// surfaced by the suggestion tier.
	SuggestRoutes   []db.GraphNode
	SuggestEntities []db.GraphNode
}

// StepMatcher abstracts the match.MatchService so the graph package can
// consume it without importing match (which already imports graph).
type StepMatcher interface {
	MatchForGenesis(stepText string) (*StepMatchResult, error)
}

// maxGenesisDepth is the maximum traversal depth allowed for genesis queries.
// Silently clamped, not rejected — consistent with DoS prevention pattern.
const maxGenesisDepth = 10

// maxChainLength caps delegation chain length to prevent pathological DFS.
const maxChainLength = 20

// maxGenesisEdgeFanout caps the number of inbound edges processed for entity
// queries to prevent DoS from high-connectivity entities.
const maxGenesisEdgeFanout = 50

// GenesisService composes match, graph traversal, and categorization.
type GenesisService struct {
	repo    *GraphRepository
	matcher StepMatcher
}

// NewGenesisService creates a new GenesisService.
func NewGenesisService(repo *GraphRepository, matcher StepMatcher) *GenesisService {
	return &GenesisService{repo: repo, matcher: matcher}
}

// Genesis executes a composite query that resolves a step, entity, or node ID
// into a classified result with traversed graph context and delegation chains.
func (s *GenesisService) Genesis(input GenesisInput) (*GenesisResult, error) {
	// Validate: exactly one input field must be set.
	count := 0
	if input.Step != "" {
		count++
	}
	if input.Entity != "" {
		count++
	}
	if input.NodeID != "" {
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("genesis: exactly one of Step, Entity, or NodeID must be provided")
	}
	if count > 1 {
		return nil, fmt.Errorf("genesis: only one of Step, Entity, or NodeID may be provided")
	}

	// Default and cap depth.
	if input.Depth <= 0 {
		input.Depth = 5
	}
	if input.Depth > maxGenesisDepth {
		input.Depth = maxGenesisDepth
	}

	result := &GenesisResult{}
	var startNodeIDs []string

	// Declare before switch — entity case overrides this.
	edgeKinds := []db.EdgeKind{db.EdgeDelegatesTo, db.EdgeTouchesEntity}

	// entityTouchEdges holds the single-hop touches_entity edges found during
	// entity-mode reverse lookup. These are injected into the subgraph after
	// traversal so ExtractDelegationChains can terminate chains at the entity.
	var entityTouchEdges []db.GraphEdge

	switch {
	case input.Step != "":
		matchResult, err := s.matcher.MatchForGenesis(input.Step)
		if err != nil {
			return nil, fmt.Errorf("genesis match: %w", err)
		}

		result.MatchTier = matchResult.Tier

		switch matchResult.Tier {
		case "exact":
			result.Classification = "covered"
			if matchResult.Action != nil {
				result.Action = &ActionRef{
					ID:              matchResult.Action.Node.ID,
					Name:            matchResult.Action.Node.Name,
					GherkinPatterns: matchResult.Action.Node.GherkinPatterns(),
				}
				for _, r := range matchResult.Action.Routes {
					startNodeIDs = append(startNodeIDs, r.ID)
				}
				for _, e := range matchResult.Action.Entities {
					startNodeIDs = append(startNodeIDs, e.ID)
				}
			}

		case "fuzzy":
			// Fuzzy is informational only — no classification, no traversal.
			return result, nil

		case "suggest":
			if len(matchResult.SuggestRoutes) > 0 || len(matchResult.SuggestEntities) > 0 {
				result.Classification = "wirable"
			} else {
				result.Classification = "new"
			}
			for _, r := range matchResult.SuggestRoutes {
				startNodeIDs = append(startNodeIDs, r.ID)
			}
			for _, e := range matchResult.SuggestEntities {
				startNodeIDs = append(startNodeIDs, e.ID)
			}
		}

	case input.Entity != "":
		kind := db.NodeEntity
		results, err := s.repo.Search(db.SanitizeFTS5ColumnQuery("name", input.Entity), &kind, 10)
		if err != nil {
			return nil, fmt.Errorf("genesis entity search: %w", err)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no entity found matching %q", input.Entity)
		}
		entityNode := results[0].Node

		// Reverse lookup: find nodes that directly touch this entity.
		touchKind := db.EdgeTouchesEntity
		touchingEdges, err := s.repo.GetEdgesTo(entityNode.ID, &touchKind)
		if err != nil {
			return nil, fmt.Errorf("genesis entity edges: %w", err)
		}

		// Cap fanout to prevent DoS from high-connectivity entities.
		if len(touchingEdges) > maxGenesisEdgeFanout {
			touchingEdges = touchingEdges[:maxGenesisEdgeFanout]
		}

		// Deduplicate source node IDs (same node may have multiple edges to entity).
		seen := make(map[string]bool)
		for _, e := range touchingEdges {
			if !seen[e.SrcID] {
				seen[e.SrcID] = true
				startNodeIDs = append(startNodeIDs, e.SrcID)
			}
		}

		// Include the entity itself so it appears in output.
		startNodeIDs = append(startNodeIDs, entityNode.ID)

		// Save touching edges for chain extraction (injected after traversal).
		entityTouchEdges = touchingEdges

		// Override: only traverse delegation chains, NOT touches_entity.
		// touches_entity was already used above for the single-hop reverse lookup.
		edgeKinds = []db.EdgeKind{db.EdgeDelegatesTo}

	case input.NodeID != "":
		node, err := s.repo.GetNode(input.NodeID)
		if err != nil {
			return nil, fmt.Errorf("genesis get node: %w", err)
		}
		if node == nil {
			return nil, fmt.Errorf("node %q not found", input.NodeID)
		}
		startNodeIDs = append(startNodeIDs, node.ID)
	}

	// If no start nodes (e.g., "new" classification with empty context), return early.
	if len(startNodeIDs) == 0 {
		return result, nil
	}

	// Build set of queried start nodes for post-filtering delegation chains.
	startSet := make(map[string]bool, len(startNodeIDs))
	for _, id := range startNodeIDs {
		startSet[id] = true
	}

	// Traverse and merge subgraphs.
	allNodes := make(map[string]db.GraphNode)
	allEdges := make(map[string]db.GraphEdge)

	for _, nodeID := range startNodeIDs {
		sg, err := s.repo.GetConnected(nodeID, input.Depth, edgeKinds)
		if err != nil {
			return nil, fmt.Errorf("genesis traverse from %s: %w", nodeID, err)
		}
		for _, n := range sg.Nodes {
			allNodes[n.ID] = n
		}
		for _, e := range sg.Edges {
			allEdges[e.ID] = e
		}
	}

	// Inject entity-mode touching edges so chains can terminate at the entity.
	for _, e := range entityTouchEdges {
		allEdges[e.ID] = e
	}

	// Categorize nodes into typed buckets.
	for _, n := range allNodes {
		ref := nodeRefFromGraphNode(n)
		switch n.Kind {
		case db.NodeRoute:
			result.Routes = append(result.Routes, ref)
		case db.NodeEntity:
			result.Entities = append(result.Entities, ref)
		case db.NodeModule:
			result.Modules = append(result.Modules, ref)
		case db.NodePage:
			result.Pages = append(result.Pages, ref)
		}
	}

	// Extract delegation chains.
	nodes := make([]db.GraphNode, 0, len(allNodes))
	for _, n := range allNodes {
		nodes = append(nodes, n)
	}
	edges := make([]db.GraphEdge, 0, len(allEdges))
	for _, e := range allEdges {
		edges = append(edges, e)
	}
	result.DelegationChains = ExtractDelegationChains(nodes, edges)

	// Filter chains to only those terminating at a queried start node.
	// This prunes noise from hub nodes (e.g., authz middleware) that
	// connect to every entity in the graph.
	filtered := result.DelegationChains[:0]
	for _, chain := range result.DelegationChains {
		if len(chain) > 0 && startSet[chain[len(chain)-1]] {
			filtered = append(filtered, chain)
		}
	}
	result.DelegationChains = filtered

	return result, nil
}

// edgeTarget pairs a destination node ID with the edge kind.
type edgeTarget struct {
	dstID string
	kind  db.EdgeKind
}

// ExtractDelegationChains walks edges to build ordered delegation chains.
// Each chain starts from a route node, follows delegates_to edges through
// modules, and terminates when reaching an entity via touches_entity.
func ExtractDelegationChains(nodes []db.GraphNode, edges []db.GraphEdge) [][]string {
	// Build forward adjacency: src → [(dst, edgeKind)].
	adj := make(map[string][]edgeTarget)
	for _, e := range edges {
		if e.Kind == db.EdgeDelegatesTo || e.Kind == db.EdgeTouchesEntity {
			adj[e.SrcID] = append(adj[e.SrcID], edgeTarget{dstID: e.DstID, kind: e.Kind})
		}
	}

	// Identify route nodes as starting points.
	routeIDs := make([]string, 0)
	for _, n := range nodes {
		if n.Kind == db.NodeRoute {
			routeIDs = append(routeIDs, n.ID)
		}
	}

	var chains [][]string

	for _, routeID := range routeIDs {
		visited := make(map[string]bool)
		path := []string{routeID}
		visited[routeID] = true
		dfsChains(routeID, path, visited, adj, &chains)
	}

	return chains
}

// dfsChains performs iterative-style DFS via recursion with backtracking to
// collect delegation chains. A chain is emitted when a touches_entity edge
// is followed to a terminal entity node.
func dfsChains(current string, path []string, visited map[string]bool, adj map[string][]edgeTarget, chains *[][]string) {
	if len(path) >= maxChainLength {
		return
	}

	for _, target := range adj[current] {
		if visited[target.dstID] {
			continue
		}

		switch target.kind {
		case db.EdgeDelegatesTo:
			// Continue traversal through delegation.
			// Explicit copy avoids append aliasing across sibling iterations.
			newPath := make([]string, len(path)+1)
			copy(newPath, path)
			newPath[len(path)] = target.dstID
			visited[target.dstID] = true
			dfsChains(target.dstID, newPath, visited, adj, chains)
			visited[target.dstID] = false

		case db.EdgeTouchesEntity:
			// Terminal: emit completed chain including the entity.
			chain := make([]string, len(path)+1)
			copy(chain, path)
			chain[len(path)] = target.dstID
			*chains = append(*chains, chain)
		}
	}
}

// nodeRefFromGraphNode converts a db.GraphNode to a lightweight NodeRef.
func nodeRefFromGraphNode(n db.GraphNode) NodeRef {
	ref := NodeRef{
		ID:   n.ID,
		Name: n.Name,
	}
	if n.SourceFile != nil {
		ref.SourceFile = *n.SourceFile
	}
	return ref
}
