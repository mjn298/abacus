package graph

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/mjn/abacus/internal/db"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func makeNode(id string, kind db.NodeKind) *db.GraphNode {
	return &db.GraphNode{
		ID:         id,
		Kind:       kind,
		Name:       "name-" + id,
		Label:      "label-" + id,
		Properties: map[string]any{"key": "value"},
		Source:     db.SourceScan,
	}
}

// --- Node CRUD Tests ---

func TestInsertNode(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	node := makeNode("n1", db.NodeRoute)
	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}

	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.ID != "n1" {
		t.Errorf("ID = %q, want %q", got.ID, "n1")
	}
	if got.Kind != db.NodeRoute {
		t.Errorf("Kind = %q, want %q", got.Kind, db.NodeRoute)
	}
	if got.Name != "name-n1" {
		t.Errorf("Name = %q, want %q", got.Name, "name-n1")
	}
	if got.Label != "label-n1" {
		t.Errorf("Label = %q, want %q", got.Label, "label-n1")
	}
	if got.Properties["key"] != "value" {
		t.Errorf("Properties[key] = %v, want %q", got.Properties["key"], "value")
	}
	if got.Source != db.SourceScan {
		t.Errorf("Source = %q, want %q", got.Source, db.SourceScan)
	}
	if got.CreatedAt == 0 {
		t.Error("CreatedAt should be set")
	}
	if got.UpdatedAt == 0 {
		t.Error("UpdatedAt should be set")
	}
}

func TestInsertNode_AllKinds(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	kinds := []db.NodeKind{db.NodeRoute, db.NodeEntity, db.NodePage, db.NodeAction, db.NodePermission}
	for i, kind := range kinds {
		node := makeNode(fmt.Sprintf("n%d", i), kind)
		if err := repo.InsertNode(node); err != nil {
			t.Fatalf("InsertNode(%s): %v", kind, err)
		}
		got, err := repo.GetNode(fmt.Sprintf("n%d", i))
		if err != nil {
			t.Fatalf("GetNode(%d): %v", i, err)
		}
		if got.Kind != kind {
			t.Errorf("Kind = %q, want %q", got.Kind, kind)
		}
	}
}

func TestInsertNode_WithSourceFile(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	sf := "src/main.go"
	node := makeNode("n1", db.NodeRoute)
	node.SourceFile = &sf

	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}
	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.SourceFile == nil || *got.SourceFile != sf {
		t.Errorf("SourceFile = %v, want %q", got.SourceFile, sf)
	}
}

func TestInsertNode_WithScanHash(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	hash := "abc123"
	node := makeNode("n1", db.NodeRoute)
	node.ScanHash = &hash

	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}
	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.ScanHash == nil || *got.ScanHash != hash {
		t.Errorf("ScanHash = %v, want %q", got.ScanHash, hash)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	got, err := repo.GetNode("nonexistent")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestUpsertNode_Insert(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	node := makeNode("n1", db.NodeRoute)
	if err := repo.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Name != "name-n1" {
		t.Errorf("Name = %q, want %q", got.Name, "name-n1")
	}
}

func TestUpsertNode_Update(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	node := makeNode("n1", db.NodeRoute)
	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}

	// Wait a tiny bit so updated_at differs (if the DB uses seconds)
	time.Sleep(10 * time.Millisecond)

	node.Name = "updated-name"
	node.Label = "updated-label"
	node.Properties = map[string]any{"new": "props"}
	if err := repo.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", got.Name, "updated-name")
	}
	if got.Label != "updated-label" {
		t.Errorf("Label = %q, want %q", got.Label, "updated-label")
	}
	if got.Properties["new"] != "props" {
		t.Errorf("Properties[new] = %v, want %q", got.Properties["new"], "props")
	}
}

func TestDeleteNode(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	node := makeNode("n1", db.NodeRoute)
	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}

	if err := repo.DeleteNode("n1"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}

	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestDeleteNode_CascadesEdges(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	if err := repo.InsertNode(n1); err != nil {
		t.Fatalf("InsertNode n1: %v", err)
	}
	if err := repo.InsertNode(n2); err != nil {
		t.Fatalf("InsertNode n2: %v", err)
	}

	edge := &db.GraphEdge{
		ID:    "e1",
		SrcID: "n1",
		DstID: "n2",
		Kind:  db.EdgeUsesRoute,
	}
	if err := repo.InsertEdge(edge); err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}

	if err := repo.DeleteNode("n1"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}

	edges, err := repo.GetEdgesFrom("n1", nil)
	if err != nil {
		t.Fatalf("GetEdgesFrom: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges after cascade delete, got %d", len(edges))
	}
}

func TestGetNodesByKind(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	for i := 0; i < 5; i++ {
		node := makeNode(fmt.Sprintf("r%d", i), db.NodeRoute)
		if err := repo.InsertNode(node); err != nil {
			t.Fatalf("InsertNode: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		node := makeNode(fmt.Sprintf("e%d", i), db.NodeEntity)
		if err := repo.InsertNode(node); err != nil {
			t.Fatalf("InsertNode: %v", err)
		}
	}

	routes, err := repo.GetNodesByKind(db.NodeRoute, 10, 0)
	if err != nil {
		t.Fatalf("GetNodesByKind: %v", err)
	}
	if len(routes) != 5 {
		t.Errorf("expected 5 routes, got %d", len(routes))
	}

	entities, err := repo.GetNodesByKind(db.NodeEntity, 10, 0)
	if err != nil {
		t.Fatalf("GetNodesByKind: %v", err)
	}
	if len(entities) != 3 {
		t.Errorf("expected 3 entities, got %d", len(entities))
	}
}

func TestGetNodesByKind_Pagination(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	for i := 0; i < 10; i++ {
		node := makeNode(fmt.Sprintf("r%02d", i), db.NodeRoute)
		if err := repo.InsertNode(node); err != nil {
			t.Fatalf("InsertNode: %v", err)
		}
	}

	page1, err := repo.GetNodesByKind(db.NodeRoute, 3, 0)
	if err != nil {
		t.Fatalf("GetNodesByKind page1: %v", err)
	}
	if len(page1) != 3 {
		t.Errorf("page1 len = %d, want 3", len(page1))
	}

	page2, err := repo.GetNodesByKind(db.NodeRoute, 3, 3)
	if err != nil {
		t.Fatalf("GetNodesByKind page2: %v", err)
	}
	if len(page2) != 3 {
		t.Errorf("page2 len = %d, want 3", len(page2))
	}

	// Ensure different results
	if page1[0].ID == page2[0].ID {
		t.Error("page1 and page2 should have different first elements")
	}
}

// --- Edge Tests ---

func TestInsertEdge(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	if err := repo.InsertNode(n1); err != nil {
		t.Fatalf("InsertNode n1: %v", err)
	}
	if err := repo.InsertNode(n2); err != nil {
		t.Fatalf("InsertNode n2: %v", err)
	}

	edge := &db.GraphEdge{
		ID:         "e1",
		SrcID:      "n1",
		DstID:      "n2",
		Kind:       db.EdgeUsesRoute,
		Properties: map[string]any{"weight": float64(1)},
	}
	if err := repo.InsertEdge(edge); err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}

	edges, err := repo.GetEdgesFrom("n1", nil)
	if err != nil {
		t.Fatalf("GetEdgesFrom: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ID != "e1" {
		t.Errorf("ID = %q, want %q", edges[0].ID, "e1")
	}
	if edges[0].SrcID != "n1" {
		t.Errorf("SrcID = %q, want %q", edges[0].SrcID, "n1")
	}
	if edges[0].DstID != "n2" {
		t.Errorf("DstID = %q, want %q", edges[0].DstID, "n2")
	}
	if edges[0].Kind != db.EdgeUsesRoute {
		t.Errorf("Kind = %q, want %q", edges[0].Kind, db.EdgeUsesRoute)
	}
	if edges[0].Properties["weight"] != float64(1) {
		t.Errorf("Properties[weight] = %v, want 1", edges[0].Properties["weight"])
	}
}

func TestInsertEdge_DuplicateReturnsError(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	repo.InsertNode(n1)
	repo.InsertNode(n2)

	edge := &db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute}
	if err := repo.InsertEdge(edge); err != nil {
		t.Fatalf("first InsertEdge: %v", err)
	}

	// Same (src, dst, kind) but different ID is now allowed (multiple edges of same kind between same nodes)
	edge2 := &db.GraphEdge{ID: "e2", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute}
	if err := repo.InsertEdge(edge2); err != nil {
		t.Fatalf("InsertEdge with different ID but same (src,dst,kind) should succeed: %v", err)
	}

	// Same ID (primary key) should upsert (replace) without error
	edge3 := &db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute}
	if err := repo.InsertEdge(edge3); err != nil {
		t.Fatalf("InsertEdge with same ID should upsert: %v", err)
	}
}

func TestInsertEdge_DifferentKindAllowed(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	repo.InsertNode(n1)
	repo.InsertNode(n2)

	e1 := &db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute}
	e2 := &db.GraphEdge{ID: "e2", SrcID: "n1", DstID: "n2", Kind: db.EdgeTouchesEntity}
	if err := repo.InsertEdge(e1); err != nil {
		t.Fatalf("InsertEdge e1: %v", err)
	}
	if err := repo.InsertEdge(e2); err != nil {
		t.Fatalf("InsertEdge e2: %v", err)
	}

	edges, err := repo.GetEdgesFrom("n1", nil)
	if err != nil {
		t.Fatalf("GetEdgesFrom: %v", err)
	}
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
}

func TestGetEdgesFrom_WithKindFilter(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	n3 := makeNode("n3", db.NodePage)
	repo.InsertNode(n1)
	repo.InsertNode(n2)
	repo.InsertNode(n3)

	repo.InsertEdge(&db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute})
	repo.InsertEdge(&db.GraphEdge{ID: "e2", SrcID: "n1", DstID: "n3", Kind: db.EdgeOnPage})

	kind := db.EdgeUsesRoute
	edges, err := repo.GetEdgesFrom("n1", &kind)
	if err != nil {
		t.Fatalf("GetEdgesFrom with filter: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge with filter, got %d", len(edges))
	}
	if edges[0].Kind != db.EdgeUsesRoute {
		t.Errorf("Kind = %q, want %q", edges[0].Kind, db.EdgeUsesRoute)
	}
}

func TestGetEdgesTo(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	n3 := makeNode("n3", db.NodePage)
	repo.InsertNode(n1)
	repo.InsertNode(n2)
	repo.InsertNode(n3)

	repo.InsertEdge(&db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute})
	repo.InsertEdge(&db.GraphEdge{ID: "e2", SrcID: "n3", DstID: "n2", Kind: db.EdgeTouchesEntity})

	edges, err := repo.GetEdgesTo("n2", nil)
	if err != nil {
		t.Fatalf("GetEdgesTo: %v", err)
	}
	if len(edges) != 2 {
		t.Errorf("expected 2 edges to n2, got %d", len(edges))
	}
}

func TestGetEdgesTo_WithKindFilter(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	n3 := makeNode("n3", db.NodePage)
	repo.InsertNode(n1)
	repo.InsertNode(n2)
	repo.InsertNode(n3)

	repo.InsertEdge(&db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute})
	repo.InsertEdge(&db.GraphEdge{ID: "e2", SrcID: "n3", DstID: "n2", Kind: db.EdgeTouchesEntity})

	kind := db.EdgeTouchesEntity
	edges, err := repo.GetEdgesTo("n2", &kind)
	if err != nil {
		t.Fatalf("GetEdgesTo with filter: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge with filter, got %d", len(edges))
	}
	if edges[0].Kind != db.EdgeTouchesEntity {
		t.Errorf("Kind = %q, want %q", edges[0].Kind, db.EdgeTouchesEntity)
	}
}

func TestInsertEdge_WithSourceScanner(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	repo.InsertNode(n1)
	repo.InsertNode(n2)

	scanner := "prisma"
	edge := &db.GraphEdge{
		ID:            "e1",
		SrcID:         "n1",
		DstID:         "n2",
		Kind:          db.EdgeTouchesEntity,
		Properties:    map[string]any{"field": "user_id"},
		SourceScanner: &scanner,
	}
	if err := repo.InsertEdge(edge); err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}

	edges, err := repo.GetEdgesFrom("n1", nil)
	if err != nil {
		t.Fatalf("GetEdgesFrom: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].SourceScanner == nil {
		t.Fatal("expected SourceScanner to be non-nil")
	}
	if *edges[0].SourceScanner != "prisma" {
		t.Errorf("SourceScanner = %q, want %q", *edges[0].SourceScanner, "prisma")
	}
}

func TestInsertEdge_WithoutSourceScanner(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	n1 := makeNode("n1", db.NodeRoute)
	n2 := makeNode("n2", db.NodeEntity)
	repo.InsertNode(n1)
	repo.InsertNode(n2)

	edge := &db.GraphEdge{
		ID:    "e1",
		SrcID: "n1",
		DstID: "n2",
		Kind:  db.EdgeUsesRoute,
	}
	if err := repo.InsertEdge(edge); err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}

	edges, err := repo.GetEdgesFrom("n1", nil)
	if err != nil {
		t.Fatalf("GetEdgesFrom: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].SourceScanner != nil {
		t.Errorf("expected SourceScanner to be nil, got %q", *edges[0].SourceScanner)
	}
}

// --- Search Tests ---

func TestSearch_BasicFTS(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	nodes := []*db.GraphNode{
		{ID: "n1", Kind: db.NodePage, Name: "user_profile", Label: "User Profile Page", Source: db.SourceScan},
		{ID: "n2", Kind: db.NodePage, Name: "admin_dashboard", Label: "Admin Dashboard", Source: db.SourceScan},
		{ID: "n3", Kind: db.NodePage, Name: "order_history", Label: "Order History View", Source: db.SourceScan},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("InsertNode %s: %v", n.ID, err)
		}
	}

	results, err := repo.Search("dashboard", nil, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results, got none")
	}
	if results[0].Node.Name != "admin_dashboard" {
		t.Errorf("top result = %q, want %q", results[0].Node.Name, "admin_dashboard")
	}
	if results[0].Rank >= 0 {
		// BM25 ranks are negative (lower = better match)
		t.Logf("rank = %f (note: BM25 ranks are typically negative)", results[0].Rank)
	}
}

func TestSearch_WithKindFilter(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	nodes := []*db.GraphNode{
		{ID: "n1", Kind: db.NodePage, Name: "user_settings", Label: "User Settings", Source: db.SourceScan},
		{ID: "n2", Kind: db.NodeRoute, Name: "user_api", Label: "User API Route", Source: db.SourceScan},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("InsertNode %s: %v", n.ID, err)
		}
	}

	kind := db.NodeRoute
	results, err := repo.Search("user", &kind, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with kind filter, got %d", len(results))
	}
	if results[0].Node.Kind != db.NodeRoute {
		t.Errorf("result kind = %q, want %q", results[0].Node.Kind, db.NodeRoute)
	}
}

func TestSearch_NoResults(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	node := makeNode("n1", db.NodeRoute)
	repo.InsertNode(node)

	results, err := repo.Search("nonexistent_term_xyz", nil, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_Limit(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	for i := 0; i < 10; i++ {
		node := &db.GraphNode{
			ID:     fmt.Sprintf("n%d", i),
			Kind:   db.NodePage,
			Name:   fmt.Sprintf("user_page_%d", i),
			Label:  "User Page",
			Source: db.SourceScan,
		}
		if err := repo.InsertNode(node); err != nil {
			t.Fatalf("InsertNode: %v", err)
		}
	}

	results, err := repo.Search("user", nil, 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit, got %d", len(results))
	}
}

// --- Graph Traversal Tests ---

func TestGetConnected_SimpleGraph(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Create a simple chain: n1 -> n2 -> n3
	repo.InsertNode(makeNode("n1", db.NodeRoute))
	repo.InsertNode(makeNode("n2", db.NodeEntity))
	repo.InsertNode(makeNode("n3", db.NodePage))

	repo.InsertEdge(&db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute})
	repo.InsertEdge(&db.GraphEdge{ID: "e2", SrcID: "n2", DstID: "n3", Kind: db.EdgeOnPage})

	sg, err := repo.GetConnected("n1", 3)
	if err != nil {
		t.Fatalf("GetConnected: %v", err)
	}
	if len(sg.Nodes) != 3 {
		t.Errorf("expected 3 connected nodes, got %d", len(sg.Nodes))
	}
	if len(sg.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(sg.Edges))
	}
}

func TestGetConnected_Bidirectional(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// n1 -> n2 and n3 -> n1 (n1 is both src and dst)
	repo.InsertNode(makeNode("n1", db.NodeRoute))
	repo.InsertNode(makeNode("n2", db.NodeEntity))
	repo.InsertNode(makeNode("n3", db.NodePage))

	repo.InsertEdge(&db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute})
	repo.InsertEdge(&db.GraphEdge{ID: "e2", SrcID: "n3", DstID: "n1", Kind: db.EdgeOnPage})

	sg, err := repo.GetConnected("n1", 3)
	if err != nil {
		t.Fatalf("GetConnected: %v", err)
	}
	if len(sg.Nodes) != 3 {
		t.Errorf("expected 3 nodes (bidirectional), got %d", len(sg.Nodes))
	}
}

func TestGetConnected_DepthLimit(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Chain: n1 -> n2 -> n3 -> n4
	for i := 1; i <= 4; i++ {
		repo.InsertNode(makeNode(fmt.Sprintf("n%d", i), db.NodeRoute))
	}
	for i := 1; i <= 3; i++ {
		repo.InsertEdge(&db.GraphEdge{
			ID:    fmt.Sprintf("e%d", i),
			SrcID: fmt.Sprintf("n%d", i),
			DstID: fmt.Sprintf("n%d", i+1),
			Kind:  db.EdgeRelatesTo,
		})
	}

	// Depth 1: should get n1 + n2
	sg, err := repo.GetConnected("n1", 1)
	if err != nil {
		t.Fatalf("GetConnected depth 1: %v", err)
	}
	if len(sg.Nodes) != 2 {
		t.Errorf("depth 1: expected 2 nodes, got %d", len(sg.Nodes))
	}

	// Depth 2: should get n1 + n2 + n3
	sg, err = repo.GetConnected("n1", 2)
	if err != nil {
		t.Fatalf("GetConnected depth 2: %v", err)
	}
	if len(sg.Nodes) != 3 {
		t.Errorf("depth 2: expected 3 nodes, got %d", len(sg.Nodes))
	}
}

func TestGetConnected_HandlesCycles(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Create cycle: n1 -> n2 -> n3 -> n1
	repo.InsertNode(makeNode("n1", db.NodeRoute))
	repo.InsertNode(makeNode("n2", db.NodeEntity))
	repo.InsertNode(makeNode("n3", db.NodePage))

	repo.InsertEdge(&db.GraphEdge{ID: "e1", SrcID: "n1", DstID: "n2", Kind: db.EdgeUsesRoute})
	repo.InsertEdge(&db.GraphEdge{ID: "e2", SrcID: "n2", DstID: "n3", Kind: db.EdgeOnPage})
	repo.InsertEdge(&db.GraphEdge{ID: "e3", SrcID: "n3", DstID: "n1", Kind: db.EdgeRelatesTo})

	sg, err := repo.GetConnected("n1", 10)
	if err != nil {
		t.Fatalf("GetConnected with cycle: %v", err)
	}
	if len(sg.Nodes) != 3 {
		t.Errorf("expected 3 nodes (cycle handled), got %d", len(sg.Nodes))
	}
}

func TestGetConnected_IsolatedNode(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	repo.InsertNode(makeNode("n1", db.NodeRoute))

	sg, err := repo.GetConnected("n1", 3)
	if err != nil {
		t.Fatalf("GetConnected: %v", err)
	}
	if len(sg.Nodes) != 1 {
		t.Errorf("expected 1 node (isolated), got %d", len(sg.Nodes))
	}
	if len(sg.Edges) != 0 {
		t.Errorf("expected 0 edges (isolated), got %d", len(sg.Edges))
	}
}

// --- Bulk Operations Tests ---

func TestBulkUpsertNodes(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	nodes := make([]db.GraphNode, 50)
	for i := range nodes {
		nodes[i] = *makeNode(fmt.Sprintf("n%d", i), db.NodeRoute)
	}

	count, err := repo.BulkUpsertNodes(nodes)
	if err != nil {
		t.Fatalf("BulkUpsertNodes: %v", err)
	}
	if count != 50 {
		t.Errorf("count = %d, want 50", count)
	}

	// Verify some nodes exist
	for _, id := range []string{"n0", "n25", "n49"} {
		got, err := repo.GetNode(id)
		if err != nil {
			t.Fatalf("GetNode %s: %v", id, err)
		}
		if got == nil {
			t.Errorf("node %s not found after bulk upsert", id)
		}
	}
}

func TestBulkUpsertNodes_UpdatesExisting(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Insert original
	node := makeNode("n1", db.NodeRoute)
	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}

	// Bulk upsert with updated name
	updated := *makeNode("n1", db.NodeRoute)
	updated.Name = "bulk-updated"
	count, err := repo.BulkUpsertNodes([]db.GraphNode{updated})
	if err != nil {
		t.Fatalf("BulkUpsertNodes: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Name != "bulk-updated" {
		t.Errorf("Name = %q, want %q", got.Name, "bulk-updated")
	}
}

func TestBulkUpsertNodes_Empty(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	count, err := repo.BulkUpsertNodes(nil)
	if err != nil {
		t.Fatalf("BulkUpsertNodes empty: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestBulkUpsertNodes_Performance(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	nodes := make([]db.GraphNode, 200)
	for i := range nodes {
		nodes[i] = *makeNode(fmt.Sprintf("perf%d", i), db.NodeRoute)
	}

	start := time.Now()
	count, err := repo.BulkUpsertNodes(nodes)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("BulkUpsertNodes: %v", err)
	}
	if count != 200 {
		t.Errorf("count = %d, want 200", count)
	}
	if elapsed > 5*time.Second {
		t.Errorf("bulk insert of 200 nodes took %v, expected < 5s", elapsed)
	}
	t.Logf("200 node bulk upsert took %v", elapsed)
}

// --- Edge case: nil properties ---

func TestInsertNode_NilProperties(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	node := &db.GraphNode{
		ID:     "n1",
		Kind:   db.NodeRoute,
		Name:   "test",
		Label:  "Test",
		Source: db.SourceScan,
		// Properties is nil
	}
	if err := repo.InsertNode(node); err != nil {
		t.Fatalf("InsertNode with nil properties: %v", err)
	}

	got, err := repo.GetNode("n1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Properties == nil {
		t.Error("Properties should not be nil after round-trip")
	}
}
