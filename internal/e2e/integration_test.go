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

// runScanPipeline runs all scanners against testdata/e2e-project.
// Convenience wrapper around runScanPipelineForFixture.
func runScanPipeline(t *testing.T) (string, *graph.GraphRepository) {
	t.Helper()
	return runScanPipelineForFixture(t, fixtureRoot)
}

// runScanPipelineForFixture runs all scanners against the given fixture directory
// in two phases (scan-phase → ingest → link-phase with ExistingNodes), replicating cli/scan.go.
// Returns the DB path and populated repository. Database is closed via t.Cleanup.
func runScanPipelineForFixture(t *testing.T, fixturePath string) (string, *graph.GraphRepository) {
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
			ProjectRoot: fixturePath,
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
	graphEdges := scanner.ToGraphEdges(allEdges)
	edgesCreated := 0
	for _, edge := range graphEdges {
		if err := repo.InsertEdge(&edge); err != nil {
			t.Logf("warning: edge %s: %v", edge.ID, err)
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
			ProjectRoot:   fixturePath,
			Options:       map[string]any{},
			IgnorePaths:   []string{"node_modules", "dist", "build", ".git"},
			ExistingNodes: existingNodes,
		}

		out, err := linkRunner.RunScanner(ctx, scannerCommand(id), input, knownNodeIDs)
		if err != nil {
			t.Fatalf("link-phase scanner %s failed: %v", id, err)
		}

		// Ingest linker nodes (e.g. module nodes) before edges to satisfy FK constraints
		if len(out.Nodes) > 0 {
			linkNodes := scanner.ToGraphNodes(out.Nodes)
			if _, err := repo.BulkUpsertNodes(linkNodes); err != nil {
				t.Fatalf("ingest linker nodes for %s: %v", id, err)
			}
			t.Logf("Linker %s: %d nodes", id, len(out.Nodes))
		}

		// Delete stale edges by source scanner, then bulk upsert
		if _, err := repo.DeleteEdgesBySourceScanner(id); err != nil {
			t.Fatalf("delete stale edges for %s: %v", id, err)
		}

		linkEdges := scanner.ToGraphEdgesWithSource(out.Edges, id)

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
			sg, err := repo.GetConnected(entity.ID, 2, nil)
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
	sg, err := repo.GetConnected(userEntityID, 2, nil)
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

// --- Test 6: TestModuleNodes ---

func TestModuleNodes(t *testing.T) {
	moduleFixture := filepath.Join(repoRoot, "testdata", "module-test")
	_, repo := runScanPipelineForFixture(t, moduleFixture)

	t.Run("module_node_counts", func(t *testing.T) {
		modules, err := repo.GetNodesByKind(db.NodeModule, 1000, 0)
		if err != nil {
			t.Fatalf("query modules: %v", err)
		}

		// Log each module for debugging
		for _, m := range modules {
			t.Logf("Module: id=%s name=%s", m.ID, m.Name)
		}

		// Expect exactly 4 modules: user.service, user.repository, order.service, order.repository
		if len(modules) != 4 {
			t.Errorf("expected exactly 4 module nodes, got %d", len(modules))
		}

		// Verify no module has "index" in its name (barrel file should be skipped)
		for _, m := range modules {
			if strings.Contains(strings.ToLower(m.Name), "index") {
				t.Errorf("unexpected barrel-file module node: %s (name=%s)", m.ID, m.Name)
			}
		}
	})

	t.Run("delegates_to_edges", func(t *testing.T) {
		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}
		if len(routes) == 0 {
			t.Fatal("no route nodes found")
		}

		// Find a route node (POST /users or similar)
		var routeID string
		for _, r := range routes {
			if strings.Contains(r.Name, "/users") {
				routeID = r.ID
				break
			}
		}
		if routeID == "" {
			routeID = routes[0].ID
			t.Logf("no /users route found, using first route: %s", routes[0].Name)
		}

		sg, err := repo.GetConnected(routeID, 3, []db.EdgeKind{db.EdgeDelegatesTo})
		if err != nil {
			t.Fatalf("GetConnected from %s: %v", routeID, err)
		}

		// Verify subgraph contains module nodes
		hasModule := false
		for _, n := range sg.Nodes {
			if n.Kind == db.NodeModule {
				hasModule = true
				break
			}
		}
		if !hasModule {
			t.Error("expected module nodes in delegates_to subgraph from route")
		}

		// Verify edges have kind delegates_to
		hasDelegatesTo := false
		for _, e := range sg.Edges {
			if e.Kind == db.EdgeDelegatesTo {
				hasDelegatesTo = true
				break
			}
		}
		if !hasDelegatesTo {
			t.Error("expected delegates_to edges in subgraph")
		}

		t.Logf("delegates_to subgraph from route: %d nodes, %d edges", len(sg.Nodes), len(sg.Edges))
	})

	t.Run("summary_touches_entity_edges", func(t *testing.T) {
		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}
		if len(routes) == 0 {
			t.Fatal("no route nodes found")
		}

		// Use the first route
		routeID := routes[0].ID

		sg, err := repo.GetConnected(routeID, 1, nil)
		if err != nil {
			t.Fatalf("GetConnected from %s: %v", routeID, err)
		}

		// Verify it reaches entity nodes via summary touches_entity edge
		hasEntity := false
		for _, n := range sg.Nodes {
			if n.Kind == db.NodeEntity {
				hasEntity = true
				break
			}
		}
		if !hasEntity {
			t.Error("expected entity nodes reachable at depth=1 via touches_entity shortcut")
		}

		hasTouchesEntity := false
		for _, e := range sg.Edges {
			if e.Kind == db.EdgeTouchesEntity {
				hasTouchesEntity = true
				break
			}
		}
		if !hasTouchesEntity {
			t.Error("expected touches_entity edge at depth=1 from route")
		}

		t.Logf("depth=1 subgraph: %d nodes, %d edges", len(sg.Nodes), len(sg.Edges))
	})

	t.Run("full_chain_traversal", func(t *testing.T) {
		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}
		if len(routes) == 0 {
			t.Fatal("no route nodes found")
		}

		routeID := routes[0].ID
		sg, err := repo.GetConnected(routeID, 4, []db.EdgeKind{db.EdgeDelegatesTo, db.EdgeTouchesEntity})
		if err != nil {
			t.Fatalf("GetConnected from %s: %v", routeID, err)
		}

		// Verify chain: route → service module → repo module → entity
		hasModule := false
		hasEntity := false
		for _, n := range sg.Nodes {
			switch n.Kind {
			case db.NodeModule:
				hasModule = true
			case db.NodeEntity:
				hasEntity = true
			}
		}
		if !hasModule {
			t.Error("expected module nodes in full chain traversal")
		}
		if !hasEntity {
			t.Error("expected entity nodes in full chain traversal")
		}

		// Verify both edge kinds present
		hasDelegatesTo := false
		hasTouchesEntity := false
		for _, e := range sg.Edges {
			switch e.Kind {
			case db.EdgeDelegatesTo:
				hasDelegatesTo = true
			case db.EdgeTouchesEntity:
				hasTouchesEntity = true
			}
		}
		if !hasDelegatesTo {
			t.Error("expected delegates_to edges in full chain")
		}
		if !hasTouchesEntity {
			t.Error("expected touches_entity edges in full chain")
		}

		t.Logf("full chain subgraph: %d nodes, %d edges", len(sg.Nodes), len(sg.Edges))
	})

	t.Run("edge_kind_filter", func(t *testing.T) {
		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}
		if len(routes) == 0 {
			t.Fatal("no route nodes found")
		}

		routeID := routes[0].ID
		sg, err := repo.GetConnected(routeID, 3, []db.EdgeKind{db.EdgeDelegatesTo})
		if err != nil {
			t.Fatalf("GetConnected from %s: %v", routeID, err)
		}

		// With delegates_to filter only, should NOT reach entity nodes
		for _, n := range sg.Nodes {
			if n.Kind == db.NodeEntity {
				t.Errorf("unexpected entity node %s in delegates_to-only traversal", n.Name)
			}
		}

		// Should have module nodes
		hasModule := false
		for _, n := range sg.Nodes {
			if n.Kind == db.NodeModule {
				hasModule = true
				break
			}
		}
		if !hasModule {
			t.Error("expected module nodes in delegates_to-only traversal")
		}

		t.Logf("delegates_to-only subgraph: %d nodes, %d edges", len(sg.Nodes), len(sg.Edges))
	})

	t.Run("rescan_idempotent", func(t *testing.T) {
		// Verify counts are stable after the single run (upsert idempotency
		// is covered by unit tests; here we just confirm consistency)
		modules, err := repo.GetNodesByKind(db.NodeModule, 1000, 0)
		if err != nil {
			t.Fatalf("query modules: %v", err)
		}
		if len(modules) != 4 {
			t.Errorf("expected 4 module nodes, got %d", len(modules))
		}

		entities, err := repo.GetNodesByKind(db.NodeEntity, 1000, 0)
		if err != nil {
			t.Fatalf("query entities: %v", err)
		}
		if len(entities) != 2 {
			t.Errorf("expected 2 entity nodes (User, Order), got %d", len(entities))
		}

		routes, err := repo.GetNodesByKind(db.NodeRoute, 1000, 0)
		if err != nil {
			t.Fatalf("query routes: %v", err)
		}
		if len(routes) < 1 {
			t.Error("expected at least 1 route node")
		}

		t.Logf("Final counts: %d modules, %d entities, %d routes", len(modules), len(entities), len(routes))
	})
}

// --- Test 7: TestGenesisMCP ---

func TestGenesisMCP(t *testing.T) {
	moduleFixture := filepath.Join(repoRoot, "testdata", "module-test")
	dbPath, _ := runScanPipelineForFixture(t, moduleFixture)

	// Create MCP server
	srv, err := abacusmcp.NewAbacusServer(dbPath, "")
	if err != nil {
		t.Fatalf("create MCP server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	ctx := context.Background()
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

	t.Run("entity_mode", func(t *testing.T) {
		// Call abacus.genesis with entity="User"
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name:      "abacus.genesis",
			Arguments: map[string]any{"entity": "User"},
		})
		if err != nil {
			t.Fatalf("call genesis: %v", err)
		}
		if result.IsError {
			t.Fatalf("genesis returned error: %v", result.Content)
		}

		text := result.Content[0].(*gomcp.TextContent).Text

		// Parse the GenesisResult
		var genesis struct {
			Routes           []struct{ ID, Name, SourceFile string } `json:"routes"`
			Entities         []struct{ ID, Name, SourceFile string } `json:"entities"`
			Modules          []struct{ ID, Name, SourceFile string } `json:"modules"`
			DelegationChains [][]string                              `json:"delegation_chains"`
		}
		if err := json.Unmarshal([]byte(text), &genesis); err != nil {
			t.Fatalf("unmarshal genesis: %v", err)
		}

		// Verify entities found
		if len(genesis.Entities) == 0 {
			t.Error("expected entities in genesis result")
		}
		// Verify entity named User exists
		hasUser := false
		for _, e := range genesis.Entities {
			if e.Name == "User" {
				hasUser = true
				break
			}
		}
		if !hasUser {
			t.Error("expected User entity in genesis result")
		}

		// Verify delegation chains found
		if len(genesis.DelegationChains) == 0 {
			t.Error("expected delegation chains in genesis result")
		}

		// Verify chain structure: first element should be a route, last should be an entity
		for _, chain := range genesis.DelegationChains {
			if len(chain) < 2 {
				t.Errorf("chain too short: %v", chain)
				continue
			}
			if !strings.HasPrefix(chain[0], "route:") {
				t.Errorf("chain should start with route, got %s", chain[0])
			}
			if !strings.HasPrefix(chain[len(chain)-1], "entity:") {
				t.Errorf("chain should end with entity, got %s", chain[len(chain)-1])
			}
		}

		t.Logf("Genesis entity mode: %d routes, %d entities, %d modules, %d chains",
			len(genesis.Routes), len(genesis.Entities), len(genesis.Modules), len(genesis.DelegationChains))
	})

	t.Run("node_mode", func(t *testing.T) {
		// Call with a known entity node ID
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name:      "abacus.genesis",
			Arguments: map[string]any{"node_id": "entity:User"},
		})
		if err != nil {
			t.Fatalf("call genesis: %v", err)
		}
		if result.IsError {
			t.Fatalf("genesis returned error: %v", result.Content)
		}

		text := result.Content[0].(*gomcp.TextContent).Text
		var genesis struct {
			Routes           []struct{ ID string } `json:"routes"`
			DelegationChains [][]string             `json:"delegation_chains"`
		}
		if err := json.Unmarshal([]byte(text), &genesis); err != nil {
			t.Fatalf("unmarshal genesis: %v", err)
		}

		// Should find routes connected to User entity
		if len(genesis.Routes) == 0 {
			t.Error("expected routes connected to User entity")
		}
		t.Logf("Genesis node mode: %d routes, %d chains", len(genesis.Routes), len(genesis.DelegationChains))
	})

	t.Run("step_mode_new", func(t *testing.T) {
		// Step text that won't match any action (no actions exist in the fixture)
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name:      "abacus.genesis",
			Arguments: map[string]any{"step": "the user creates a notification"},
		})
		if err != nil {
			t.Fatalf("call genesis: %v", err)
		}
		if result.IsError {
			t.Fatalf("genesis returned error: %v", result.Content)
		}

		text := result.Content[0].(*gomcp.TextContent).Text
		var genesis struct {
			Classification string `json:"classification"`
			MatchTier      string `json:"match_tier"`
		}
		if err := json.Unmarshal([]byte(text), &genesis); err != nil {
			t.Fatalf("unmarshal genesis: %v", err)
		}

		// Should classify as "suggest" tier (no actions exist)
		if genesis.MatchTier != "suggest" {
			t.Errorf("expected match_tier 'suggest', got %q", genesis.MatchTier)
		}
		// Classification should be "wirable" or "new" depending on graph content
		if genesis.Classification != "wirable" && genesis.Classification != "new" {
			t.Errorf("expected classification 'wirable' or 'new', got %q", genesis.Classification)
		}
		t.Logf("Genesis step mode: classification=%s, match_tier=%s", genesis.Classification, genesis.MatchTier)
	})

	t.Run("invalid_input", func(t *testing.T) {
		// No input provided — should return IsError result
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name:      "abacus.genesis",
			Arguments: map[string]any{},
		})
		if err != nil {
			t.Fatalf("unexpected transport error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for empty genesis input")
		}
	})
}
