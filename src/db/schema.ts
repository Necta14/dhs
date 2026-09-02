export const SCHEMA_VERSION = 1;

/**
 * Schema SQLite. Idempotentă (IF NOT EXISTS), rulată la fiecare deschidere.
 *  - documents: unitatea de ingestie (fișier, memorie, notă); `active = 0` = șters logic / înlocuit.
 *  - chunks: fragmentele; vectorul e BLOB Float32 LE, NULL cât timp e în așteptare.
 *  - chunks_fts: index FTS5 „external content" peste chunks, ținut sincron prin triggere.
 *  - embedding_cache: hash(text) → vector, ca re-indexarea să nu mai cheme API-ul.
 *  - memories: stratul de memorie peste documents (tip, importanță, etichete, înlocuire, expirare).
 */
export const SCHEMA_SQL = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS documents (
  id           INTEGER PRIMARY KEY,
  uri          TEXT    NOT NULL UNIQUE,
  kind         TEXT    NOT NULL CHECK (kind IN ('file', 'memory', 'note')),
  namespace    TEXT    NOT NULL DEFAULT 'default',
  title        TEXT,
  content_hash TEXT    NOT NULL,
  metadata     TEXT    NOT NULL DEFAULT '{}',
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  active       INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_documents_ns_kind ON documents (namespace, kind, active);

CREATE TABLE IF NOT EXISTS chunks (
  id              INTEGER PRIMARY KEY,
  document_id     INTEGER NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
  ordinal         INTEGER NOT NULL,
  heading_path    TEXT,
  text            TEXT    NOT NULL,
  content_hash    TEXT    NOT NULL,
  token_estimate  INTEGER NOT NULL,
  embedding       BLOB,
  embedding_model TEXT,
  UNIQUE (document_id, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_chunks_pending ON chunks (document_id) WHERE embedding IS NULL;

CREATE TABLE IF NOT EXISTS embedding_cache (
  key        TEXT    PRIMARY KEY,
  model      TEXT    NOT NULL,
  vector     BLOB    NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS memories (
  document_id      INTEGER PRIMARY KEY REFERENCES documents (id) ON DELETE CASCADE,
  memory_type      TEXT    NOT NULL,
  importance       REAL    NOT NULL DEFAULT 0.5,
  tags             TEXT    NOT NULL DEFAULT '[]',
  supersedes       INTEGER REFERENCES documents (id) ON DELETE SET NULL,
  superseded_by    INTEGER REFERENCES documents (id) ON DELETE SET NULL,
  last_accessed_at INTEGER,
  access_count     INTEGER NOT NULL DEFAULT 0,
  expires_at       INTEGER
);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories (memory_type);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
  text,
  heading_path,
  content = 'chunks',
  content_rowid = 'id',
  tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
  INSERT INTO chunks_fts (rowid, text, heading_path) VALUES (new.id, new.text, new.heading_path);
END;
CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
  INSERT INTO chunks_fts (chunks_fts, rowid, text, heading_path)
    VALUES ('delete', old.id, old.text, old.heading_path);
END;
CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE OF text, heading_path ON chunks BEGIN
  INSERT INTO chunks_fts (chunks_fts, rowid, text, heading_path)
    VALUES ('delete', old.id, old.text, old.heading_path);
  INSERT INTO chunks_fts (rowid, text, heading_path) VALUES (new.id, new.text, new.heading_path);
END;
`;
