package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func openAndInitTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return db
}

func TestOpenDB_WALAndForeignKeys(t *testing.T) {
	db := openTestDB(t)

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("querying foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestInitSchema_CreatesTables(t *testing.T) {
	db := openAndInitTestDB(t)

	tables := []string{"nodes", "edges", "nodes_fts", "schema_version"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type IN ('table','shadow') AND name=?", table,
		).Scan(&name)
		if err == sql.ErrNoRows {
			// FTS5 tables may appear differently; check if it's a virtual table
			err = db.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
			).Scan(&name)
		}
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	db := openTestDB(t)

	if err := InitSchema(db); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("second InitSchema: %v", err)
	}

	var version int
	if err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("reading version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("version = %d, want %d", version, schemaVersion)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count); err != nil {
		t.Fatalf("counting versions: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_version row count = %d, want 1", count)
	}
}

func TestNodeInsertAndSelect_AllKinds(t *testing.T) {
	db := openAndInitTestDB(t)

	kinds := []NodeKind{NodeRoute, NodeEntity, NodePage, NodeAction, NodePermission}
	for i, kind := range kinds {
		id := fmt.Sprintf("node-%d", i)
		name := fmt.Sprintf("test-%s", kind)
		_, err := db.Exec(
			"INSERT INTO nodes (id, kind, name, label, source) VALUES (?, ?, ?, ?, ?)",
			id, string(kind), name, "Test Label", "scan",
		)
		if err != nil {
			t.Fatalf("inserting node kind %q: %v", kind, err)
		}

		var gotKind, gotName string
		err = db.QueryRow("SELECT kind, name FROM nodes WHERE id=?", id).Scan(&gotKind, &gotName)
		if err != nil {
			t.Fatalf("selecting node %q: %v", id, err)
		}
		if gotKind != string(kind) {
			t.Errorf("kind = %q, want %q", gotKind, kind)
		}
		if gotName != name {
			t.Errorf("name = %q, want %q", gotName, name)
		}
	}
}

func TestEdgePrimaryKeyConstraint(t *testing.T) {
	db := openAndInitTestDB(t)

	// Insert two nodes
	_, err := db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n1', 'route', 'r1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n1: %v", err)
	}
	_, err = db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n2', 'entity', 'e1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n2: %v", err)
	}

	// Insert edge
	_, err = db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind) VALUES ('e1', 'n1', 'n2', 'field_relation')",
	)
	if err != nil {
		t.Fatalf("inserting edge: %v", err)
	}

	// Same src/dst/kind with different id should succeed (no UNIQUE constraint)
	_, err = db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind) VALUES ('e2', 'n1', 'n2', 'field_relation')",
	)
	if err != nil {
		t.Fatalf("inserting second edge with same src/dst/kind but different id should succeed: %v", err)
	}

	// Duplicate primary key should still fail
	_, err = db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind) VALUES ('e1', 'n1', 'n2', 'field_relation')",
	)
	if err == nil {
		t.Fatal("expected primary key violation, got nil")
	}
}

func TestEdgeForeignKeyConstraint(t *testing.T) {
	db := openAndInitTestDB(t)

	// Edge with non-existent src_id should fail
	_, err := db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind) VALUES ('e1', 'nonexistent', 'also-nonexistent', 'uses_route')",
	)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}
}

func TestFTS5Search(t *testing.T) {
	db := openAndInitTestDB(t)

	// Insert nodes with searchable content
	nodes := []struct {
		id, name, label string
	}{
		{"n1", "user_profile", "User Profile Page"},
		{"n2", "admin_dashboard", "Admin Dashboard"},
		{"n3", "order_history", "Order History View"},
	}
	for _, n := range nodes {
		_, err := db.Exec(
			"INSERT INTO nodes (id, kind, name, label, source) VALUES (?, 'page', ?, ?, 'scan')",
			n.id, n.name, n.label,
		)
		if err != nil {
			t.Fatalf("inserting node %q: %v", n.id, err)
		}
	}

	// Search for "dashboard"
	rows, err := db.Query(
		"SELECT name, rank FROM nodes_fts WHERE nodes_fts MATCH ? ORDER BY rank",
		"dashboard",
	)
	if err != nil {
		t.Fatalf("FTS5 query: %v", err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var name string
		var rank float64
		if err := rows.Scan(&name, &rank); err != nil {
			t.Fatalf("scanning FTS result: %v", err)
		}
		results = append(results, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating FTS results: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("FTS5 search returned no results")
	}
	if results[0] != "admin_dashboard" {
		t.Errorf("top result = %q, want %q", results[0], "admin_dashboard")
	}
}

func TestJSONPropertiesRoundTrip(t *testing.T) {
	db := openAndInitTestDB(t)

	props := map[string]any{
		"method":  "GET",
		"path":    "/api/users",
		"count":   float64(42),
		"nested":  map[string]any{"key": "value"},
		"enabled": true,
	}

	propsJSON, err := MarshalProperties(props)
	if err != nil {
		t.Fatalf("MarshalProperties: %v", err)
	}

	_, err = db.Exec(
		"INSERT INTO nodes (id, kind, name, properties, source) VALUES ('n1', 'route', 'test', ?, 'scan')",
		propsJSON,
	)
	if err != nil {
		t.Fatalf("inserting node with properties: %v", err)
	}

	var gotJSON string
	if err := db.QueryRow("SELECT properties FROM nodes WHERE id='n1'").Scan(&gotJSON); err != nil {
		t.Fatalf("selecting properties: %v", err)
	}

	gotProps, err := UnmarshalProperties(gotJSON)
	if err != nil {
		t.Fatalf("UnmarshalProperties: %v", err)
	}

	if gotProps["method"] != "GET" {
		t.Errorf("method = %v, want GET", gotProps["method"])
	}
	if gotProps["path"] != "/api/users" {
		t.Errorf("path = %v, want /api/users", gotProps["path"])
	}
	if gotProps["count"] != float64(42) {
		t.Errorf("count = %v, want 42", gotProps["count"])
	}
	if gotProps["enabled"] != true {
		t.Errorf("enabled = %v, want true", gotProps["enabled"])
	}
	nested, ok := gotProps["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested is not map[string]any: %T", gotProps["nested"])
	}
	if nested["key"] != "value" {
		t.Errorf("nested.key = %v, want value", nested["key"])
	}
}

func TestMarshalProperties_Nil(t *testing.T) {
	result, err := MarshalProperties(nil)
	if err != nil {
		t.Fatalf("MarshalProperties(nil): %v", err)
	}
	if result != "{}" {
		t.Errorf("result = %q, want %q", result, "{}")
	}
}

func TestCascadingEdgeDeletion(t *testing.T) {
	db := openAndInitTestDB(t)

	// Insert nodes
	_, err := db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n1', 'route', 'r1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n1: %v", err)
	}
	_, err = db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n2', 'entity', 'e1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n2: %v", err)
	}

	// Insert edge
	_, err = db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind) VALUES ('e1', 'n1', 'n2', 'uses_route')",
	)
	if err != nil {
		t.Fatalf("inserting edge: %v", err)
	}

	// Verify edge exists
	var edgeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM edges WHERE src_id='n1'").Scan(&edgeCount); err != nil {
		t.Fatalf("counting edges: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("edge count = %d, want 1", edgeCount)
	}

	// Delete the source node
	_, err = db.Exec("DELETE FROM nodes WHERE id='n1'")
	if err != nil {
		t.Fatalf("deleting n1: %v", err)
	}

	// Edge should be cascade-deleted
	if err := db.QueryRow("SELECT COUNT(*) FROM edges WHERE src_id='n1'").Scan(&edgeCount); err != nil {
		t.Fatalf("counting edges after delete: %v", err)
	}
	if edgeCount != 0 {
		t.Errorf("edge count after cascade = %d, want 0", edgeCount)
	}
}

func TestEdgeSourceScanner(t *testing.T) {
	db := openAndInitTestDB(t)

	// Insert two nodes
	_, err := db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n1', 'route', 'r1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n1: %v", err)
	}
	_, err = db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n2', 'entity', 'e1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n2: %v", err)
	}

	// Insert edge with source_scanner set
	_, err = db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind, source_scanner) VALUES ('e1', 'n1', 'n2', 'touches_entity', 'prisma')",
	)
	if err != nil {
		t.Fatalf("inserting edge with source_scanner: %v", err)
	}

	var gotScanner sql.NullString
	err = db.QueryRow("SELECT source_scanner FROM edges WHERE id='e1'").Scan(&gotScanner)
	if err != nil {
		t.Fatalf("selecting source_scanner: %v", err)
	}
	if !gotScanner.Valid {
		t.Fatal("expected source_scanner to be non-NULL")
	}
	if gotScanner.String != "prisma" {
		t.Errorf("source_scanner = %q, want %q", gotScanner.String, "prisma")
	}
}

func TestEdgeSourceScanner_Null(t *testing.T) {
	db := openAndInitTestDB(t)

	// Insert two nodes
	_, err := db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n1', 'route', 'r1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n1: %v", err)
	}
	_, err = db.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n2', 'entity', 'e1', 'scan')")
	if err != nil {
		t.Fatalf("inserting n2: %v", err)
	}

	// Insert edge without source_scanner
	_, err = db.Exec(
		"INSERT INTO edges (id, src_id, dst_id, kind) VALUES ('e1', 'n1', 'n2', 'touches_entity')",
	)
	if err != nil {
		t.Fatalf("inserting edge without source_scanner: %v", err)
	}

	var gotScanner sql.NullString
	err = db.QueryRow("SELECT source_scanner FROM edges WHERE id='e1'").Scan(&gotScanner)
	if err != nil {
		t.Fatalf("selecting source_scanner: %v", err)
	}
	if gotScanner.Valid {
		t.Errorf("expected source_scanner to be NULL, got %q", gotScanner.String)
	}
}

func TestTransactionRollback(t *testing.T) {
	db := openAndInitTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	_, err = tx.Exec("INSERT INTO nodes (id, kind, name, source) VALUES ('n1', 'route', 'r1', 'scan')")
	if err != nil {
		t.Fatalf("inserting in tx: %v", err)
	}

	// Rollback
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// Node should not exist
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id='n1'").Scan(&count); err != nil {
		t.Fatalf("counting nodes: %v", err)
	}
	if count != 0 {
		t.Errorf("node count after rollback = %d, want 0", count)
	}
}
