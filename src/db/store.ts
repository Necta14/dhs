import { mkdirSync } from 'node:fs';
import { dirname } from 'node:path';
import { DatabaseSync, type SQLInputValue, type SQLOutputValue, type StatementSync } from 'node:sqlite';
import { fromBlob, toBlob } from '../embed/embedder.ts';
import type { VectorRows } from '../search/vector.ts';
import type {
  ChunkInput, DocFilter, DocumentInput, DocumentKind, DocumentRecord, MemoryListItem, MemoryRecord,
  MemoryType, Metadata, PendingChunk, StoreStats,
} from '../types.ts';
import { SCHEMA_SQL, SCHEMA_VERSION } from './schema.ts';

type Row = Record<string, SQLOutputValue>;

function num(v: SQLOutputValue | undefined): number {
  if (typeof v === 'number') return v;
  if (typeof v === 'bigint') return Number(v);
  if (v === null || v === undefined) return 0;
  return Number(v);
}
function numOrNull(v: SQLOutputValue | undefined): number | null {
  return v === null || v === undefined ? null : num(v);
}
function str(v: SQLOutputValue | undefined): string {
  return v === null || v === undefined ? '' : String(v);
}
function strOrNull(v: SQLOutputValue | undefined): string | null {
  return v === null || v === undefined ? null : String(v);
}
function json<T>(v: SQLOutputValue | undefined, fallback: T): T {
  if (typeof v !== 'string') return fallback;
  try {
    return JSON.parse(v) as T;
  } catch {
    return fallback;
  }
}
function blob(v: SQLOutputValue | undefined): Uint8Array {
  if (v instanceof Uint8Array) return v;
  throw new Error('Așteptam un BLOB în coloana de vector');
}
function placeholders(n: number): string {
  return new Array<string>(n).fill('?').join(',');
}

export interface UpsertResult {
  id: number;
  created: boolean;
  /** Conținutul s-a schimbat (sau documentul a fost reactivat) — fragmentele trebuie refăcute. */
  changed: boolean;
}
export interface ReplaceChunksResult {
  inserted: number;
  /** Fragmente identice cu cele vechi, care și-au păstrat vectorul fără cache și fără API. */
  reusedEmbeddings: number;
}
export interface LexicalHit {
  chunkId: number;
  /** −BM25: mai mare = mai relevant. */
  score: number;
}
export interface HitBase {
  chunkId: number;
  documentId: number;
  uri: string;
  kind: DocumentKind;
  namespace: string;
  title: string | null;
  headingPath: string | null;
  text: string;
  metadata: Metadata;
  updatedAt: number;
}
export interface SqlFragment {
  where: string;
  params: SQLInputValue[];
}
export interface MemoryInsert {
  documentId: number;
  memoryType: MemoryType;
  importance: number;
  tags: readonly string[];
  supersedes: number | null;
  expiresAt: number | null;
}

const IN_BATCH = 400;

/**
 * Tot accesul la SQLite trece pe aici. Sincron (node:sqlite), tranzacții explicite, statement-uri
 * pregătite și reutilizate. `version` crește la orice scriere care poate schimba rezultatele
 * căutării; indexul vectorial din memorie se reconstruiește când o vede schimbată.
 */
export class Store {
  readonly db: DatabaseSync;
  readonly path: string;
  version = 0;
  #txDepth = 0;
  readonly #stmts = new Map<string, StatementSync>();

  private constructor(path: string) {
    this.path = path;
    if (path !== ':memory:') mkdirSync(dirname(path), { recursive: true });
    this.db = new DatabaseSync(path);
    this.db.exec(`
      PRAGMA journal_mode = WAL;
      PRAGMA synchronous = NORMAL;
      PRAGMA foreign_keys = ON;
      PRAGMA temp_store = MEMORY;
      PRAGMA cache_size = -32000;
    `);
    this.#migrate();
  }

  static open(path = ':memory:'): Store {
    return new Store(path);
  }

  close(): void {
    this.db.close();
  }

  #migrate(): void {
    this.db.exec(SCHEMA_SQL);
    const row = this.db.prepare("SELECT value FROM meta WHERE key = 'schema_version'").get() as Row | undefined;
    if (!row) {
      this.db.prepare("INSERT INTO meta (key, value) VALUES ('schema_version', ?)").run(String(SCHEMA_VERSION));
    }
  }

  #prepare(sql: string): StatementSync {
    let s = this.#stmts.get(sql);
    if (!s) {
      s = this.db.prepare(sql);
      this.#stmts.set(sql, s);
    }
    return s;
  }

  transaction<T>(fn: () => T): T {
    if (this.#txDepth > 0) return fn();
    this.db.exec('BEGIN IMMEDIATE');
    this.#txDepth++;
    try {
      const result = fn();
      this.db.exec('COMMIT');
      return result;
    } catch (err) {
      this.db.exec('ROLLBACK');
      throw err;
    } finally {
      this.#txDepth--;
    }
  }

  /** Compactează indexul FTS și actualizează statisticile planificatorului. De rulat după ingestii mari. */
  optimize(): void {
    this.db.exec("INSERT INTO chunks_fts (chunks_fts) VALUES ('optimize'); PRAGMA optimize;");
  }

  // ───────────────────────────── documente ─────────────────────────────

  upsertDocument(input: DocumentInput, now = Date.now()): UpsertResult {
    const existing = this.#prepare('SELECT id, content_hash, active FROM documents WHERE uri = ?').get(input.uri) as Row | undefined;
    const meta = JSON.stringify(input.metadata ?? {});
    if (!existing) {
      const r = this.#prepare(
        `INSERT INTO documents (uri, kind, namespace, title, content_hash, metadata, created_at, updated_at, active)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
      ).run(input.uri, input.kind, input.namespace, input.title, input.contentHash, meta, now, now);
      this.version++;
      return { id: Number(r.lastInsertRowid), created: true, changed: true };
    }
    const id = num(existing.id);
    const changed = str(existing.content_hash) !== input.contentHash || num(existing.active) !== 1;
    this.#prepare(
      `UPDATE documents
         SET kind = ?, namespace = ?, title = ?, content_hash = ?, metadata = ?,
             updated_at = CASE WHEN content_hash = ? AND active = 1 THEN updated_at ELSE ? END,
             active = 1
       WHERE id = ?`,
    ).run(input.kind, input.namespace, input.title, input.contentHash, meta, input.contentHash, now, id);
    if (changed) this.version++;
    return { id, created: false, changed };
  }

  getDocument(id: number): DocumentRecord | null {
    const r = this.#prepare('SELECT * FROM documents WHERE id = ?').get(id) as Row | undefined;
    return r ? docFromRow(r) : null;
  }

  getDocumentByUri(uri: string): DocumentRecord | null {
    const r = this.#prepare('SELECT * FROM documents WHERE uri = ?').get(uri) as Row | undefined;
    return r ? docFromRow(r) : null;
  }

  listDocuments(filter: DocFilter & { limit?: number | undefined } = {}): DocumentRecord[] {
    const { where, params } = this.filterSql(filter);
    const rows = this.db
      .prepare(`SELECT d.* FROM documents d WHERE ${where} ORDER BY d.id LIMIT ?`)
      .all(...params, filter.limit ?? 100_000) as Row[];
    return rows.map(docFromRow);
  }

  setDocumentActive(id: number, active: boolean, now = Date.now()): void {
    const r = this.#prepare('UPDATE documents SET active = ?, updated_at = ? WHERE id = ? AND active <> ?').run(
      active ? 1 : 0, now, id, active ? 1 : 0,
    );
    if (Number(r.changes) > 0) this.version++;
  }

  /** Ștergere fizică (cascadă pe fragmente și memorie). Preferă setDocumentActive(false). */
  deleteDocument(id: number): boolean {
    const r = this.#prepare('DELETE FROM documents WHERE id = ?').run(id);
    if (Number(r.changes) > 0) this.version++;
    return Number(r.changes) > 0;
  }

  // ───────────────────────────── fragmente ─────────────────────────────

  replaceChunks(documentId: number, chunks: readonly ChunkInput[]): ReplaceChunksResult {
    return this.transaction(() => {
      const old = this.#prepare(
        'SELECT content_hash, embedding, embedding_model FROM chunks WHERE document_id = ? AND embedding IS NOT NULL',
      ).all(documentId) as Row[];
      const reuse = new Map<string, { embedding: Uint8Array; model: string }>();
      for (const r of old) reuse.set(str(r.content_hash), { embedding: blob(r.embedding), model: str(r.embedding_model) });

      this.#prepare('DELETE FROM chunks WHERE document_id = ?').run(documentId);
      const insert = this.#prepare(
        `INSERT INTO chunks (document_id, ordinal, heading_path, text, content_hash, token_estimate, embedding, embedding_model)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
      );
      let reused = 0;
      for (const c of chunks) {
        const prev = reuse.get(c.contentHash);
        if (prev) reused++;
        insert.run(documentId, c.ordinal, c.headingPath, c.text, c.contentHash, c.tokenEstimate, prev?.embedding ?? null, prev?.model ?? null);
      }
      this.version++;
      return { inserted: chunks.length, reusedEmbeddings: reused };
    });
  }

  pendingChunks(modelId: string, limit: number): PendingChunk[] {
    const rows = this.#prepare(
      `SELECT c.id, c.document_id, c.heading_path, c.text, d.title
         FROM chunks c JOIN documents d ON d.id = c.document_id
        WHERE d.active = 1 AND (c.embedding IS NULL OR c.embedding_model IS NOT ?)
        ORDER BY c.id LIMIT ?`,
    ).all(modelId, limit) as Row[];
    return rows.map((r) => ({
      id: num(r.id),
      documentId: num(r.document_id),
      headingPath: strOrNull(r.heading_path),
      text: str(r.text),
      title: strOrNull(r.title),
    }));
  }

  countPending(modelId: string): number {
    const r = this.#prepare(
      `SELECT COUNT(*) AS n FROM chunks c JOIN documents d ON d.id = c.document_id
        WHERE d.active = 1 AND (c.embedding IS NULL OR c.embedding_model IS NOT ?)`,
    ).get(modelId) as Row;
    return num(r.n);
  }

  setEmbeddings(rows: readonly { chunkId: number; vector: Float32Array }[], modelId: string): void {
    if (rows.length === 0) return;
    this.transaction(() => {
      const update = this.#prepare('UPDATE chunks SET embedding = ?, embedding_model = ? WHERE id = ?');
      for (const r of rows) update.run(toBlob(r.vector), modelId, r.chunkId);
    });
    this.version++;
  }

  // ───────────────────────────── cache embeddings ─────────────────────────────

  cacheGet(keys: readonly string[]): Map<string, Float32Array> {
    const out = new Map<string, Float32Array>();
    for (let i = 0; i < keys.length; i += IN_BATCH) {
      const slice = keys.slice(i, i + IN_BATCH);
      const rows = this.db
        .prepare(`SELECT key, vector FROM embedding_cache WHERE key IN (${placeholders(slice.length)})`)
        .all(...slice) as Row[];
      for (const r of rows) out.set(str(r.key), fromBlob(blob(r.vector)));
    }
    return out;
  }

  cachePut(entries: readonly { key: string; vector: Float32Array }[], modelId: string, now = Date.now()): void {
    if (entries.length === 0) return;
    this.transaction(() => {
      const insert = this.#prepare('INSERT OR REPLACE INTO embedding_cache (key, model, vector, created_at) VALUES (?, ?, ?, ?)');
      for (const e of entries) insert.run(e.key, modelId, toBlob(e.vector), now);
    });
  }

  // ───────────────────────────── vectori ─────────────────────────────

  /** Toți vectorii activi ai unui model, într-o matrice contiguă (copiere directă din BLOB-uri). */
  loadVectors(modelId: string): VectorRows {
    const where = 'd.active = 1 AND c.embedding IS NOT NULL AND c.embedding_model = ?';
    const count = num(
      (this.#prepare(`SELECT COUNT(*) AS n FROM chunks c JOIN documents d ON d.id = c.document_id WHERE ${where}`).get(modelId) as Row).n,
    );
    const empty: VectorRows = { ids: new Int32Array(0), docIds: new Int32Array(0), matrix: new Float32Array(0), dims: 0, count: 0 };
    if (count === 0) return empty;

    const ids = new Int32Array(count);
    const docIds = new Int32Array(count);
    let dims = 0;
    let matrix: Float32Array | null = null;
    let bytes: Uint8Array | null = null;
    let i = 0;
    const stmt = this.db.prepare(
      `SELECT c.id, c.document_id, c.embedding FROM chunks c JOIN documents d ON d.id = c.document_id WHERE ${where} ORDER BY c.id`,
    );
    for (const raw of stmt.iterate(modelId)) {
      const r = raw as Row;
      const b = blob(r.embedding);
      if (matrix === null) {
        dims = b.byteLength >>> 2;
        matrix = new Float32Array(count * dims);
        bytes = new Uint8Array(matrix.buffer);
      }
      if (b.byteLength !== dims * 4 || i >= count) continue; // vector străin (altă dimensiune) — ignorat
      bytes?.set(b, i * dims * 4);
      ids[i] = num(r.id);
      docIds[i] = num(r.document_id);
      i++;
    }
    if (matrix === null) return empty;
    return { ids: ids.subarray(0, i), docIds: docIds.subarray(0, i), matrix: matrix.subarray(0, i * dims), dims, count: i };
  }

  // ───────────────────────────── căutare lexicală ─────────────────────────────

  lexicalSearch(ftsQuery: string, limit: number, filter: DocFilter = {}): LexicalHit[] {
    const { where, params } = this.filterSql(filter);
    const rows = this.db
      .prepare(
        `SELECT chunks_fts.rowid AS chunk_id, bm25(chunks_fts, 1.0, 0.6) AS rank
           FROM chunks_fts
           JOIN chunks c ON c.id = chunks_fts.rowid
           JOIN documents d ON d.id = c.document_id
          WHERE chunks_fts MATCH ? AND ${where}
          ORDER BY rank LIMIT ?`,
      )
      .all(ftsQuery, ...params, limit) as Row[];
    return rows.map((r) => ({ chunkId: num(r.chunk_id), score: -num(r.rank) }));
  }

  // ───────────────────────────── filtre ─────────────────────────────

  /** Fragment WHERE pentru aliasul `d` (documents). Ordinea parametrilor urmează ordinea condițiilor. */
  filterSql(f: DocFilter): SqlFragment {
    const conds: string[] = [];
    const params: SQLInputValue[] = [];
    const list = (values: readonly (string | number)[]): string => {
      params.push(...values);
      return placeholders(values.length);
    };
    if (!f.includeInactive) conds.push('d.active = 1');
    if (f.namespace !== undefined) {
      const ns = typeof f.namespace === 'string' ? [f.namespace] : [...f.namespace];
      if (ns.length > 0) conds.push(`d.namespace IN (${list(ns)})`);
    }
    if (f.kinds && f.kinds.length > 0) conds.push(`d.kind IN (${list([...f.kinds])})`);
    if (f.uriPrefix) {
      conds.push('d.uri >= ? AND d.uri < ?');
      params.push(f.uriPrefix, `${f.uriPrefix}￿`);
    }
    if (f.documentIds && f.documentIds.length > 0) conds.push(`d.id IN (${list([...f.documentIds])})`);
    const hasTypes = f.memoryTypes !== undefined && f.memoryTypes.length > 0;
    const hasTags = f.tags !== undefined && f.tags.length > 0;
    if (hasTypes || hasTags) {
      const sub: string[] = [];
      if (hasTypes) sub.push(`m.memory_type IN (${list([...(f.memoryTypes ?? [])])})`);
      if (hasTags) sub.push(`EXISTS (SELECT 1 FROM json_each(m.tags) AS je WHERE je.value IN (${list([...(f.tags ?? [])])}))`);
      conds.push(`d.id IN (SELECT m.document_id FROM memories m WHERE ${sub.join(' AND ')})`);
    }
    if (!f.includeInactive) {
      conds.push('NOT EXISTS (SELECT 1 FROM memories mx WHERE mx.document_id = d.id AND mx.expires_at IS NOT NULL AND mx.expires_at <= ?)');
      params.push(f.now ?? Date.now());
    }
    return { where: conds.length > 0 ? conds.join(' AND ') : '1 = 1', params };
  }

  /** Are filtrul restricții dincolo de „activ"? (Dacă nu, indexul vectorial se poate folosi direct.) */
  needsDocFilter(f: DocFilter): boolean {
    return (
      f.namespace !== undefined ||
      (f.kinds !== undefined && f.kinds.length > 0) ||
      f.uriPrefix !== undefined ||
      (f.documentIds !== undefined && f.documentIds.length > 0) ||
      (f.memoryTypes !== undefined && f.memoryTypes.length > 0) ||
      (f.tags !== undefined && f.tags.length > 0) ||
      f.includeInactive === true ||
      this.hasExpiringMemories()
    );
  }

  hasExpiringMemories(): boolean {
    const r = this.#prepare('SELECT EXISTS (SELECT 1 FROM memories WHERE expires_at IS NOT NULL) AS e').get() as Row;
    return num(r.e) === 1;
  }

  allowedDocumentIds(f: DocFilter): Set<number> {
    const { where, params } = this.filterSql(f);
    const rows = this.db.prepare(`SELECT d.id FROM documents d WHERE ${where}`).all(...params) as Row[];
    return new Set(rows.map((r) => num(r.id)));
  }

  // ───────────────────────────── hidratare ─────────────────────────────

  getHits(chunkIds: readonly number[]): Map<number, HitBase> {
    const out = new Map<number, HitBase>();
    for (let i = 0; i < chunkIds.length; i += IN_BATCH) {
      const slice = chunkIds.slice(i, i + IN_BATCH);
      const rows = this.db
        .prepare(
          `SELECT c.id AS chunk_id, c.document_id, c.heading_path, c.text,
                  d.uri, d.kind, d.namespace, d.title, d.metadata, d.updated_at
             FROM chunks c JOIN documents d ON d.id = c.document_id
            WHERE c.id IN (${placeholders(slice.length)})`,
        )
        .all(...slice) as Row[];
      for (const r of rows) {
        out.set(num(r.chunk_id), {
          chunkId: num(r.chunk_id),
          documentId: num(r.document_id),
          uri: str(r.uri),
          kind: str(r.kind) as DocumentKind,
          namespace: str(r.namespace),
          title: strOrNull(r.title),
          headingPath: strOrNull(r.heading_path),
          text: str(r.text),
          metadata: json<Metadata>(r.metadata, {}),
          updatedAt: num(r.updated_at),
        });
      }
    }
    return out;
  }

  // ───────────────────────────── memorie ─────────────────────────────

  insertMemory(m: MemoryInsert): void {
    this.#prepare(
      `INSERT OR REPLACE INTO memories
         (document_id, memory_type, importance, tags, supersedes, superseded_by, last_accessed_at, access_count, expires_at)
       VALUES (?, ?, ?, ?, ?, NULL, NULL, 0, ?)`,
    ).run(m.documentId, m.memoryType, m.importance, JSON.stringify([...m.tags]), m.supersedes, m.expiresAt);
    this.version++;
  }

  getMemory(documentId: number): MemoryRecord | null {
    const r = this.#prepare('SELECT * FROM memories WHERE document_id = ?').get(documentId) as Row | undefined;
    return r ? memoryFromRow(r) : null;
  }

  getMemories(documentIds: readonly number[]): Map<number, MemoryRecord> {
    const out = new Map<number, MemoryRecord>();
    for (let i = 0; i < documentIds.length; i += IN_BATCH) {
      const slice = documentIds.slice(i, i + IN_BATCH);
      const rows = this.db
        .prepare(`SELECT * FROM memories WHERE document_id IN (${placeholders(slice.length)})`)
        .all(...slice) as Row[];
      for (const r of rows) out.set(num(r.document_id), memoryFromRow(r));
    }
    return out;
  }

  touchMemories(documentIds: readonly number[], now = Date.now()): void {
    if (documentIds.length === 0) return;
    this.db
      .prepare(`UPDATE memories SET last_accessed_at = ?, access_count = access_count + 1 WHERE document_id IN (${placeholders(documentIds.length)})`)
      .run(now, ...documentIds);
    // nu incrementăm version: statisticile de acces nu schimbă rezultatele căutării
  }

  /** Memoria veche devine inactivă; legătura rămâne în ambele sensuri, pentru istoric. */
  supersede(oldDocumentId: number, newDocumentId: number, now = Date.now()): void {
    this.transaction(() => {
      this.#prepare('UPDATE memories SET superseded_by = ? WHERE document_id = ?').run(newDocumentId, oldDocumentId);
      this.#prepare('UPDATE memories SET supersedes = ? WHERE document_id = ?').run(oldDocumentId, newDocumentId);
      this.#prepare('UPDATE documents SET active = 0, updated_at = ? WHERE id = ?').run(now, oldDocumentId);
    });
    this.version++;
  }

  listMemories(filter: DocFilter & { limit?: number | undefined } = {}): MemoryListItem[] {
    const { where, params } = this.filterSql(filter);
    const rows = this.db
      .prepare(
        `SELECT m.document_id, m.memory_type, m.importance, m.tags, m.supersedes, m.superseded_by,
                m.last_accessed_at, m.access_count, m.expires_at,
                d.uri, d.namespace, d.title, d.created_at, d.updated_at, d.active, d.metadata,
                (SELECT group_concat(t, char(10) || char(10))
                   FROM (SELECT c.text AS t FROM chunks c WHERE c.document_id = d.id ORDER BY c.ordinal)) AS text
           FROM memories m JOIN documents d ON d.id = m.document_id
          WHERE ${where}
          ORDER BY d.updated_at DESC
          LIMIT ?`,
      )
      .all(...params, filter.limit ?? 1000) as Row[];
    return rows.map((r) => ({
      ...memoryFromRow(r),
      uri: str(r.uri),
      namespace: str(r.namespace),
      title: strOrNull(r.title),
      text: str(r.text),
      createdAt: num(r.created_at),
      updatedAt: num(r.updated_at),
      active: num(r.active) === 1,
      metadata: json<Metadata>(r.metadata, {}),
    }));
  }

  /** Lanțul de înlocuiri al unei memorii, de la cea mai veche la cea mai nouă. */
  memoryChain(documentId: number): MemoryListItem[] {
    const ids: number[] = [];
    const seen = new Set<number>();
    let cur: number | null = documentId;
    while (cur !== null && !seen.has(cur) && ids.length < 200) {
      seen.add(cur);
      ids.unshift(cur);
      cur = this.getMemory(cur)?.supersedes ?? null;
    }
    cur = this.getMemory(documentId)?.supersededBy ?? null;
    while (cur !== null && !seen.has(cur) && ids.length < 400) {
      seen.add(cur);
      ids.push(cur);
      cur = this.getMemory(cur)?.supersededBy ?? null;
    }
    if (ids.length === 0) return [];
    const items = this.listMemories({ documentIds: ids, includeInactive: true, limit: ids.length });
    const order = new Map(ids.map((id, i) => [id, i]));
    return items.sort((a, b) => (order.get(a.documentId) ?? 0) - (order.get(b.documentId) ?? 0));
  }

  // ───────────────────────────── statistici ─────────────────────────────

  stats(): StoreStats {
    const one = (sql: string): number => num((this.db.prepare(sql).get() as Row).n);
    const pairs = (sql: string): Record<string, number> => {
      const out: Record<string, number> = {};
      for (const r of this.db.prepare(sql).all() as Row[]) out[str(r.k)] = num(r.n);
      return out;
    };
    const pageCount = num((this.db.prepare('PRAGMA page_count').get() as Row).page_count);
    const pageSize = num((this.db.prepare('PRAGMA page_size').get() as Row).page_size);
    return {
      dbPath: this.path,
      dbBytes: pageCount * pageSize,
      documents: {
        total: one('SELECT COUNT(*) AS n FROM documents'),
        active: one('SELECT COUNT(*) AS n FROM documents WHERE active = 1'),
        byKind: pairs('SELECT kind AS k, COUNT(*) AS n FROM documents WHERE active = 1 GROUP BY kind'),
        byNamespace: pairs('SELECT namespace AS k, COUNT(*) AS n FROM documents WHERE active = 1 GROUP BY namespace'),
      },
      chunks: {
        total: one('SELECT COUNT(*) AS n FROM chunks'),
        embedded: one('SELECT COUNT(*) AS n FROM chunks WHERE embedding IS NOT NULL'),
        pending: one('SELECT COUNT(*) AS n FROM chunks c JOIN documents d ON d.id = c.document_id WHERE d.active = 1 AND c.embedding IS NULL'),
        models: (this.db.prepare('SELECT DISTINCT embedding_model AS m FROM chunks WHERE embedding_model IS NOT NULL').all() as Row[]).map((r) => str(r.m)),
      },
      memories: {
        total: one('SELECT COUNT(*) AS n FROM memories'),
        active: one('SELECT COUNT(*) AS n FROM memories m JOIN documents d ON d.id = m.document_id WHERE d.active = 1'),
        byType: pairs('SELECT m.memory_type AS k, COUNT(*) AS n FROM memories m JOIN documents d ON d.id = m.document_id WHERE d.active = 1 GROUP BY m.memory_type'),
      },
      cache: { entries: one('SELECT COUNT(*) AS n FROM embedding_cache') },
    };
  }
}

function docFromRow(r: Row): DocumentRecord {
  return {
    id: num(r.id),
    uri: str(r.uri),
    kind: str(r.kind) as DocumentKind,
    namespace: str(r.namespace),
    title: strOrNull(r.title),
    contentHash: str(r.content_hash),
    metadata: json<Metadata>(r.metadata, {}),
    createdAt: num(r.created_at),
    updatedAt: num(r.updated_at),
    active: num(r.active) === 1,
  };
}

function memoryFromRow(r: Row): MemoryRecord {
  return {
    documentId: num(r.document_id),
    memoryType: str(r.memory_type) as MemoryType,
    importance: num(r.importance),
    tags: json<string[]>(r.tags, []),
    supersedes: numOrNull(r.supersedes),
    supersededBy: numOrNull(r.superseded_by),
    lastAccessedAt: numOrNull(r.last_accessed_at),
    accessCount: num(r.access_count),
    expiresAt: numOrNull(r.expires_at),
  };
}
