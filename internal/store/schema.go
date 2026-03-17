package store

const schema = `
CREATE TABLE IF NOT EXISTS nodes (
    uid          TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    start_line   INTEGER,
    end_line     INTEGER,
    exported     INTEGER DEFAULT 0,
    package_path TEXT,
    cluster_id   TEXT,
    indexed_at   TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_nodes_name    ON nodes(name);
CREATE INDEX IF NOT EXISTS idx_nodes_file    ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_cluster ON nodes(cluster_id);

CREATE TABLE IF NOT EXISTS edges (
    id         TEXT PRIMARY KEY,
    from_uid   TEXT NOT NULL,
    to_uid     TEXT NOT NULL,
    type       TEXT NOT NULL,
    confidence REAL NOT NULL,
    FOREIGN KEY (from_uid) REFERENCES nodes(uid) ON DELETE CASCADE,
    FOREIGN KEY (to_uid)   REFERENCES nodes(uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_uid);
CREATE INDEX IF NOT EXISTS idx_edges_to   ON edges(to_uid);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(type, confidence);

CREATE TABLE IF NOT EXISTS clusters (
    id             TEXT PRIMARY KEY,
    label          TEXT,
    cohesion_score REAL
);

CREATE TABLE IF NOT EXISTS cluster_members (
    cluster_id TEXT NOT NULL,
    symbol_uid TEXT NOT NULL,
    PRIMARY KEY (cluster_id, symbol_uid)
);

CREATE TABLE IF NOT EXISTS processes (
    id           TEXT PRIMARY KEY,
    name         TEXT,
    entry_uid    TEXT,
    process_type TEXT
);

CREATE TABLE IF NOT EXISTS process_steps (
    process_id TEXT    NOT NULL,
    step_index INTEGER NOT NULL,
    symbol_uid TEXT    NOT NULL,
    depth      INTEGER,
    PRIMARY KEY (process_id, step_index)
);

CREATE TABLE IF NOT EXISTS bm25_index (
    term       TEXT NOT NULL,
    symbol_uid TEXT NOT NULL,
    tfidf      REAL NOT NULL,
    PRIMARY KEY (term, symbol_uid)
);
CREATE INDEX IF NOT EXISTS idx_bm25_term ON bm25_index(term);

CREATE TABLE IF NOT EXISTS embed_cache (
    text_hash TEXT PRIMARY KEY,
    embedding BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS node_embeddings (
    uid       TEXT PRIMARY KEY,
    embedding BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS index_meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);
`

func (s *Store) migrate() error {
	_, err := s.write.Exec(schema)
	return err
}
