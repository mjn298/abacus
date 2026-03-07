package db

const schemaVersion = 3

const schemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK(kind IN ('route','entity','page','action','permission')),
    name TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    properties TEXT NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT 'scan' CHECK(source IN ('scan','agent','manual')),
    source_file TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    scan_hash TEXT
);

CREATE TABLE IF NOT EXISTS edges (
    id TEXT PRIMARY KEY,
    src_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    dst_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('uses_route','touches_entity','on_page','requires_permission','relates_to','field_relation')),
    properties TEXT NOT NULL DEFAULT '{}',
    source_scanner TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_nodes_kind ON nodes(kind);
CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
CREATE INDEX IF NOT EXISTS idx_nodes_source ON nodes(source);
CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_id);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_id);
CREATE INDEX IF NOT EXISTS idx_edges_kind ON edges(kind);
CREATE INDEX IF NOT EXISTS idx_edges_source_scanner ON edges(source_scanner);

-- FTS5 virtual table for full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
    name, label, properties,
    content='nodes',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS nodes_ai AFTER INSERT ON nodes BEGIN
    INSERT INTO nodes_fts(rowid, name, label, properties)
    VALUES (new.rowid, new.name, new.label, new.properties);
END;

CREATE TRIGGER IF NOT EXISTS nodes_ad AFTER DELETE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, name, label, properties)
    VALUES ('delete', old.rowid, old.name, old.label, old.properties);
END;

CREATE TRIGGER IF NOT EXISTS nodes_au AFTER UPDATE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, name, label, properties)
    VALUES ('delete', old.rowid, old.name, old.label, old.properties);
    INSERT INTO nodes_fts(rowid, name, label, properties)
    VALUES (new.rowid, new.name, new.label, new.properties);
END;
`
