package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
)

// setupTestDBWithNodes creates a DB seeded with route, entity, and page nodes
// in addition to the action node from setupTestDB.
func setupTestDBWithNodes(t *testing.T) string {
	t.Helper()
	dir := setupTestDB(t) // reuse existing helper
	dbFilePath := filepath.Join(dir, ".abacus", "abacus.db")

	database, err := db.OpenDB(dbFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)

	// Seed route nodes
	if err := repo.InsertNode(&db.GraphNode{
		ID:         "route:GET-/api/users",
		Kind:       db.NodeRoute,
		Name:       "GET /api/users",
		Label:      "List users",
		Properties: map[string]any{"method": "GET", "path": "/api/users"},
		Source:     db.SourceScan,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertNode(&db.GraphNode{
		ID:         "route:POST-/api/users",
		Kind:       db.NodeRoute,
		Name:       "POST /api/users",
		Label:      "Create user",
		Properties: map[string]any{"method": "POST", "path": "/api/users"},
		Source:     db.SourceScan,
	}); err != nil {
		t.Fatal(err)
	}

	// Seed an entity node
	if err := repo.InsertNode(&db.GraphNode{
		ID:         "entity:User",
		Kind:       db.NodeEntity,
		Name:       "User",
		Label:      "User model",
		Properties: map[string]any{"fields": []any{"id", "name", "email"}},
		Source:     db.SourceScan,
	}); err != nil {
		t.Fatal(err)
	}

	// Seed a page node
	if err := repo.InsertNode(&db.GraphNode{
		ID:         "page:/dashboard",
		Kind:       db.NodePage,
		Name:       "Dashboard",
		Label:      "Main dashboard",
		Properties: map[string]any{"path": "/dashboard"},
		Source:     db.SourceScan,
	}); err != nil {
		t.Fatal(err)
	}

	return dir
}

// --- Routes tests ---

func TestRoutesListEmpty(t *testing.T) {
	dir := setupTestDB(t) // no extra nodes seeded
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"routes", "--db", dbFile, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("routes --json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(results) != 0 {
		t.Errorf("expected 0 routes in empty DB, got %d", len(results))
	}
}

func TestRoutesListWithData(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"routes", "--db", dbFile, "--json=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("routes failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "GET /api/users") {
		t.Errorf("expected 'GET /api/users' in output, got: %s", output)
	}
	if !strings.Contains(output, "POST /api/users") {
		t.Errorf("expected 'POST /api/users' in output, got: %s", output)
	}
}

func TestRoutesListJSON(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"routes", "--db", dbFile, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("routes --json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(results) != 2 {
		t.Errorf("expected 2 routes, got %d", len(results))
	}
}

func TestRoutesMatchQuery(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"routes", "--db", dbFile, "--json", "--match", "users"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("routes --match failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(results) == 0 {
		t.Error("expected at least one route matching 'users'")
	}
}

// --- Entities tests ---

func TestEntitiesListWithData(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"entities", "--db", dbFile, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("entities --json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(results) != 1 {
		t.Errorf("expected 1 entity, got %d", len(results))
	}

	if results[0]["name"] != "User" {
		t.Errorf("expected entity name 'User', got %v", results[0]["name"])
	}
}

// --- Pages tests ---

func TestPagesListWithData(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"pages", "--db", dbFile, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pages --json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(results) != 1 {
		t.Errorf("expected 1 page, got %d", len(results))
	}

	if results[0]["name"] != "Dashboard" {
		t.Errorf("expected page name 'Dashboard', got %v", results[0]["name"])
	}
}

// --- Actions tests ---

func TestActionsListJSON(t *testing.T) {
	dir := setupTestDB(t) // has "register user" action
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"actions", "--db", dbFile, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("actions --json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(results) < 1 {
		t.Fatal("expected at least 1 action (register user)")
	}

	// Verify the seeded action is present (Go JSON uses uppercase field names: "Node", "Name")
	found := false
	for _, a := range results {
		// Try uppercase (Go struct field name) then lowercase
		node, ok := a["Node"].(map[string]interface{})
		if !ok {
			node, ok = a["node"].(map[string]interface{})
		}
		if !ok {
			continue
		}
		if node["name"] == "register user" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'register user' action in output, got: %s", buf.String())
	}
}

func TestActionsCreateCommand(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Create a new action via CLI
	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"actions", "create", "login user", "--db", dbFile, "--json",
		"--label", "User login action",
		"--gherkin", "I log in as a user",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("actions create failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	// Go JSON uses uppercase field names from struct tags or field names
	action, ok := result["Action"].(map[string]interface{})
	if !ok {
		action, ok = result["action"].(map[string]interface{})
	}
	if !ok {
		t.Fatalf("expected 'action' or 'Action' in result, got: %v", result)
	}
	if action["name"] != "login user" {
		t.Errorf("expected action name 'login user', got %v", action["name"])
	}

	// Now list actions and verify it appears
	buf.Reset()
	cmd.SetArgs([]string{"actions", "--db", dbFile, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("actions list after create failed: %v", err)
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &list); err != nil {
		t.Fatalf("list output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	found := false
	for _, a := range list {
		node, ok := a["Node"].(map[string]interface{})
		if !ok {
			node, ok = a["node"].(map[string]interface{})
		}
		if !ok {
			continue
		}
		if node["name"] == "login user" {
			found = true
			break
		}
	}
	if !found {
		t.Error("created action 'login user' not found in list")
	}
}

// --- Graph tests ---

func TestGraphCommand(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Create an edge between route and entity
	database, err := db.OpenDB(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	repo := graph.NewGraphRepository(database)
	if err := repo.InsertEdge(&db.GraphEdge{
		ID:    "edge:route-entity-1",
		SrcID: "route:GET-/api/users",
		DstID: "entity:User",
		Kind:  db.EdgeTouchesEntity,
	}); err != nil {
		t.Fatal(err)
	}
	database.Close()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"graph", "--db", dbFile, "--json=false", "route:GET-/api/users"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "route:GET-/api/users") {
		t.Errorf("expected source node in output, got: %s", output)
	}
	if !strings.Contains(output, "entity:User") {
		t.Errorf("expected connected entity in output, got: %s", output)
	}
}

func TestGraphCommandJSON(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	// Create an edge between route and entity
	database, err := db.OpenDB(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	repo := graph.NewGraphRepository(database)
	if err := repo.InsertEdge(&db.GraphEdge{
		ID:    "edge:route-entity-2",
		SrcID: "route:POST-/api/users",
		DstID: "entity:User",
		Kind:  db.EdgeTouchesEntity,
	}); err != nil {
		t.Fatal(err)
	}
	database.Close()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"graph", "--db", dbFile, "--json", "route:POST-/api/users"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	nodes, ok := result["Nodes"]
	if !ok {
		// Try lowercase
		nodes, ok = result["nodes"]
	}
	if !ok {
		t.Fatalf("expected 'nodes' or 'Nodes' in JSON output, got keys: %v", result)
	}
	nodeList, ok := nodes.([]interface{})
	if !ok {
		t.Fatalf("expected nodes to be an array, got: %T", nodes)
	}
	if len(nodeList) < 2 {
		t.Errorf("expected at least 2 nodes (source + connected), got %d", len(nodeList))
	}

	edges, ok := result["Edges"]
	if !ok {
		edges, ok = result["edges"]
	}
	if !ok {
		t.Fatalf("expected 'edges' or 'Edges' in JSON output, got keys: %v", result)
	}
	edgeList, ok := edges.([]interface{})
	if !ok {
		t.Fatalf("expected edges to be an array, got: %T", edges)
	}
	if len(edgeList) < 1 {
		t.Errorf("expected at least 1 edge, got %d", len(edgeList))
	}
}

func TestGraphCommandEmptyGraph(t *testing.T) {
	dir := setupTestDB(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"graph", "--db", dbFile, "--json", "nonexistent:node"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("graph command for nonexistent node failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	// Should have empty or null nodes/edges
	nodes := result["Nodes"]
	if nodes == nil {
		nodes = result["nodes"]
	}
	if nodeList, ok := nodes.([]interface{}); ok && len(nodeList) > 0 {
		t.Errorf("expected empty nodes for nonexistent node, got %d", len(nodeList))
	}
}

// --- Stats tests ---

func TestStatsCommand(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// Use --json=false explicitly since persistent flags leak between tests
	cmd.SetArgs([]string{"stats", "--db", dbFile, "--json=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("stats failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "route") {
		t.Errorf("expected 'route' kind in stats output, got: %s", output)
	}
	if !strings.Contains(output, "Total") {
		t.Errorf("expected 'Total' in stats output, got: %s", output)
	}
}

func TestStatsCommandJSON(t *testing.T) {
	dir := setupTestDBWithNodes(t)
	dbFile := filepath.Join(dir, ".abacus", "abacus.db")

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"stats", "--db", dbFile, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("stats --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	// Should have kind counts
	if _, ok := result["route"]; !ok {
		t.Error("JSON stats missing 'route' count")
	}
	if _, ok := result["entity"]; !ok {
		t.Error("JSON stats missing 'entity' count")
	}
	if _, ok := result["total"]; !ok {
		t.Error("JSON stats missing 'total' count")
	}

	// Verify counts are correct
	routeCount, ok := result["route"].(float64)
	if !ok || routeCount != 2 {
		t.Errorf("expected route count 2, got %v", result["route"])
	}
	entityCount, ok := result["entity"].(float64)
	if !ok || entityCount != 1 {
		t.Errorf("expected entity count 1, got %v", result["entity"])
	}
	total, ok := result["total"].(float64)
	if !ok || total < 4 {
		// 2 routes + 1 entity + 1 page + 1 action = 5
		t.Errorf("expected total >= 4, got %v", result["total"])
	}
}

func TestStatsEmptyGraph(t *testing.T) {
	// Create a DB with no nodes at all (bypass setupTestDB which seeds an action)
	dir := t.TempDir()
	abacusDir := filepath.Join(dir, ".abacus")
	if err := os.MkdirAll(abacusDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbFilePath := filepath.Join(abacusDir, "abacus.db")
	database, err := db.OpenDB(dbFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatal(err)
	}
	database.Close()

	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"stats", "--db", dbFilePath, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("stats --json on empty graph failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	total, ok := result["total"].(float64)
	if !ok || total != 0 {
		t.Errorf("expected total 0 for empty graph, got %v", result["total"])
	}
}
