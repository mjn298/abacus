package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/match"
	"github.com/mjn/abacus/internal/scanner"
	abacusmcp "github.com/mjn/abacus/mcp"
)

// Known gaps:
// - stdio MCP transport not tested (in-process only)
// - Fixture does not cover all scanner-supported patterns (per-scanner tests do)
// - No negative testing of malformed TypeScript input

var (
	repoRoot    string
	scannerRoot string
	fixtureRoot string

	scanPhase = []string{"express", "prisma", "react-router", "orpc"}
	linkPhase = []string{"linker-ts"}
)

func TestMain(m *testing.M) {
	// 1. Check node is available, skip all if missing
	if _, err := exec.LookPath("node"); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: node not found in PATH\n")
		os.Exit(0)
	}

	// 2. Find repo root
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cannot find repo root: %v\n", err)
		os.Exit(0)
	}
	repoRoot = root
	fixtureRoot = filepath.Join(repoRoot, "testdata", "e2e-project")

	// 3. Resolve scanner paths (ABACUS_SCANNERS_PATH or relative)
	scannerRoot = os.Getenv("ABACUS_SCANNERS_PATH")
	if scannerRoot == "" {
		scannerRoot = filepath.Join(repoRoot, "scanners")
	}

	// 4. Verify dist/index.js exists for each scanner (or skip with message)
	for _, id := range scanPhase {
		if !scannerBuilt(id) {
			os.Exit(0)
		}
	}
	for _, id := range linkPhase {
		if !scannerBuilt(id) {
			os.Exit(0)
		}
	}

	os.Exit(m.Run())
}

func scannerBuilt(id string) bool {
	distPath := filepath.Join(scannerRoot, id, "dist", "index.js")
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "SKIP: scanner %s not built (missing %s)\n", id, distPath)
		return false
	}
	return true
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in parent directories")
		}
		dir = parent
	}
}

func scannerCommand(id string) string {
	return fmt.Sprintf("node %s", filepath.Join(scannerRoot, id, "dist", "index.js"))
}

// runScanPipeline runs all 5 scanners against testdata/e2e-project in two phases
// (scan-phase → ingest → link-phase with ExistingNodes), replicating cli/scan.go.
// Returns the DB path and populated repository. Database is closed via t.Cleanup.
func runScanPipeline(t *testing.T) (string, *graph.GraphRepository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "abacus.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	repo := graph.NewGraphRepository(database)
	ctx := context.Background()

	// Phase 1: Run scan-phase scanners (Express, Prisma, React Router, oRPC)
	runner := scanner.NewRunner(60 * time.Second)
	var allNodes []scanner.ScanNode
	var allEdges []scanner.ScanEdge

	for _, id := range scanPhase {
		input := scanner.ScanInput{
			Version:     1,
			ProjectRoot: fixtureRoot,
			Options:     map[string]any{},
			IgnorePaths: []string{"node_modules", "dist", "build", ".git"},
		}
		out, err := runner.RunScanner(ctx, scannerCommand(id), input, nil)
		if err != nil {
			t.Fatalf("scan-phase scanner %s failed: %v", id, err)
		}
		t.Logf("Scanner %s: %d nodes, %d edges", id, out.Stats.NodesFound, out.Stats.EdgesFound)
		allNodes = append(allNodes, out.Nodes...)
		allEdges = append(allEdges, out.Edges...)
	}

	// Ingest scan-phase nodes
	graphNodes := scanner.ToGraphNodes(allNodes)

	nodesIngested, err := repo.BulkUpsertNodes(graphNodes)
	if err != nil {
		t.Fatalf("ingest nodes: %v", err)
	}
	t.Logf("Ingested %d nodes", nodesIngested)

	// Ingest scan-phase edges
	edgesCreated := 0
	for _, se := range allEdges {
		edge := &db.GraphEdge{
			ID:         se.ID,
			SrcID:      se.SrcID,
			DstID:      se.DstID,
			Kind:       db.EdgeKind(se.Kind),
			Properties: se.Properties,
		}
		if err := repo.InsertEdge(edge); err != nil {
			t.Logf("warning: edge %s: %v", se.ID, err)
		} else {
			edgesCreated++
		}
	}
	t.Logf("Ingested %d scan-phase edges", edgesCreated)

	// Phase 2: Collect ScanNodeRefs for route+entity nodes
	var existingNodes []scanner.ScanNodeRef
	for _, sn := range allNodes {
		if sn.Kind == "route" || sn.Kind == "entity" {
			existingNodes = append(existingNodes, scanner.ScanNodeRef{
				ID:         sn.ID,
				Kind:       sn.Kind,
				Name:       sn.Name,
				SourceFile: sn.SourceFile,
			})
		}
	}

	knownNodeIDs := make(map[string]bool, len(existingNodes))
	for _, ref := range existingNodes {
		knownNodeIDs[ref.ID] = true
	}

	// Run link-phase scanners (linker-ts with ExistingNodes)
	linkRunner := scanner.NewRunner(5 * time.Minute)
	for _, id := range linkPhase {
		input := scanner.ScanInput{
			Version:       1,
			ProjectRoot:   fixtureRoot,
			Options:       map[string]any{},
			IgnorePaths:   []string{"node_modules", "dist", "build", ".git"},
			ExistingNodes: existingNodes,
		}

		out, err := linkRunner.RunScanner(ctx, scannerCommand(id), input, knownNodeIDs)
		if err != nil {
			t.Fatalf("link-phase scanner %s failed: %v", id, err)
		}

		// Delete stale edges by source scanner, then bulk upsert
		if _, err := repo.DeleteEdgesBySourceScanner(id); err != nil {
			t.Fatalf("delete stale edges for %s: %v", id, err)
		}

		linkEdges := make([]db.GraphEdge, len(out.Edges))
		scannerID := id
		for i, se := range out.Edges {
			linkEdges[i] = db.GraphEdge{
				ID:            se.ID,
				SrcID:         se.SrcID,
				DstID:         se.DstID,
				Kind:          db.EdgeKind(se.Kind),
				Properties:    se.Properties,
				SourceScanner: &scannerID,
			}
		}

		linkerEdges, err := repo.BulkUpsertEdges(linkEdges)
		if err != nil {
			t.Fatalf("ingest linker edges: %v", err)
		}
		t.Logf("Linker %s: %d edges", id, linkerEdges)
	}

	return dbPath, repo
}

// --- Test 1: TestScanPipeline ---

func TestScanPipeline(t *testing.T) {
	_, repo := runScanPipeline(t)

	t.Run("node_counts", func(t *testing.T) {
		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}
		// Express 3 + oRPC 3 - 2 ID collisions (same method+path) = 4 unique routes
		if len(routes) < 4 {
			t.Errorf("expected ≥4 routes, got %d", len(routes))
		}
		t.Logf("Routes: %d", len(routes))

		entities, err := repo.GetNodesByKind(db.NodeEntity, 1000, 0)
		if err != nil {
			t.Fatalf("query entities: %v", err)
		}
		if len(entities) < 2 {
			t.Errorf("expected ≥2 entities, got %d", len(entities))
		}
		t.Logf("Entities: %d", len(entities))

		pages, err := repo.GetNodesByKind(db.NodePage, 1000, 0)
		if err != nil {
			t.Fatalf("query pages: %v", err)
		}
		if len(pages) < 3 {
			t.Errorf("expected ≥3 pages, got %d", len(pages))
		}
		t.Logf("Pages: %d", len(pages))
	})

	t.Run("edge_counts", func(t *testing.T) {
		entities, err := repo.GetNodesByKind(db.NodeEntity, 100, 0)
		if err != nil {
			t.Fatalf("query entities: %v", err)
		}

		hasFieldRelation := false
		hasTouchesEntity := false

		for _, entity := range entities {
			sg, err := repo.GetConnected(entity.ID, 2)
			if err != nil {
				t.Fatalf("get connected for %s: %v", entity.ID, err)
			}
			for _, edge := range sg.Edges {
				if edge.Kind == db.EdgeFieldRelation {
					hasFieldRelation = true
				}
				if edge.Kind == db.EdgeTouchesEntity {
					hasTouchesEntity = true
				}
			}
		}

		if !hasFieldRelation {
			t.Error("expected ≥1 field_relation edge (from Prisma)")
		}
		if !hasTouchesEntity {
			t.Error("expected ≥1 touches_entity edge (from linker-ts)")
		}
	})

	t.Run("query_routes", func(t *testing.T) {
		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}

		// Route names vary by scanner format (e.g., "[routerName] GET /path").
		// Use substring matching for method + path fragment.
		assertRouteExists := func(method, pathFragment string) {
			t.Helper()
			for _, r := range routes {
				if strings.Contains(r.Name, method) && strings.Contains(r.Name, pathFragment) {
					return
				}
			}
			names := make([]string, len(routes))
			for i, r := range routes {
				names[i] = r.Name
			}
			t.Errorf("expected route with %s and %q, not found in %v", method, pathFragment, names)
		}

		assertRouteExists("GET", "/users")
		assertRouteExists("POST", "/users")
	})

	t.Run("query_entities", func(t *testing.T) {
		entities, err := repo.GetNodesByKind(db.NodeEntity, 100, 0)
		if err != nil {
			t.Fatalf("query entities: %v", err)
		}

		entityNames := make(map[string]bool)
		for _, e := range entities {
			entityNames[e.Name] = true
		}

		if !entityNames["User"] {
			t.Error("expected entity 'User' not found")
		}
		if !entityNames["Post"] {
			t.Error("expected entity 'Post' not found")
		}
	})
}

// --- Test 2: TestFTS5Search ---

func TestFTS5Search(t *testing.T) {
	_, repo := runScanPipeline(t)

	results, err := repo.Search("user", nil, 100)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected search results for 'user', got none")
	}

	// Verify results span multiple node kinds
	kinds := make(map[db.NodeKind]bool)
	for _, r := range results {
		kinds[r.Node.Kind] = true
	}

	if !kinds[db.NodeRoute] {
		t.Error("expected search results to include route nodes")
	}
	if !kinds[db.NodeEntity] {
		t.Error("expected search results to include entity nodes")
	}
	t.Logf("Search 'user': %d results across kinds %v", len(results), kinds)
}

// --- Test 3: TestCrossScannerEdgeTraversal ---

func TestCrossScannerEdgeTraversal(t *testing.T) {
	_, repo := runScanPipeline(t)

	// Find the User entity node
	entities, err := repo.GetNodesByKind(db.NodeEntity, 100, 0)
	if err != nil {
		t.Fatalf("query entities: %v", err)
	}

	var userEntityID string
	for _, e := range entities {
		if e.Name == "User" {
			userEntityID = e.ID
			break
		}
	}
	if userEntityID == "" {
		t.Fatal("User entity not found")
	}

	// GetConnected from entity → verify edges to route nodes from different scanners
	sg, err := repo.GetConnected(userEntityID, 2)
	if err != nil {
		t.Fatalf("get connected from %s: %v", userEntityID, err)
	}

	// Verify touches_entity edges exist (created by linker-ts, connecting Express routes to Prisma entities)
	hasTouchesEntity := false
	for _, edge := range sg.Edges {
		if edge.Kind == db.EdgeTouchesEntity {
			hasTouchesEntity = true
			break
		}
	}
	if !hasTouchesEntity {
		t.Error("expected touches_entity edges in connected subgraph (cross-scanner composition)")
	}

	// Verify the subgraph contains route nodes connected to this entity
	hasRouteNode := false
	for _, node := range sg.Nodes {
		if node.Kind == db.NodeRoute {
			hasRouteNode = true
			break
		}
	}
	if !hasRouteNode {
		t.Error("expected route nodes in connected subgraph of User entity")
	}

	t.Logf("User entity subgraph: %d nodes, %d edges", len(sg.Nodes), len(sg.Edges))
}

// --- Test 4: TestActionAndMatch ---

func TestActionAndMatch(t *testing.T) {
	_, repo := runScanPipeline(t)

	actions := graph.NewActionService(repo)
	matcher := match.NewMatchService(repo, actions, match.MatchOptions{})

	// Use real route and entity IDs from scanned data
	routes, err := repo.GetNodesByKind(db.NodeRoute, 1, 0)
	if err != nil || len(routes) == 0 {
		t.Fatal("no routes found to reference in action")
	}
	entities, err := repo.GetNodesByKind(db.NodeEntity, 1, 0)
	if err != nil || len(entities) == 0 {
		t.Fatal("no entities found to reference in action")
	}

	// Create action with Gherkin pattern referencing scanned nodes
	_, err = actions.Create(graph.CreateActionInput{
		Name:            "List Users",
		Label:           "User retrieves the list of users",
		GherkinPatterns: []string{"the user views the user list"},
		RouteRefs:       []string{routes[0].ID},
		EntityRefs:      []string{entities[0].ID},
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}

	// Match step text → verify tier 1 (exact) hit
	result, err := matcher.Match("the user views the user list")
	if err != nil {
		t.Fatalf("match step: %v", err)
	}

	if result.Tier != "exact" {
		t.Errorf("expected tier 'exact', got %q", result.Tier)
	}
}

// --- Test 5: TestMCPTools ---

func TestMCPTools(t *testing.T) {
	dbPath, _ := runScanPipeline(t)

	// Create AbacusServer pointing to the populated DB
	srv, err := abacusmcp.NewAbacusServer(dbPath, "")
	if err != nil {
		t.Fatalf("create MCP server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	ctx := context.Background()

	// Connect server and client via in-memory transport
	t1, t2 := gomcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, t1)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "e2e-test", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	// Call abacus.query_routes
	routeResult, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name:      "abacus.query_routes",
		Arguments: map[string]any{"limit": 100},
	})
	if err != nil {
		t.Fatalf("call query_routes: %v", err)
	}

	routeText := routeResult.Content[0].(*gomcp.TextContent).Text
	var routes []json.RawMessage
	if err := json.Unmarshal([]byte(routeText), &routes); err != nil {
		t.Fatalf("unmarshal routes: %v", err)
	}
	if len(routes) < 4 {
		t.Errorf("expected ≥4 routes from MCP tool, got %d", len(routes))
	}

	// Call abacus.stats
	statsResult, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name: "abacus.stats",
	})
	if err != nil {
		t.Fatalf("call stats: %v", err)
	}

	statsText := statsResult.Content[0].(*gomcp.TextContent).Text
	var stats map[string]int
	if err := json.Unmarshal([]byte(statsText), &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}

	if stats["route"] < 4 {
		t.Errorf("expected ≥4 routes in stats, got %d", stats["route"])
	}
	if stats["entity"] < 2 {
		t.Errorf("expected ≥2 entities in stats, got %d", stats["entity"])
	}
	if stats["page"] < 3 {
		t.Errorf("expected ≥3 pages in stats, got %d", stats["page"])
	}
	t.Logf("MCP stats: %v", stats)
}
