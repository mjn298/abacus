package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDB opens a SQLite database at the given path and configures it for
// optimal CLI performance with WAL mode, foreign keys, and appropriate cache.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Set pragmas for performance and correctness
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-64000", // 64MB
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	return db, nil
}

// InitSchema creates the database schema if it does not exist and verifies
// the schema version matches the expected version.
func InitSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(schemaSQL); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}

	// Check/set schema version
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count); err != nil {
		return fmt.Errorf("checking schema version: %w", err)
	}

	if count == 0 {
		if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
	} else {
		var version int
		if err := tx.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
			return fmt.Errorf("reading schema version: %w", err)
		}
		if version != schemaVersion {
			// Schema version mismatch — drop all tables/triggers and recreate
			dropStatements := []string{
				"DROP TRIGGER IF EXISTS nodes_ai",
				"DROP TRIGGER IF EXISTS nodes_ad",
				"DROP TRIGGER IF EXISTS nodes_au",
				"DROP TABLE IF EXISTS nodes_fts",
				"DROP TABLE IF EXISTS edges",
				"DROP TABLE IF EXISTS nodes",
				"DROP TABLE IF EXISTS schema_version",
			}
			for _, stmt := range dropStatements {
				if _, err := tx.Exec(stmt); err != nil {
					return fmt.Errorf("dropping old schema: %w", err)
				}
			}
			if _, err := tx.Exec(schemaSQL); err != nil {
				return fmt.Errorf("recreating schema: %w", err)
			}
			if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
				return fmt.Errorf("setting new schema version: %w", err)
			}
		}
	}

	return tx.Commit()
}

// MarshalProperties converts a properties map to JSON for storage.
func MarshalProperties(props map[string]any) (string, error) {
	if props == nil {
		return "{}", nil
	}
	data, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshaling properties: %w", err)
	}
	return string(data), nil
}

// UnmarshalProperties converts JSON from storage back to a properties map.
func UnmarshalProperties(data string) (map[string]any, error) {
	var props map[string]any
	if err := json.Unmarshal([]byte(data), &props); err != nil {
		return nil, fmt.Errorf("unmarshaling properties: %w", err)
	}
	return props, nil
}
