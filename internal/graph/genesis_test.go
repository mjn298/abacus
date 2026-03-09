package graph

import (
	"database/sql"
	"testing"

	"github.com/mjn/abacus/internal/db"
)

// --- Test mocks ---

// mockMatcher implements StepMatcher for testing.
type mockMatcher struct {
	result *StepMatchResult
	err    error
}

func (m *mockMatcher) MatchForGenesis(stepText string) (*StepMatchResult, error) {
	return m.result, m.err
}

// panicMatcher panics if called — used to verify entity/nodeID modes
// don't invoke the matcher.
type panicMatcher struct{}

func (m *panicMatcher) MatchForGenesis(stepText string) (*StepMatchResult, error) {
	panic("matcher should not be called for entity/nodeID mode")
}

// --- Test helper ---

func setupGenesisTestDB(t *testing.T) (*sql.DB, *GraphRepository, *GenesisService) {
	t.Helper()
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Insert test nodes
	sf1 := "src/routes/users.ts"
	sf2 := "src/services/user.service.ts"
	sf3 := "src/repositories/user.repository.ts"
	sf4 := "prisma/schema.prisma"

	nodes := []*db.GraphNode{
		{ID: "route:GET-/users", Kind: db.NodeRoute, Name: "GET /users", Label: "GET /users", Source: db.SourceScan, SourceFile: &sf1},
		{ID: "route:POST-/users", Kind: db.NodeRoute, Name: "POST /users", Label: "POST /users", Source: db.SourceScan, SourceFile: &sf1},
		{ID: "module:src/services/user.service", Kind: db.NodeModule, Name: "user.service", Label: "user.service", Source: db.SourceScan, SourceFile: &sf2},
		{ID: "module:src/repositories/user.repository", Kind: db.NodeModule, Name: "user.repository", Label: "user.repository", Source: db.SourceScan, SourceFile: &sf3},
		{ID: "entity:User", Kind: db.NodeEntity, Name: "User", Label: "User", Source: db.SourceScan, SourceFile: &sf4},
		{
			ID:   "action:list-users",
			Kind: db.NodeAction,
			Name: "List Users",
			Label: "List Users",
			Properties: map[string]any{
				"gherkin_patterns": []any{"the user views the user list"},
			},
			Source: db.SourceAgent,
		},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("setupGenesisTestDB InsertNode %s: %v", n.ID, err)
		}
	}

	// Insert test edges
	edges := []*db.GraphEdge{
		{ID: "e-dt1", SrcID: "route:GET-/users", DstID: "module:src/services/user.service", Kind: db.EdgeDelegatesTo},
		{ID: "e-dt2", SrcID: "module:src/services/user.service", DstID: "module:src/repositories/user.repository", Kind: db.EdgeDelegatesTo},
		{ID: "e-te1", SrcID: "module:src/repositories/user.repository", DstID: "entity:User", Kind: db.EdgeTouchesEntity},
		{ID: "e-te2", SrcID: "route:GET-/users", DstID: "entity:User", Kind: db.EdgeTouchesEntity},
		{ID: "e-ur1", SrcID: "action:list-users", DstID: "route:GET-/users", Kind: db.EdgeUsesRoute},
		{ID: "e-te3", SrcID: "action:list-users", DstID: "entity:User", Kind: db.EdgeTouchesEntity},
	}
	for _, e := range edges {
		if err := repo.InsertEdge(e); err != nil {
			t.Fatalf("setupGenesisTestDB InsertEdge %s: %v", e.ID, err)
		}
	}

	// Create service with panic matcher — tests that need a real matcher
	// will override it.
	svc := NewGenesisService(repo, &panicMatcher{})
	return database, repo, svc
}

// --- Validation Tests ---

func TestGenesisValidation(t *testing.T) {
	_, _, svc := setupGenesisTestDB(t)

	t.Run("no input", func(t *testing.T) {
		_, err := svc.Genesis(GenesisInput{})
		if err == nil {
			t.Fatal("expected error for no input, got nil")
		}
	})

	t.Run("multiple inputs", func(t *testing.T) {
		_, err := svc.Genesis(GenesisInput{Step: "something", Entity: "User"})
		if err == nil {
			t.Fatal("expected error for multiple inputs, got nil")
		}
	})

	t.Run("all three inputs", func(t *testing.T) {
		_, err := svc.Genesis(GenesisInput{Step: "a", Entity: "b", NodeID: "c"})
		if err == nil {
			t.Fatal("expected error for all three inputs, got nil")
		}
	})
}

// --- Step Mode Tests ---

func TestGenesisStepCovered(t *testing.T) {
	_, repo, _ := setupGenesisTestDB(t)

	// Prepare action node with routes and entities for the mock to return.
	actionNode := &ActionNode{
		Node: db.GraphNode{
			ID:   "action:list-users",
			Kind: db.NodeAction,
			Name: "List Users",
			Properties: map[string]any{
				"gherkin_patterns": []any{"the user views the user list"},
			},
		},
		Routes: []db.GraphNode{
			{ID: "route:GET-/users", Kind: db.NodeRoute, Name: "GET /users"},
		},
		Entities: []db.GraphNode{
			{ID: "entity:User", Kind: db.NodeEntity, Name: "User"},
		},
	}

	matcher := &mockMatcher{
		result: &StepMatchResult{
			Tier:   "exact",
			Action: actionNode,
		},
	}
	svc := NewGenesisService(repo, matcher)

	result, err := svc.Genesis(GenesisInput{Step: "the user views the user list"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	if result.Classification != "covered" {
		t.Errorf("Classification = %q, want %q", result.Classification, "covered")
	}
	if result.MatchTier != "exact" {
		t.Errorf("MatchTier = %q, want %q", result.MatchTier, "exact")
	}
	if result.Action == nil {
		t.Fatal("Action should not be nil")
	}
	if result.Action.ID != "action:list-users" {
		t.Errorf("Action.ID = %q, want %q", result.Action.ID, "action:list-users")
	}
	if result.Action.Name != "List Users" {
		t.Errorf("Action.Name = %q, want %q", result.Action.Name, "List Users")
	}

	// Verify traversal populated routes, entities, modules
	if len(result.Routes) == 0 {
		t.Error("expected at least 1 route from traversal")
	}
	if len(result.Entities) == 0 {
		t.Error("expected at least 1 entity from traversal")
	}
	if len(result.Modules) == 0 {
		t.Error("expected at least 1 module from traversal")
	}

	// Verify delegation chains are found
	if len(result.DelegationChains) == 0 {
		t.Error("expected at least 1 delegation chain")
	}
}

func TestGenesisStepWirable(t *testing.T) {
	_, repo, _ := setupGenesisTestDB(t)

	matcher := &mockMatcher{
		result: &StepMatchResult{
			Tier: "suggest",
			SuggestRoutes: []db.GraphNode{
				{ID: "route:GET-/users", Kind: db.NodeRoute, Name: "GET /users"},
			},
		},
	}
	svc := NewGenesisService(repo, matcher)

	result, err := svc.Genesis(GenesisInput{Step: "the user searches for users"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	if result.Classification != "wirable" {
		t.Errorf("Classification = %q, want %q", result.Classification, "wirable")
	}
	if result.MatchTier != "suggest" {
		t.Errorf("MatchTier = %q, want %q", result.MatchTier, "suggest")
	}
}

func TestGenesisStepNew(t *testing.T) {
	_, repo, _ := setupGenesisTestDB(t)

	matcher := &mockMatcher{
		result: &StepMatchResult{
			Tier:            "suggest",
			SuggestRoutes:   nil,
			SuggestEntities: nil,
		},
	}
	svc := NewGenesisService(repo, matcher)

	result, err := svc.Genesis(GenesisInput{Step: "the admin configures the flux capacitor"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	if result.Classification != "new" {
		t.Errorf("Classification = %q, want %q", result.Classification, "new")
	}
	if len(result.Routes) != 0 {
		t.Errorf("Routes len = %d, want 0", len(result.Routes))
	}
	if len(result.Entities) != 0 {
		t.Errorf("Entities len = %d, want 0", len(result.Entities))
	}
	if len(result.Modules) != 0 {
		t.Errorf("Modules len = %d, want 0", len(result.Modules))
	}
}

func TestGenesisStepFuzzy(t *testing.T) {
	_, repo, _ := setupGenesisTestDB(t)

	matcher := &mockMatcher{
		result: &StepMatchResult{
			Tier: "fuzzy",
		},
	}
	svc := NewGenesisService(repo, matcher)

	result, err := svc.Genesis(GenesisInput{Step: "the user lists stuff"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	if result.Classification != "" {
		t.Errorf("Classification = %q, want empty", result.Classification)
	}
	if result.MatchTier != "fuzzy" {
		t.Errorf("MatchTier = %q, want %q", result.MatchTier, "fuzzy")
	}
	// Fuzzy should not trigger traversal, so no routes/entities/modules
	if len(result.Routes) != 0 {
		t.Errorf("Routes len = %d, want 0", len(result.Routes))
	}
	if len(result.Entities) != 0 {
		t.Errorf("Entities len = %d, want 0", len(result.Entities))
	}
}

// --- Entity Mode Tests ---

func TestGenesisEntity(t *testing.T) {
	_, _, svc := setupGenesisTestDB(t)

	result, err := svc.Genesis(GenesisInput{Entity: "User"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	// Should find routes, modules, and the entity itself via traversal
	if len(result.Routes) == 0 {
		t.Error("expected at least 1 route")
	}
	if len(result.Entities) == 0 {
		t.Error("expected at least 1 entity")
	}
	if len(result.Modules) == 0 {
		t.Error("expected at least 1 module")
	}

	// Delegation chains should contain the expected path
	if len(result.DelegationChains) == 0 {
		t.Error("expected at least 1 delegation chain")
	}

	// Verify a chain contains the expected pattern:
	// route -> module -> module -> entity
	found := false
	for _, chain := range result.DelegationChains {
		if len(chain) >= 4 &&
			chain[0] == "route:GET-/users" &&
			chain[1] == "module:src/services/user.service" &&
			chain[2] == "module:src/repositories/user.repository" &&
			chain[3] == "entity:User" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected delegation chain [route:GET-/users -> module:... -> module:... -> entity:User], got %v", result.DelegationChains)
	}

	// Classification should be empty for entity mode (only set for step mode)
	if result.Classification != "" {
		t.Errorf("Classification = %q, want empty for entity mode", result.Classification)
	}
}

// --- NodeID Mode Tests ---

func TestGenesisNodeID(t *testing.T) {
	_, _, svc := setupGenesisTestDB(t)

	result, err := svc.Genesis(GenesisInput{NodeID: "entity:User"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	if len(result.Routes) == 0 {
		t.Error("expected at least 1 route")
	}
	if len(result.Entities) == 0 {
		t.Error("expected at least 1 entity")
	}
	if len(result.Modules) == 0 {
		t.Error("expected at least 1 module")
	}
	if len(result.DelegationChains) == 0 {
		t.Error("expected at least 1 delegation chain")
	}
}

func TestGenesisNodeIDNotFound(t *testing.T) {
	_, _, svc := setupGenesisTestDB(t)

	_, err := svc.Genesis(GenesisInput{NodeID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent node, got nil")
	}
}

// --- Hub Node Filtering Tests ---

func TestGenesisDelegationChainHubFiltering(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Set up a hub-node scenario:
	//   route:A → delegates_to → module:hub → touches_entity → entity:E1 (relevant)
	//   route:A → delegates_to → module:hub → touches_entity → entity:E2 (irrelevant)
	sf := "src/hub.ts"
	nodes := []*db.GraphNode{
		{ID: "route:A", Kind: db.NodeRoute, Name: "GET /a", Label: "GET /a", Source: db.SourceScan, SourceFile: &sf},
		{ID: "module:hub", Kind: db.NodeModule, Name: "hub", Label: "hub", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:E1", Kind: db.NodeEntity, Name: "E1", Label: "E1", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:E2", Kind: db.NodeEntity, Name: "E2", Label: "E2", Source: db.SourceScan, SourceFile: &sf},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("InsertNode %s: %v", n.ID, err)
		}
	}

	edges := []*db.GraphEdge{
		{ID: "e-h1", SrcID: "route:A", DstID: "module:hub", Kind: db.EdgeDelegatesTo},
		{ID: "e-h2", SrcID: "module:hub", DstID: "entity:E1", Kind: db.EdgeTouchesEntity},
		{ID: "e-h3", SrcID: "module:hub", DstID: "entity:E2", Kind: db.EdgeTouchesEntity},
	}
	for _, e := range edges {
		if err := repo.InsertEdge(e); err != nil {
			t.Fatalf("InsertEdge %s: %v", e.ID, err)
		}
	}

	svc := NewGenesisService(repo, &panicMatcher{})

	// Query for entity E1 only — chains ending at E2 should be filtered out.
	result, err := svc.Genesis(GenesisInput{NodeID: "entity:E1"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	// All chains must terminate at entity:E1 (the queried start node).
	for i, chain := range result.DelegationChains {
		terminal := chain[len(chain)-1]
		if terminal != "entity:E1" {
			t.Errorf("chain[%d] terminates at %q, want %q; chain = %v", i, terminal, "entity:E1", chain)
		}
	}

	// We expect at least one chain: route:A → module:hub → entity:E1
	if len(result.DelegationChains) == 0 {
		t.Fatal("expected at least 1 delegation chain ending at entity:E1, got 0")
	}

	// Verify no chain ends at entity:E2 (the irrelevant hub target).
	for i, chain := range result.DelegationChains {
		terminal := chain[len(chain)-1]
		if terminal == "entity:E2" {
			t.Errorf("chain[%d] should have been filtered out (terminates at entity:E2): %v", i, chain)
		}
	}
}

// --- Entity Hub Node Explosion Tests ---

func TestGenesisEntityHubNodeExplosion(t *testing.T) {
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	// Set up a hub-node scenario where module:Hub touches many entities,
	// but we only query for entity:Target.
	sf := "src/hub.ts"
	nodes := []*db.GraphNode{
		{ID: "entity:Target", Kind: db.NodeEntity, Name: "Target", Label: "Target", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Noise1", Kind: db.NodeEntity, Name: "Noise1", Label: "Noise1", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Noise2", Kind: db.NodeEntity, Name: "Noise2", Label: "Noise2", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Noise3", Kind: db.NodeEntity, Name: "Noise3", Label: "Noise3", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Noise4", Kind: db.NodeEntity, Name: "Noise4", Label: "Noise4", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Noise5", Kind: db.NodeEntity, Name: "Noise5", Label: "Noise5", Source: db.SourceScan, SourceFile: &sf},
		{ID: "route:R1", Kind: db.NodeRoute, Name: "GET /r1", Label: "GET /r1", Source: db.SourceScan, SourceFile: &sf},
		{ID: "module:Hub", Kind: db.NodeModule, Name: "Hub", Label: "Hub", Source: db.SourceScan, SourceFile: &sf},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("InsertNode %s: %v", n.ID, err)
		}
	}

	edges := []*db.GraphEdge{
		{ID: "e-dt1", SrcID: "route:R1", DstID: "module:Hub", Kind: db.EdgeDelegatesTo},
		{ID: "e-te-r1", SrcID: "route:R1", DstID: "entity:Target", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-hub-target", SrcID: "module:Hub", DstID: "entity:Target", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-hub-n1", SrcID: "module:Hub", DstID: "entity:Noise1", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-hub-n2", SrcID: "module:Hub", DstID: "entity:Noise2", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-hub-n3", SrcID: "module:Hub", DstID: "entity:Noise3", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-hub-n4", SrcID: "module:Hub", DstID: "entity:Noise4", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-hub-n5", SrcID: "module:Hub", DstID: "entity:Noise5", Kind: db.EdgeTouchesEntity},
	}
	for _, e := range edges {
		if err := repo.InsertEdge(e); err != nil {
			t.Fatalf("InsertEdge %s: %v", e.ID, err)
		}
	}

	svc := NewGenesisService(repo, nil) // entity case doesn't use matcher

	result, err := svc.Genesis(GenesisInput{Entity: "Target"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	// Entities should contain only Target, NOT Noise1-5.
	for _, e := range result.Entities {
		if e.ID != "entity:Target" {
			t.Errorf("unexpected entity in result: %s (hub explosion leaked noise entities)", e.ID)
		}
	}
	if len(result.Entities) != 1 {
		t.Errorf("Entities count = %d, want 1 (only Target); got %v", len(result.Entities), result.Entities)
	}

	// Routes should contain only R1.
	if len(result.Routes) != 1 {
		t.Errorf("Routes count = %d, want 1; got %v", len(result.Routes), result.Routes)
	}
	if len(result.Routes) > 0 && result.Routes[0].ID != "route:R1" {
		t.Errorf("Routes[0].ID = %q, want %q", result.Routes[0].ID, "route:R1")
	}

	// Modules should contain only Hub.
	if len(result.Modules) != 1 {
		t.Errorf("Modules count = %d, want 1; got %v", len(result.Modules), result.Modules)
	}
	if len(result.Modules) > 0 && result.Modules[0].ID != "module:Hub" {
		t.Errorf("Modules[0].ID = %q, want %q", result.Modules[0].ID, "module:Hub")
	}
}

// --- Entity Forward-Only Traversal Tests ---

func TestGenesisEntitySharedModuleNoLeak(t *testing.T) {
	// Setup: route:A and route:B both delegate to module:M.
	// route:A touches entity:Target, route:B touches entity:Other.
	// Query for entity:Target should NOT include route:B.
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	sf := "src/shared.ts"
	nodes := []*db.GraphNode{
		{ID: "route:A", Kind: db.NodeRoute, Name: "GET /a", Label: "GET /a", Source: db.SourceScan, SourceFile: &sf},
		{ID: "route:B", Kind: db.NodeRoute, Name: "GET /b", Label: "GET /b", Source: db.SourceScan, SourceFile: &sf},
		{ID: "module:M", Kind: db.NodeModule, Name: "shared-module", Label: "shared-module", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Target", Kind: db.NodeEntity, Name: "Target", Label: "Target", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Other", Kind: db.NodeEntity, Name: "Other", Label: "Other", Source: db.SourceScan, SourceFile: &sf},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("InsertNode %s: %v", n.ID, err)
		}
	}

	edges := []*db.GraphEdge{
		{ID: "e-dt-a-m", SrcID: "route:A", DstID: "module:M", Kind: db.EdgeDelegatesTo},
		{ID: "e-dt-b-m", SrcID: "route:B", DstID: "module:M", Kind: db.EdgeDelegatesTo},
		{ID: "e-te-m-target", SrcID: "module:M", DstID: "entity:Target", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-m-other", SrcID: "module:M", DstID: "entity:Other", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-a-target", SrcID: "route:A", DstID: "entity:Target", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-b-other", SrcID: "route:B", DstID: "entity:Other", Kind: db.EdgeTouchesEntity},
	}
	for _, e := range edges {
		if err := repo.InsertEdge(e); err != nil {
			t.Fatalf("InsertEdge %s: %v", e.ID, err)
		}
	}

	svc := NewGenesisService(repo, &panicMatcher{})

	result, err := svc.Genesis(GenesisInput{Entity: "Target"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	// Only route:A should appear, NOT route:B.
	for _, r := range result.Routes {
		if r.ID == "route:B" {
			t.Errorf("route:B leaked into results through shared module (bidirectional traversal bug)")
		}
	}
	if len(result.Routes) != 1 {
		t.Errorf("Routes count = %d, want 1; got %v", len(result.Routes), result.Routes)
	}
	if len(result.Routes) > 0 && result.Routes[0].ID != "route:A" {
		t.Errorf("Routes[0].ID = %q, want %q", result.Routes[0].ID, "route:A")
	}

	// Module:M should still be discovered via forward traversal from route:A.
	if len(result.Modules) != 1 {
		t.Errorf("Modules count = %d, want 1; got %v", len(result.Modules), result.Modules)
	}
}

func TestGenesisEntityMultipleSeedsSharedModule(t *testing.T) {
	// Setup: route:A and route:C both touch entity:Target and delegate to module:M.
	// route:B also delegates to module:M but touches entity:Other.
	// Query for entity:Target should include route:A and route:C, NOT route:B.
	database := setupTestDB(t)
	repo := NewGraphRepository(database)

	sf := "src/multi.ts"
	nodes := []*db.GraphNode{
		{ID: "route:A", Kind: db.NodeRoute, Name: "GET /a", Label: "GET /a", Source: db.SourceScan, SourceFile: &sf},
		{ID: "route:B", Kind: db.NodeRoute, Name: "GET /b", Label: "GET /b", Source: db.SourceScan, SourceFile: &sf},
		{ID: "route:C", Kind: db.NodeRoute, Name: "GET /c", Label: "GET /c", Source: db.SourceScan, SourceFile: &sf},
		{ID: "module:M", Kind: db.NodeModule, Name: "shared-module", Label: "shared-module", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Target", Kind: db.NodeEntity, Name: "Target", Label: "Target", Source: db.SourceScan, SourceFile: &sf},
		{ID: "entity:Other", Kind: db.NodeEntity, Name: "Other", Label: "Other", Source: db.SourceScan, SourceFile: &sf},
	}
	for _, n := range nodes {
		if err := repo.InsertNode(n); err != nil {
			t.Fatalf("InsertNode %s: %v", n.ID, err)
		}
	}

	edges := []*db.GraphEdge{
		{ID: "e-dt-a-m", SrcID: "route:A", DstID: "module:M", Kind: db.EdgeDelegatesTo},
		{ID: "e-dt-b-m", SrcID: "route:B", DstID: "module:M", Kind: db.EdgeDelegatesTo},
		{ID: "e-dt-c-m", SrcID: "route:C", DstID: "module:M", Kind: db.EdgeDelegatesTo},
		{ID: "e-te-a-target", SrcID: "route:A", DstID: "entity:Target", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-b-other", SrcID: "route:B", DstID: "entity:Other", Kind: db.EdgeTouchesEntity},
		{ID: "e-te-c-target", SrcID: "route:C", DstID: "entity:Target", Kind: db.EdgeTouchesEntity},
	}
	for _, e := range edges {
		if err := repo.InsertEdge(e); err != nil {
			t.Fatalf("InsertEdge %s: %v", e.ID, err)
		}
	}

	svc := NewGenesisService(repo, &panicMatcher{})

	result, err := svc.Genesis(GenesisInput{Entity: "Target"})
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}

	// Both route:A and route:C should appear, NOT route:B.
	routeIDs := make(map[string]bool)
	for _, r := range result.Routes {
		routeIDs[r.ID] = true
	}
	if !routeIDs["route:A"] {
		t.Error("expected route:A in results")
	}
	if !routeIDs["route:C"] {
		t.Error("expected route:C in results")
	}
	if routeIDs["route:B"] {
		t.Error("route:B leaked into results through shared module")
	}
	if len(result.Routes) != 2 {
		t.Errorf("Routes count = %d, want 2; got %v", len(result.Routes), result.Routes)
	}
}

// --- ExtractDelegationChains Tests ---

func TestExtractDelegationChains(t *testing.T) {
	t.Run("basic chain", func(t *testing.T) {
		nodes := []db.GraphNode{
			{ID: "route:r1", Kind: db.NodeRoute},
			{ID: "module:m1", Kind: db.NodeModule},
			{ID: "module:m2", Kind: db.NodeModule},
			{ID: "entity:e1", Kind: db.NodeEntity},
		}
		edges := []db.GraphEdge{
			{ID: "e1", SrcID: "route:r1", DstID: "module:m1", Kind: db.EdgeDelegatesTo},
			{ID: "e2", SrcID: "module:m1", DstID: "module:m2", Kind: db.EdgeDelegatesTo},
			{ID: "e3", SrcID: "module:m2", DstID: "entity:e1", Kind: db.EdgeTouchesEntity},
		}

		chains := ExtractDelegationChains(nodes, edges)
		if len(chains) != 1 {
			t.Fatalf("expected 1 chain, got %d: %v", len(chains), chains)
		}
		expected := []string{"route:r1", "module:m1", "module:m2", "entity:e1"}
		if len(chains[0]) != len(expected) {
			t.Fatalf("chain len = %d, want %d", len(chains[0]), len(expected))
		}
		for i, id := range expected {
			if chains[0][i] != id {
				t.Errorf("chain[0][%d] = %q, want %q", i, chains[0][i], id)
			}
		}
	})

	t.Run("multiple routes produce multiple chains", func(t *testing.T) {
		nodes := []db.GraphNode{
			{ID: "route:r1", Kind: db.NodeRoute},
			{ID: "route:r2", Kind: db.NodeRoute},
			{ID: "module:m1", Kind: db.NodeModule},
			{ID: "entity:e1", Kind: db.NodeEntity},
		}
		edges := []db.GraphEdge{
			{ID: "e1", SrcID: "route:r1", DstID: "module:m1", Kind: db.EdgeDelegatesTo},
			{ID: "e2", SrcID: "route:r2", DstID: "module:m1", Kind: db.EdgeDelegatesTo},
			{ID: "e3", SrcID: "module:m1", DstID: "entity:e1", Kind: db.EdgeTouchesEntity},
		}

		chains := ExtractDelegationChains(nodes, edges)
		if len(chains) != 2 {
			t.Fatalf("expected 2 chains, got %d: %v", len(chains), chains)
		}
	})

	t.Run("cycle does not cause infinite loop", func(t *testing.T) {
		nodes := []db.GraphNode{
			{ID: "route:r1", Kind: db.NodeRoute},
			{ID: "module:m1", Kind: db.NodeModule},
			{ID: "module:m2", Kind: db.NodeModule},
		}
		edges := []db.GraphEdge{
			{ID: "e1", SrcID: "route:r1", DstID: "module:m1", Kind: db.EdgeDelegatesTo},
			{ID: "e2", SrcID: "module:m1", DstID: "module:m2", Kind: db.EdgeDelegatesTo},
			{ID: "e3", SrcID: "module:m2", DstID: "module:m1", Kind: db.EdgeDelegatesTo}, // cycle
		}

		chains := ExtractDelegationChains(nodes, edges)
		// Should not hang. No entity terminal so no chains emitted.
		if len(chains) != 0 {
			t.Logf("chains (cycle): %v", chains)
		}
	})

	t.Run("no routes produces empty chains", func(t *testing.T) {
		nodes := []db.GraphNode{
			{ID: "module:m1", Kind: db.NodeModule},
			{ID: "entity:e1", Kind: db.NodeEntity},
		}
		edges := []db.GraphEdge{
			{ID: "e1", SrcID: "module:m1", DstID: "entity:e1", Kind: db.EdgeTouchesEntity},
		}

		chains := ExtractDelegationChains(nodes, edges)
		if len(chains) != 0 {
			t.Errorf("expected 0 chains (no routes), got %d: %v", len(chains), chains)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		chains := ExtractDelegationChains(nil, nil)
		if len(chains) != 0 {
			t.Errorf("expected 0 chains for nil inputs, got %d", len(chains))
		}
	})
}
