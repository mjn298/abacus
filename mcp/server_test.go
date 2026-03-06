package mcp

import (
	"context"
	"encoding/json"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/match"
)

// setupTestServer creates an AbacusServer backed by an in-memory SQLite DB.
func setupTestServer(t *testing.T) *AbacusServer {
	t.Helper()

	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)
	matcher := match.NewMatchService(repo, actions, match.MatchOptions{})

	srv := &AbacusServer{
		server:  gomcp.NewServer(&gomcp.Implementation{Name: "abacus-test", Version: "0.1.0"}, nil),
		repo:    repo,
		actions: actions,
		matcher: matcher,
		db:      database,
	}
	srv.registerTools()

	return srv
}

// seedNodes inserts test nodes into the DB.
func seedNodes(t *testing.T, repo *graph.GraphRepository) {
	t.Helper()

	nodes := []db.GraphNode{
		{ID: "route:get-users", Kind: db.NodeRoute, Name: "GET /users", Label: "List users", Properties: map[string]any{"method": "GET", "path": "/users"}, Source: db.SourceScan},
		{ID: "route:post-users", Kind: db.NodeRoute, Name: "POST /users", Label: "Create user", Properties: map[string]any{"method": "POST", "path": "/users"}, Source: db.SourceScan},
		{ID: "entity:user", Kind: db.NodeEntity, Name: "User", Label: "User entity", Properties: map[string]any{"fields": []string{"id", "name", "email"}}, Source: db.SourceScan},
		{ID: "page:dashboard", Kind: db.NodePage, Name: "Dashboard", Label: "Main dashboard", Properties: map[string]any{}, Source: db.SourceScan},
	}

	for _, n := range nodes {
		if err := repo.InsertNode(&n); err != nil {
			t.Fatalf("insert node %s: %v", n.ID, err)
		}
	}
}

func TestNewAbacusServer(t *testing.T) {
	// Use in-memory DB path
	srv, err := NewAbacusServer(":memory:", "")
	if err != nil {
		t.Fatalf("NewAbacusServer failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.server == nil {
		t.Fatal("expected non-nil MCP server")
	}
	if srv.repo == nil {
		t.Fatal("expected non-nil repo")
	}
	if srv.actions == nil {
		t.Fatal("expected non-nil actions")
	}
	if srv.matcher == nil {
		t.Fatal("expected non-nil matcher")
	}
}

func TestQueryRoutesHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// Test listing all routes
	input := QueryNodesInput{Limit: 50}
	result, _, err := srv.queryRoutesHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("queryRoutes failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	// Parse the JSON response
	text := result.Content[0].(*gomcp.TextContent).Text
	var nodes []db.GraphNode
	if err := json.Unmarshal([]byte(text), &nodes); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(nodes))
	}
}

func TestQueryRoutesHandlerWithSearch(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// Test search for routes containing "users"
	input := QueryNodesInput{Query: "users", Limit: 50}
	result, _, err := srv.queryRoutesHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("queryRoutes with search failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	// FTS5 search should return results containing "users"
	if text == "[]" {
		t.Error("expected non-empty search results for 'users'")
	}
}

func TestQueryEntitiesHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	input := QueryNodesInput{Limit: 50}
	result, _, err := srv.queryEntitiesHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("queryEntities failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var nodes []db.GraphNode
	if err := json.Unmarshal([]byte(text), &nodes); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 entity, got %d", len(nodes))
	}
	if nodes[0].Name != "User" {
		t.Errorf("expected entity name 'User', got %q", nodes[0].Name)
	}
}

func TestQueryPagesHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	input := QueryNodesInput{Limit: 50}
	result, _, err := srv.queryPagesHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("queryPages failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var nodes []db.GraphNode
	if err := json.Unmarshal([]byte(text), &nodes); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 page, got %d", len(nodes))
	}
}

func TestCreateActionHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	input := CreateActionInput{
		Name:            "Login User",
		Label:           "User logs into the system",
		GherkinPatterns: []string{"the user logs in with {string} and {string}"},
		RouteRefs:       []string{"route:post-users"},
		EntityRefs:      []string{"entity:user"},
	}
	result, _, err := srv.createActionHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("createAction failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var actionResult graph.ActionResult
	if err := json.Unmarshal([]byte(text), &actionResult); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if actionResult.Action == nil {
		t.Fatal("expected non-nil action in result")
	}
	if actionResult.Action.Name != "Login User" {
		t.Errorf("expected action name 'Login User', got %q", actionResult.Action.Name)
	}
}

func TestUpdateActionHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// First create an action
	createInput := CreateActionInput{
		Name:  "Test Action",
		Label: "Original label",
	}
	_, _, err := srv.createActionHandler(ctx, nil, createInput)
	if err != nil {
		t.Fatalf("createAction failed: %v", err)
	}

	// Now update it
	newLabel := "Updated label"
	updateInput := UpdateActionInput{
		ID:    "action:test-action",
		Label: &newLabel,
	}
	result, _, err := srv.updateActionHandler(ctx, nil, updateInput)
	if err != nil {
		t.Fatalf("updateAction failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var actionResult graph.ActionResult
	if err := json.Unmarshal([]byte(text), &actionResult); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if actionResult.Action.Label != "Updated label" {
		t.Errorf("expected label 'Updated label', got %q", actionResult.Action.Label)
	}
}

func TestQueryActionsHandler(t *testing.T) {
	srv := setupTestServer(t)

	ctx := context.Background()

	// Create some actions first
	for _, name := range []string{"Action One", "Action Two"} {
		input := CreateActionInput{Name: name, Label: name + " label"}
		if _, _, err := srv.createActionHandler(ctx, nil, input); err != nil {
			t.Fatalf("createAction %q failed: %v", name, err)
		}
	}

	input := QueryActionsInput{Limit: 50}
	result, _, err := srv.queryActionsHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("queryActions failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var actions []graph.ActionNode
	if err := json.Unmarshal([]byte(text), &actions); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}

func TestMatchStepHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// Create an action with a gherkin pattern
	createInput := CreateActionInput{
		Name:            "View Users",
		Label:           "View user list",
		GherkinPatterns: []string{"I view the user list"},
		RouteRefs:       []string{"route:get-users"},
	}
	if _, _, err := srv.createActionHandler(ctx, nil, createInput); err != nil {
		t.Fatalf("createAction failed: %v", err)
	}

	// Match against the pattern
	input := MatchStepInput{StepText: "I view the user list"}
	result, _, err := srv.matchStepHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("matchStep failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var matchResult match.MatchResult
	if err := json.Unmarshal([]byte(text), &matchResult); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if matchResult.Tier != "exact" {
		t.Errorf("expected tier 'exact', got %q", matchResult.Tier)
	}
}

func TestMatchScenarioHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// Create an action
	createInput := CreateActionInput{
		Name:            "Check Dashboard",
		Label:           "Check dashboard page",
		GherkinPatterns: []string{"I check the dashboard"},
	}
	if _, _, err := srv.createActionHandler(ctx, nil, createInput); err != nil {
		t.Fatalf("createAction failed: %v", err)
	}

	input := MatchScenarioInput{
		Steps: []StepInput{
			{Keyword: "When", Text: "I check the dashboard"},
			{Keyword: "Then", Text: "I should see something unknown"},
		},
	}
	result, _, err := srv.matchScenarioHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("matchScenario failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var results []match.MatchResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 match results, got %d", len(results))
	}
}

func TestGraphContextHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// Create action with edges to test connectivity
	createInput := CreateActionInput{
		Name:       "Browse Users",
		Label:      "Browse user list",
		RouteRefs:  []string{"route:get-users"},
		EntityRefs: []string{"entity:user"},
	}
	if _, _, err := srv.createActionHandler(ctx, nil, createInput); err != nil {
		t.Fatalf("createAction failed: %v", err)
	}

	input := GraphContextInput{NodeID: "route:get-users", MaxDepth: 2}
	result, _, err := srv.graphContextHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("graphContext failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var subgraph graph.SubGraph
	if err := json.Unmarshal([]byte(text), &subgraph); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Should have at least the route node and the action node
	if len(subgraph.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes in subgraph, got %d", len(subgraph.Nodes))
	}
}

func TestStatsHandler(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	input := StatsInput{}
	result, _, err := srv.statsHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var stats map[string]int
	if err := json.Unmarshal([]byte(text), &stats); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if stats["route"] != 2 {
		t.Errorf("expected 2 routes, got %d", stats["route"])
	}
	if stats["entity"] != 1 {
		t.Errorf("expected 1 entity, got %d", stats["entity"])
	}
	if stats["page"] != 1 {
		t.Errorf("expected 1 page, got %d", stats["page"])
	}
	if stats["action"] != 0 {
		t.Errorf("expected 0 actions, got %d", stats["action"])
	}
}

func TestDefaultLimit(t *testing.T) {
	srv := setupTestServer(t)
	seedNodes(t, srv.repo)

	ctx := context.Background()

	// When limit is 0, should default to 50
	input := QueryNodesInput{Limit: 0}
	result, _, err := srv.queryRoutesHandler(ctx, nil, input)
	if err != nil {
		t.Fatalf("queryRoutes with default limit failed: %v", err)
	}

	text := result.Content[0].(*gomcp.TextContent).Text
	var nodes []db.GraphNode
	if err := json.Unmarshal([]byte(text), &nodes); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Should still return all routes (2) since default limit is 50
	if len(nodes) != 2 {
		t.Errorf("expected 2 routes with default limit, got %d", len(nodes))
	}
}
