package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mjn/abacus/internal/config"
	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/match"
	"github.com/mjn/abacus/internal/scanner"
	"time"
)

// AbacusServer wraps the MCP server with all abacus services.
type AbacusServer struct {
	server  *gomcp.Server
	repo    *graph.GraphRepository
	actions *graph.ActionService
	matcher *match.MatchService
	runner  *scanner.Runner
	cfg     *config.Config
	db      *sql.DB
}

// NewAbacusServer creates a new AbacusServer, opening the DB and loading config.
func NewAbacusServer(dbPath, configPath string) (*AbacusServer, error) {
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.InitSchema(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)
	matcher := match.NewMatchService(repo, actions, match.MatchOptions{})
	runner := scanner.NewRunner(60 * time.Second)

	var cfg *config.Config
	if configPath != "" {
		cfg, err = config.Load(configPath)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	srv := &AbacusServer{
		server:  gomcp.NewServer(&gomcp.Implementation{Name: "abacus", Version: "0.1.0"}, nil),
		repo:    repo,
		actions: actions,
		matcher: matcher,
		runner:  runner,
		cfg:     cfg,
		db:      database,
	}

	srv.registerTools()

	return srv, nil
}

// Run starts the MCP server on stdio.
func (s *AbacusServer) Run(ctx context.Context) error {
	defer s.db.Close()
	return s.server.Run(ctx, &gomcp.StdioTransport{})
}

// --- Input types ---

// QueryNodesInput is used by query_routes, query_entities, query_pages.
type QueryNodesInput struct {
	Query string `json:"query,omitempty" jsonschema:"optional FTS5 search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results, default 50"`
}

// QueryActionsInput is used by query_actions.
type QueryActionsInput struct {
	Query string `json:"query,omitempty" jsonschema:"optional FTS5 search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results, default 50"`
}

// CreateActionInput is the MCP tool input for creating an action.
type CreateActionInput struct {
	Name            string   `json:"name" jsonschema:"required,name of the action"`
	Label           string   `json:"label,omitempty" jsonschema:"human-readable description"`
	GherkinPatterns []string `json:"gherkin_patterns,omitempty" jsonschema:"cucumber expression patterns"`
	RouteRefs       []string `json:"route_refs,omitempty" jsonschema:"IDs of referenced route nodes"`
	EntityRefs      []string `json:"entity_refs,omitempty" jsonschema:"IDs of referenced entity nodes"`
	PageRefs        []string `json:"page_refs,omitempty" jsonschema:"IDs of referenced page nodes"`
	PermissionRefs  []string `json:"permission_refs,omitempty" jsonschema:"IDs of referenced permission nodes"`
}

// UpdateActionInput is the MCP tool input for updating an action.
type UpdateActionInput struct {
	ID              string   `json:"id" jsonschema:"required,action node ID"`
	Label           *string  `json:"label,omitempty" jsonschema:"updated human-readable description"`
	GherkinPatterns []string `json:"gherkin_patterns,omitempty" jsonschema:"updated cucumber expression patterns"`
	RouteRefs       []string `json:"route_refs,omitempty" jsonschema:"updated route node IDs"`
	EntityRefs      []string `json:"entity_refs,omitempty" jsonschema:"updated entity node IDs"`
	PageRefs        []string `json:"page_refs,omitempty" jsonschema:"updated page node IDs"`
	PermissionRefs  []string `json:"permission_refs,omitempty" jsonschema:"updated permission node IDs"`
}

// MatchStepInput is the MCP tool input for matching a Gherkin step.
type MatchStepInput struct {
	StepText string `json:"step_text" jsonschema:"required,the Gherkin step text to match"`
}

// MatchScenarioInput is the MCP tool input for matching a scenario.
type MatchScenarioInput struct {
	Steps []StepInput `json:"steps" jsonschema:"required,list of Gherkin steps"`
}

// StepInput represents a single Gherkin step.
type StepInput struct {
	Keyword string `json:"keyword" jsonschema:"Given, When, Then, etc."`
	Text    string `json:"text" jsonschema:"the step text without keyword"`
}

// GraphContextInput is the MCP tool input for getting connected subgraph.
type GraphContextInput struct {
	NodeID   string `json:"node_id" jsonschema:"required,ID of the center node"`
	MaxDepth int    `json:"max_depth,omitempty" jsonschema:"max traversal depth, default 2"`
}

// StatsInput is the MCP tool input for graph statistics (empty).
type StatsInput struct{}

// ScanInput is the MCP tool input for running scanners.
type ScanInput struct{}

// --- Tool registration ---

func (s *AbacusServer) registerTools() {
	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.scan",
		Description: "Run all configured scanners to discover routes, entities, and pages, then ingest results into the graph",
	}, s.scanHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.query_routes",
		Description: "List or search route nodes in the application graph",
	}, s.queryRoutesHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.query_entities",
		Description: "List or search entity nodes in the application graph",
	}, s.queryEntitiesHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.query_pages",
		Description: "List or search page nodes in the application graph",
	}, s.queryPagesHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.query_actions",
		Description: "List or search action nodes in the application graph",
	}, s.queryActionsHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.create_action",
		Description: "Create a new action node with optional Gherkin patterns and references to routes, entities, and pages",
	}, s.createActionHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.update_action",
		Description: "Update an existing action node's label, patterns, or references",
	}, s.updateActionHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.match_step",
		Description: "Match a Gherkin step text against the action graph using 3-tier matching (exact, fuzzy, suggest)",
	}, s.matchStepHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.match_scenario",
		Description: "Match all steps in a Gherkin scenario against the action graph",
	}, s.matchScenarioHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.graph_context",
		Description: "Get the connected subgraph around a node, traversing edges in both directions up to max_depth",
	}, s.graphContextHandler)

	gomcp.AddTool(s.server, &gomcp.Tool{
		Name:        "abacus.stats",
		Description: "Get graph statistics including node counts per kind",
	}, s.statsHandler)
}

// --- Handlers ---

func (s *AbacusServer) scanHandler(ctx context.Context, req *gomcp.CallToolRequest, input ScanInput) (*gomcp.CallToolResult, any, error) {
	if s.cfg == nil {
		return nil, nil, fmt.Errorf("no config loaded; run 'abacus init' first")
	}

	configs := make([]config.ScannerConfig, 0, len(s.cfg.Scanners))
	for _, sc := range s.cfg.Scanners {
		configs = append(configs, sc)
	}

	merged, err := s.runner.RunAll(ctx, s.cfg.Project.Root, configs, s.cfg.Project.IgnorePaths)
	if err != nil {
		return nil, nil, fmt.Errorf("run scanners: %w", err)
	}

	// Ingest nodes
	graphNodes := make([]db.GraphNode, len(merged.Nodes))
	for i, sn := range merged.Nodes {
		var sf *string
		if sn.SourceFile != "" {
			sf = &sn.SourceFile
		}
		graphNodes[i] = db.GraphNode{
			ID:         sn.ID,
			Kind:       db.NodeKind(sn.Kind),
			Name:       sn.Name,
			Label:      sn.Label,
			Properties: sn.Properties,
			Source:     db.NodeSource(sn.Source),
			SourceFile: sf,
		}
	}

	count, err := s.repo.BulkUpsertNodes(graphNodes)
	if err != nil {
		return nil, nil, fmt.Errorf("ingest nodes: %w", err)
	}

	result := map[string]any{
		"nodes_ingested": count,
		"edges_found":    len(merged.Edges),
		"warnings":       len(merged.Warnings),
		"errors":         merged.Errors,
		"stats":          merged.Stats,
	}

	return jsonResult(result)
}

func (s *AbacusServer) queryRoutesHandler(ctx context.Context, req *gomcp.CallToolRequest, input QueryNodesInput) (*gomcp.CallToolResult, any, error) {
	return s.queryNodesByKind(db.NodeRoute, input)
}

func (s *AbacusServer) queryEntitiesHandler(ctx context.Context, req *gomcp.CallToolRequest, input QueryNodesInput) (*gomcp.CallToolResult, any, error) {
	return s.queryNodesByKind(db.NodeEntity, input)
}

func (s *AbacusServer) queryPagesHandler(ctx context.Context, req *gomcp.CallToolRequest, input QueryNodesInput) (*gomcp.CallToolResult, any, error) {
	return s.queryNodesByKind(db.NodePage, input)
}

func (s *AbacusServer) queryNodesByKind(kind db.NodeKind, input QueryNodesInput) (*gomcp.CallToolResult, any, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	if input.Query != "" {
		results, err := s.repo.Search(input.Query, &kind, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("search %s: %w", kind, err)
		}
		nodes := make([]db.GraphNode, len(results))
		for i, r := range results {
			nodes[i] = r.Node
		}
		return jsonResult(nodes)
	}

	nodes, err := s.repo.GetNodesByKind(kind, limit, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("list %s: %w", kind, err)
	}
	return jsonResult(nodes)
}

func (s *AbacusServer) queryActionsHandler(ctx context.Context, req *gomcp.CallToolRequest, input QueryActionsInput) (*gomcp.CallToolResult, any, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	if input.Query != "" {
		suggestions, err := s.actions.Suggest(graph.SuggestContext{Query: input.Query, Limit: limit})
		if err != nil {
			return nil, nil, fmt.Errorf("search actions: %w", err)
		}
		return jsonResult(suggestions)
	}

	actions, err := s.actions.List(graph.ListActionOpts{Limit: limit})
	if err != nil {
		return nil, nil, fmt.Errorf("list actions: %w", err)
	}
	return jsonResult(actions)
}

func (s *AbacusServer) createActionHandler(ctx context.Context, req *gomcp.CallToolRequest, input CreateActionInput) (*gomcp.CallToolResult, any, error) {
	result, err := s.actions.Create(graph.CreateActionInput{
		Name:            input.Name,
		Label:           input.Label,
		GherkinPatterns: input.GherkinPatterns,
		RouteRefs:       input.RouteRefs,
		EntityRefs:      input.EntityRefs,
		PageRefs:        input.PageRefs,
		PermissionRefs:  input.PermissionRefs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create action: %w", err)
	}
	return jsonResult(result)
}

func (s *AbacusServer) updateActionHandler(ctx context.Context, req *gomcp.CallToolRequest, input UpdateActionInput) (*gomcp.CallToolResult, any, error) {
	result, err := s.actions.Update(input.ID, graph.UpdateActionInput{
		Label:           input.Label,
		GherkinPatterns: input.GherkinPatterns,
		RouteRefs:       input.RouteRefs,
		EntityRefs:      input.EntityRefs,
		PageRefs:        input.PageRefs,
		PermissionRefs:  input.PermissionRefs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("update action: %w", err)
	}
	return jsonResult(result)
}

func (s *AbacusServer) matchStepHandler(ctx context.Context, req *gomcp.CallToolRequest, input MatchStepInput) (*gomcp.CallToolResult, any, error) {
	result, err := s.matcher.Match(input.StepText)
	if err != nil {
		return nil, nil, fmt.Errorf("match step: %w", err)
	}
	return jsonResult(result)
}

func (s *AbacusServer) matchScenarioHandler(ctx context.Context, req *gomcp.CallToolRequest, input MatchScenarioInput) (*gomcp.CallToolResult, any, error) {
	steps := make([]match.Step, len(input.Steps))
	for i, si := range input.Steps {
		steps[i] = match.Step{Keyword: si.Keyword, Text: si.Text}
	}

	results, err := s.matcher.MatchScenario(steps)
	if err != nil {
		return nil, nil, fmt.Errorf("match scenario: %w", err)
	}
	return jsonResult(results)
}

func (s *AbacusServer) graphContextHandler(ctx context.Context, req *gomcp.CallToolRequest, input GraphContextInput) (*gomcp.CallToolResult, any, error) {
	maxDepth := input.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2
	}

	subgraph, err := s.repo.GetConnected(input.NodeID, maxDepth)
	if err != nil {
		return nil, nil, fmt.Errorf("get connected: %w", err)
	}
	return jsonResult(subgraph)
}

func (s *AbacusServer) statsHandler(ctx context.Context, req *gomcp.CallToolRequest, input StatsInput) (*gomcp.CallToolResult, any, error) {
	kinds := []db.NodeKind{db.NodeRoute, db.NodeEntity, db.NodePage, db.NodeAction, db.NodePermission}
	stats := make(map[string]int)

	for _, kind := range kinds {
		nodes, err := s.repo.GetNodesByKind(kind, 100000, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("count %s: %w", kind, err)
		}
		stats[string(kind)] = len(nodes)
	}

	return jsonResult(stats)
}

// jsonResult marshals v as JSON and returns it as a CallToolResult with TextContent.
func jsonResult(v any) (*gomcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
	}, nil, nil
}
