package graph

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/scanner"
)

// GraphRepository provides CRUD operations, search, and traversal for
// the application graph stored in SQLite.
type GraphRepository struct {
	database *sql.DB
}

// NewGraphRepository creates a new GraphRepository backed by the given database.
func NewGraphRepository(database *sql.DB) *GraphRepository {
	return &GraphRepository{database: database}
}

// SearchResult holds a node and its FTS5 BM25 rank score.
type SearchResult struct {
	Node db.GraphNode
	Rank float64
}

// SubGraph holds a set of connected nodes and the edges between them.
type SubGraph struct {
	Nodes []db.GraphNode `json:"nodes"`
	Edges []db.GraphEdge `json:"edges"`
}

// InsertNode inserts a new node into the graph. Returns an error if a node
// with the same ID already exists.
func (r *GraphRepository) InsertNode(node *db.GraphNode) error {
	props, err := db.MarshalProperties(node.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	_, err = r.database.Exec(
		`INSERT INTO nodes (id, kind, name, label, properties, source, source_file, scan_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		node.ID, string(node.Kind), node.Name, node.Label, props,
		string(node.Source), node.SourceFile, node.ScanHash,
	)
	if err != nil {
		return fmt.Errorf("insert node %q: %w", node.ID, err)
	}
	return nil
}

// UpsertNode inserts a node or updates it if it already exists (matched by ID).
func (r *GraphRepository) UpsertNode(node *db.GraphNode) error {
	props, err := db.MarshalProperties(node.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	_, err = r.database.Exec(
		`INSERT INTO nodes (id, kind, name, label, properties, source, source_file, scan_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind = excluded.kind,
		   name = excluded.name,
		   label = excluded.label,
		   properties = excluded.properties,
		   source = excluded.source,
		   source_file = excluded.source_file,
		   scan_hash = excluded.scan_hash,
		   updated_at = unixepoch()`,
		node.ID, string(node.Kind), node.Name, node.Label, props,
		string(node.Source), node.SourceFile, node.ScanHash,
	)
	if err != nil {
		return fmt.Errorf("upsert node %q: %w", node.ID, err)
	}
	return nil
}

// GetNode retrieves a node by ID. Returns nil, nil if not found.
func (r *GraphRepository) GetNode(id string) (*db.GraphNode, error) {
	row := r.database.QueryRow(
		`SELECT id, kind, name, label, properties, source, source_file, created_at, updated_at, scan_hash
		 FROM nodes WHERE id = ?`, id,
	)
	node, err := scanNode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get node %q: %w", id, err)
	}
	return node, nil
}

// GetNodesByKind returns nodes of the given kind with limit/offset pagination.
func (r *GraphRepository) GetNodesByKind(kind db.NodeKind, limit, offset int) ([]db.GraphNode, error) {
	rows, err := r.database.Query(
		`SELECT id, kind, name, label, properties, source, source_file, created_at, updated_at, scan_hash
		 FROM nodes WHERE kind = ? ORDER BY id LIMIT ? OFFSET ?`,
		string(kind), limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get nodes by kind %q: %w", kind, err)
	}
	defer rows.Close()

	return scanNodes(rows)
}

// DeleteNode removes a node by ID. Edges are cascade-deleted by the DB.
func (r *GraphRepository) DeleteNode(id string) error {
	_, err := r.database.Exec("DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete node %q: %w", id, err)
	}
	return nil
}

// InsertEdge inserts or updates an edge by ID, preserving created_at on conflict.
func (r *GraphRepository) InsertEdge(edge *db.GraphEdge) error {
	props, err := db.MarshalProperties(edge.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	_, err = r.database.Exec(
		`INSERT INTO edges (id, src_id, dst_id, kind, properties, source_scanner)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   src_id = excluded.src_id,
		   dst_id = excluded.dst_id,
		   kind = excluded.kind,
		   properties = excluded.properties,
		   source_scanner = excluded.source_scanner`,
		edge.ID, edge.SrcID, edge.DstID, string(edge.Kind), props, edge.SourceScanner,
	)
	if err != nil {
		return fmt.Errorf("insert edge %q: %w", edge.ID, err)
	}
	return nil
}

// GetEdgesFrom returns all edges originating from the given node ID.
// If kind is non-nil, only edges of that kind are returned.
func (r *GraphRepository) GetEdgesFrom(nodeID string, kind *db.EdgeKind) ([]db.GraphEdge, error) {
	var rows *sql.Rows
	var err error
	if kind != nil {
		rows, err = r.database.Query(
			`SELECT id, src_id, dst_id, kind, properties, source_scanner, created_at
			 FROM edges WHERE src_id = ? AND kind = ?`,
			nodeID, string(*kind),
		)
	} else {
		rows, err = r.database.Query(
			`SELECT id, src_id, dst_id, kind, properties, source_scanner, created_at
			 FROM edges WHERE src_id = ?`,
			nodeID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("get edges from %q: %w", nodeID, err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

// GetEdgesTo returns all edges pointing to the given node ID.
// If kind is non-nil, only edges of that kind are returned.
func (r *GraphRepository) GetEdgesTo(nodeID string, kind *db.EdgeKind) ([]db.GraphEdge, error) {
	var rows *sql.Rows
	var err error
	if kind != nil {
		rows, err = r.database.Query(
			`SELECT id, src_id, dst_id, kind, properties, source_scanner, created_at
			 FROM edges WHERE dst_id = ? AND kind = ?`,
			nodeID, string(*kind),
		)
	} else {
		rows, err = r.database.Query(
			`SELECT id, src_id, dst_id, kind, properties, source_scanner, created_at
			 FROM edges WHERE dst_id = ?`,
			nodeID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("get edges to %q: %w", nodeID, err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

// Search performs a full-text search using FTS5 with BM25 ranking.
// If kind is non-nil, results are filtered to that node kind.
func (r *GraphRepository) Search(query string, kind *db.NodeKind, limit int) ([]SearchResult, error) {
	var sqlStr string
	var args []any

	if kind != nil {
		sqlStr = `SELECT n.id, n.kind, n.name, n.label, n.properties, n.source,
		                 n.source_file, n.created_at, n.updated_at, n.scan_hash,
		                 fts.rank
		          FROM nodes_fts fts
		          JOIN nodes n ON n.rowid = fts.rowid
		          WHERE nodes_fts MATCH ? AND n.kind = ?
		          ORDER BY fts.rank
		          LIMIT ?`
		args = []any{query, string(*kind), limit}
	} else {
		sqlStr = `SELECT n.id, n.kind, n.name, n.label, n.properties, n.source,
		                 n.source_file, n.created_at, n.updated_at, n.scan_hash,
		                 fts.rank
		          FROM nodes_fts fts
		          JOIN nodes n ON n.rowid = fts.rowid
		          WHERE nodes_fts MATCH ?
		          ORDER BY fts.rank
		          LIMIT ?`
		args = []any{query, limit}
	}

	rows, err := r.database.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var sr SearchResult
		var propsJSON string
		var sourceFile, scanHash sql.NullString

		err := rows.Scan(
			&sr.Node.ID, &sr.Node.Kind, &sr.Node.Name, &sr.Node.Label,
			&propsJSON, &sr.Node.Source, &sourceFile,
			&sr.Node.CreatedAt, &sr.Node.UpdatedAt, &scanHash,
			&sr.Rank,
		)
		if err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}

		sr.Node.Properties, err = db.UnmarshalProperties(propsJSON)
		if err != nil {
			return nil, fmt.Errorf("unmarshal properties: %w", err)
		}
		if sourceFile.Valid {
			sr.Node.SourceFile = &sourceFile.String
		}
		if scanHash.Valid {
			sr.Node.ScanHash = &scanHash.String
		}

		results = append(results, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return results, nil
}

// GetConnected returns the subgraph of all nodes reachable from the given
// node ID within maxDepth hops, traversing edges in both directions.
// Handles cycles via DISTINCT in the recursive CTE.
func (r *GraphRepository) GetConnected(nodeID string, maxDepth int) (*SubGraph, error) {
	// Step 1: Find all connected node IDs using recursive CTE
	nodeRows, err := r.database.Query(
		`WITH RECURSIVE connected(id, depth) AS (
			VALUES(?, 0)
			UNION
			SELECT e.dst_id, c.depth + 1 FROM edges e JOIN connected c ON e.src_id = c.id WHERE c.depth < ?
			UNION
			SELECT e.src_id, c.depth + 1 FROM edges e JOIN connected c ON e.dst_id = c.id WHERE c.depth < ?
		)
		SELECT DISTINCT n.id, n.kind, n.name, n.label, n.properties, n.source,
		       n.source_file, n.created_at, n.updated_at, n.scan_hash
		FROM nodes n JOIN connected c ON n.id = c.id`,
		nodeID, maxDepth, maxDepth,
	)
	if err != nil {
		return nil, fmt.Errorf("get connected nodes from %q: %w", nodeID, err)
	}
	defer nodeRows.Close()

	nodes, err := scanNodes(nodeRows)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return &SubGraph{}, nil
	}

	// Build a set of node IDs for edge filtering
	nodeSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n.ID] = true
	}

	// Step 2: Get all edges between the connected nodes
	edgeRows, err := r.database.Query(
		`WITH RECURSIVE connected(id, depth) AS (
			VALUES(?, 0)
			UNION
			SELECT e.dst_id, c.depth + 1 FROM edges e JOIN connected c ON e.src_id = c.id WHERE c.depth < ?
			UNION
			SELECT e.src_id, c.depth + 1 FROM edges e JOIN connected c ON e.dst_id = c.id WHERE c.depth < ?
		)
		SELECT DISTINCT e.id, e.src_id, e.dst_id, e.kind, e.properties, e.source_scanner, e.created_at
		FROM edges e
		JOIN connected c1 ON e.src_id = c1.id
		JOIN connected c2 ON e.dst_id = c2.id`,
		nodeID, maxDepth, maxDepth,
	)
	if err != nil {
		return nil, fmt.Errorf("get connected edges from %q: %w", nodeID, err)
	}
	defer edgeRows.Close()

	edges, err := scanEdges(edgeRows)
	if err != nil {
		return nil, err
	}

	return &SubGraph{Nodes: nodes, Edges: edges}, nil
}

// BulkUpsertNodes inserts or updates multiple nodes in a single transaction.
// Returns the number of nodes processed.
func (r *GraphRepository) BulkUpsertNodes(nodes []db.GraphNode) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	tx, err := r.database.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO nodes (id, kind, name, label, properties, source, source_file, scan_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind = excluded.kind,
		   name = excluded.name,
		   label = excluded.label,
		   properties = excluded.properties,
		   source = excluded.source,
		   source_file = excluded.source_file,
		   scan_hash = excluded.scan_hash,
		   updated_at = unixepoch()`,
	)
	if err != nil {
		return 0, fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

	count := 0
	for _, node := range nodes {
		props, err := db.MarshalProperties(node.Properties)
		if err != nil {
			return 0, fmt.Errorf("marshal properties for %q: %w", node.ID, err)
		}

		_, err = stmt.Exec(
			node.ID, string(node.Kind), node.Name, node.Label, props,
			string(node.Source), node.SourceFile, node.ScanHash,
		)
		if err != nil {
			return 0, fmt.Errorf("upsert node %q: %w", node.ID, err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return count, nil
}

func (r *GraphRepository) BulkUpsertEdges(edges []db.GraphEdge) (int, error) {
	if len(edges) == 0 {
		return 0, nil
	}

	tx, err := r.database.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR REPLACE INTO edges (id, src_id, dst_id, kind, properties, source_scanner)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

	count := 0
	fkWarnings := 0
	for _, edge := range edges {
		props, err := db.MarshalProperties(edge.Properties)
		if err != nil {
			return 0, fmt.Errorf("marshal properties for %q: %w", edge.ID, err)
		}

		_, err = stmt.Exec(
			edge.ID, edge.SrcID, edge.DstID, string(edge.Kind), props, edge.SourceScanner,
		)
		if err != nil {
			if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
				fkWarnings++
				log.Printf("WARN: skipping edge %q: FK violation (src=%q dst=%q)", edge.ID, edge.SrcID, edge.DstID)
				continue
			}
			return 0, fmt.Errorf("upsert edge %q: %w", edge.ID, err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return count, nil
}

func (r *GraphRepository) DeleteEdgesBySourceScanner(sourceScanner string) (int, error) {
	result, err := r.database.Exec("DELETE FROM edges WHERE source_scanner = ?", sourceScanner)
	if err != nil {
		return 0, fmt.Errorf("delete edges by source scanner %q: %w", sourceScanner, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return int(n), nil
}

func (r *GraphRepository) GetNodeRefsByKinds(kinds []db.NodeKind) ([]scanner.ScanNodeRef, error) {
	if len(kinds) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(kinds))
	args := make([]any, len(kinds))
	for i, k := range kinds {
		placeholders[i] = "?"
		args[i] = string(k)
	}

	query := fmt.Sprintf(
		"SELECT id, kind, name, source_file FROM nodes WHERE kind IN (%s)",
		strings.Join(placeholders, ", "),
	)

	rows, err := r.database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get node refs by kinds: %w", err)
	}
	defer rows.Close()

	var refs []scanner.ScanNodeRef
	for rows.Next() {
		var ref scanner.ScanNodeRef
		var sourceFile sql.NullString
		err := rows.Scan(&ref.ID, &ref.Kind, &ref.Name, &sourceFile)
		if err != nil {
			return nil, fmt.Errorf("scan node ref: %w", err)
		}
		if sourceFile.Valid {
			ref.SourceFile = sourceFile.String
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node refs: %w", err)
	}
	return refs, nil
}

// scanNode scans a single row into a GraphNode.
func scanNode(row *sql.Row) (*db.GraphNode, error) {
	var node db.GraphNode
	var propsJSON string
	var sourceFile, scanHash sql.NullString

	err := row.Scan(
		&node.ID, &node.Kind, &node.Name, &node.Label,
		&propsJSON, &node.Source, &sourceFile,
		&node.CreatedAt, &node.UpdatedAt, &scanHash,
	)
	if err != nil {
		return nil, err
	}

	node.Properties, err = db.UnmarshalProperties(propsJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}
	if sourceFile.Valid {
		node.SourceFile = &sourceFile.String
	}
	if scanHash.Valid {
		node.ScanHash = &scanHash.String
	}
	return &node, nil
}

// scanNodes scans multiple rows into a slice of GraphNodes.
func scanNodes(rows *sql.Rows) ([]db.GraphNode, error) {
	var nodes []db.GraphNode
	for rows.Next() {
		var node db.GraphNode
		var propsJSON string
		var sourceFile, scanHash sql.NullString

		err := rows.Scan(
			&node.ID, &node.Kind, &node.Name, &node.Label,
			&propsJSON, &node.Source, &sourceFile,
			&node.CreatedAt, &node.UpdatedAt, &scanHash,
		)
		if err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}

		var unmarshalErr error
		node.Properties, unmarshalErr = db.UnmarshalProperties(propsJSON)
		if unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal properties: %w", unmarshalErr)
		}
		if sourceFile.Valid {
			node.SourceFile = &sourceFile.String
		}
		if scanHash.Valid {
			node.ScanHash = &scanHash.String
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return nodes, nil
}

// scanEdges scans multiple rows into a slice of GraphEdges.
func scanEdges(rows *sql.Rows) ([]db.GraphEdge, error) {
	var edges []db.GraphEdge
	for rows.Next() {
		var edge db.GraphEdge
		var propsJSON string
		var sourceScanner sql.NullString

		err := rows.Scan(
			&edge.ID, &edge.SrcID, &edge.DstID, &edge.Kind,
			&propsJSON, &sourceScanner, &edge.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}

		edge.Properties, err = db.UnmarshalProperties(propsJSON)
		if err != nil {
			return nil, fmt.Errorf("unmarshal properties: %w", err)
		}
		if sourceScanner.Valid {
			edge.SourceScanner = &sourceScanner.String
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return edges, nil
}
