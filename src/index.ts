import type { Config } from './config.ts';
import { Store } from './db/store.ts';
import type { Embedder } from './embed/embedder.ts';
import { GeminiEmbedder } from './embed/gemini.ts';
import { embedPending, type EmbedPendingOptions, type EmbedProgress } from './embed/pipeline.ts';
import { indexPaths, type IndexOptions, type IndexReport } from './ingest/files.ts';
import {
  handoffMarkdown, rankForHandoff, rankMemoryHits, remember as rememberImpl, scoringFrom, type HandoffItem,
} from './memory/memory.ts';
import { mmrSelect, rrfFuse, type RankedList, type Scored } from './search/hybrid.ts';
import { VectorIndex } from './search/vector.ts';
import { buildFtsQuery } from './text/normalize.ts';
import type {
  DocFilter, MemoryHit, MemoryListItem, MemoryType, RecallOptions, RememberInput, RememberResult,
  SearchHit, SearchMode, SearchOptions, StoreStats,
} from './types.ts';

export interface SuiteOptions {
  /** Implicit „:memory:". CLI-ul trimite calea din configurație. */
  dbPath?: string | undefined;
  embedder?: Embedder | null | undefined;
}

export interface IndexRunOptions extends IndexOptions {
  /** Vectorizează imediat după ingestie (dacă există embedder). Implicit true. */
  embed?: boolean | undefined;
  onProgress?: ((p: EmbedProgress) => void) | undefined;
}

export interface HandoffOptions {
  namespace?: string | undefined;
  query?: string | undefined;
  limit?: number | undefined;
  types?: readonly MemoryType[] | undefined;
  halfLifeDays?: number | undefined;
  now?: number | undefined;
}

const QUERY_CACHE_SIZE = 256;

/**
 * Fațada suitei: un Store + (opțional) un Embedder. Căutarea hibridă fuzionează prin RRF
 * lista vectorială (index în memorie) cu lista lexicală (FTS5/BM25), apoi opțional MMR.
 */
export class Suite {
  readonly store: Store;
  readonly embedder: Embedder | null;
  #index: VectorIndex | null = null;
  #indexVersion = -1;
  readonly #queryCache = new Map<string, Float32Array>();

  constructor(store: Store, embedder: Embedder | null = null) {
    this.store = store;
    this.embedder = embedder;
  }

  static open(opts: SuiteOptions = {}): Suite {
    return new Suite(Store.open(opts.dbPath ?? ':memory:'), opts.embedder ?? null);
  }

  close(): void {
    this.store.close();
  }

  // ───────────────────────────── ingestie ─────────────────────────────

  async index(paths: readonly string[], opts: IndexRunOptions = {}): Promise<IndexReport & { embed: EmbedProgress | null }> {
    const report = await indexPaths(this.store, paths, opts);
    const embed =
      this.embedder && (opts.embed ?? true)
        ? await embedPending(this.store, this.embedder, { onProgress: opts.onProgress })
        : null;
    return { ...report, embed };
  }

  embedPending(opts: EmbedPendingOptions = {}): Promise<EmbedProgress> {
    return embedPending(this.store, this.#requireEmbedder(), opts);
  }

  // ───────────────────────────── căutare ─────────────────────────────

  async search(query: string, opts: SearchOptions = {}): Promise<SearchHit[]> {
    const q = query.trim();
    if (q === '') return [];
    const limit = Math.max(1, Math.floor(opts.limit ?? 10));
    const mode: SearchMode = opts.mode ?? (this.embedder ? 'hybrid' : 'lexical');
    if (mode !== 'lexical') this.#requireEmbedder();
    const candidates = Math.max(limit, Math.floor(opts.candidates ?? Math.max(limit * 4, 40)));
    const filter = toDocFilter(opts);
    const allowed = this.store.needsDocFilter(filter) ? this.store.allowedDocumentIds(filter) : null;

    const lists: RankedList[] = [];
    const vectorScores = new Map<number, number>();
    const lexicalScores = new Map<number, number>();
    let vectorList: number[] = [];
    let lexicalList: number[] = [];

    if (mode !== 'lexical') {
      const qv = await this.#queryVector(q);
      const hits = this.#vectorIndex().search(qv, candidates, allowed ? (_chunk, doc) => allowed.has(doc) : undefined);
      vectorList = hits.map((h) => h.chunkId);
      for (const h of hits) vectorScores.set(h.chunkId, h.score);
      lists.push({ ids: vectorList, weight: opts.vectorWeight ?? 1 });
    }

    if (mode !== 'vector') {
      const andQuery = buildFtsQuery(q);
      if (andQuery) {
        const hits = this.store.lexicalSearch(andQuery, candidates, filter);
        if (hits.length < limit) {
          // Prea puține potriviri stricte: completăm cu OR (BM25 le ordonează oricum după relevanță).
          const orQuery = buildFtsQuery(q, { operator: 'OR' });
          if (orQuery && orQuery !== andQuery) {
            const seen = new Set(hits.map((h) => h.chunkId));
            for (const h of this.store.lexicalSearch(orQuery, candidates, filter)) {
              if (!seen.has(h.chunkId) && hits.length < candidates) hits.push(h);
            }
          }
        }
        lexicalList = hits.map((h) => h.chunkId);
        for (const h of hits) lexicalScores.set(h.chunkId, h.score);
        lists.push({ ids: lexicalList, weight: opts.lexicalWeight ?? 1 });
      }
    }

    let ranked: Scored[];
    if (mode === 'hybrid') {
      ranked = [...rrfFuse(lists, opts.rrfK ?? 60)]
        .map(([id, f]) => ({ id, score: f.score }))
        .sort((a, b) => b.score - a.score);
    } else if (mode === 'vector') {
      ranked = vectorList.map((id) => ({ id, score: vectorScores.get(id) ?? 0 }));
    } else {
      ranked = lexicalList.map((id) => ({ id, score: lexicalScores.get(id) ?? 0 }));
    }

    if (opts.mmrLambda !== undefined && mode !== 'lexical') {
      const index = this.#vectorIndex();
      ranked = mmrSelect(ranked, (id) => index.vectorOf(id), opts.mmrLambda, limit);
    } else {
      ranked = ranked.slice(0, limit);
    }

    const bases = this.store.getHits(ranked.map((r) => r.id));
    const vectorRank = new Map(vectorList.map((id, i) => [id, i + 1] as const));
    const lexicalRank = new Map(lexicalList.map((id, i) => [id, i + 1] as const));
    const out: SearchHit[] = [];
    for (const r of ranked) {
      const base = bases.get(r.id);
      if (!base) continue;
      out.push({
        ...base,
        score: r.score,
        vectorRank: vectorRank.get(r.id) ?? null,
        lexicalRank: lexicalRank.get(r.id) ?? null,
        vectorScore: vectorScores.get(r.id) ?? null,
        lexicalScore: lexicalScores.get(r.id) ?? null,
      });
    }
    return out;
  }

  // ───────────────────────────── memorie ─────────────────────────────

  remember(input: RememberInput): Promise<RememberResult> {
    return rememberImpl(this.store, this.embedder, input);
  }

  async recall(query: string, opts: RecallOptions = {}): Promise<MemoryHit[]> {
    const limit = Math.max(1, Math.floor(opts.limit ?? 10));
    const now = opts.now ?? Date.now();
    const hits = await this.search(query, {
      ...opts,
      kinds: ['memory'],
      memoryTypes: opts.memoryTypes ?? opts.types,
      limit: Math.max(limit * 3, 30),
      now,
      mmrLambda: undefined,
    });
    const memories = this.store.getMemories([...new Set(hits.map((h) => h.documentId))]);
    const ranked = rankMemoryHits(hits, memories, scoringFrom(opts), now).slice(0, limit);
    if (opts.touch ?? true) this.store.touchMemories(ranked.map((r) => r.documentId), now);
    return ranked;
  }

  /** Ștergere logică: memoria nu mai apare la recall, dar rămâne în istoric. */
  forget(id: number, now = Date.now()): boolean {
    const doc = this.store.getDocument(id);
    if (!doc || doc.kind !== 'memory' || !doc.active) return false;
    this.store.setDocumentActive(id, false, now);
    return true;
  }

  history(id: number): MemoryListItem[] {
    return this.store.memoryChain(id);
  }

  /** Rezumat Markdown pentru agentul următor: memoriile unui spațiu, grupate pe tip. */
  async handoff(opts: HandoffOptions = {}): Promise<{ markdown: string; items: HandoffItem[] }> {
    const namespace = opts.namespace ?? 'default';
    const limit = Math.max(1, Math.floor(opts.limit ?? 25));
    const now = opts.now ?? Date.now();
    const scoring = scoringFrom({ halfLifeDays: opts.halfLifeDays ?? 45, recencyWeight: 0.5, importanceWeight: 1 });
    let items: HandoffItem[];
    if (opts.query && opts.query.trim() !== '') {
      const hits = await this.recall(opts.query, {
        namespace, types: opts.types, limit, touch: false, now,
        halfLifeDays: scoring.halfLifeDays, recencyWeight: scoring.recencyWeight, importanceWeight: scoring.importanceWeight,
      });
      items = hits.map((h) => ({
        id: h.documentId,
        type: h.memory.memoryType,
        title: h.title,
        text: h.text,
        tags: h.memory.tags,
        importance: h.memory.importance,
        updatedAt: h.updatedAt,
        score: h.finalScore,
      }));
    } else {
      const list = this.store.listMemories({ namespace, memoryTypes: opts.types, now, limit: 5000 });
      items = rankForHandoff(list, scoring, now, limit);
    }
    return { markdown: handoffMarkdown(items, namespace, now), items };
  }

  stats(): StoreStats {
    return this.store.stats();
  }

  // ───────────────────────────── intern ─────────────────────────────

  #requireEmbedder(): Embedder {
    if (!this.embedder) {
      throw new Error('Nu există embedder configurat (lipsește GEMINI_API_KEY?). Căutarea lexicală merge cu mode: "lexical".');
    }
    return this.embedder;
  }

  #vectorIndex(): VectorIndex {
    if (!this.embedder) return VectorIndex.empty();
    if (this.#index === null || this.#indexVersion !== this.store.version) {
      this.#index = new VectorIndex(this.store.loadVectors(this.embedder.id));
      this.#indexVersion = this.store.version;
    }
    return this.#index;
  }

  async #queryVector(query: string): Promise<Float32Array> {
    const embedder = this.#requireEmbedder();
    const key = `${embedder.id}\n${query}`;
    const cached = this.#queryCache.get(key);
    if (cached) {
      this.#queryCache.delete(key);
      this.#queryCache.set(key, cached);
      return cached;
    }
    const [vector] = await embedder.embed([{ text: query }], 'query');
    if (!vector) throw new Error('Embedder-ul nu a întors vectorul interogării');
    this.#queryCache.set(key, vector);
    if (this.#queryCache.size > QUERY_CACHE_SIZE) {
      const oldest = this.#queryCache.keys().next().value;
      if (oldest !== undefined) this.#queryCache.delete(oldest);
    }
    return vector;
  }
}

function toDocFilter(o: DocFilter): DocFilter {
  return {
    namespace: o.namespace,
    kinds: o.kinds,
    uriPrefix: o.uriPrefix,
    documentIds: o.documentIds,
    memoryTypes: o.memoryTypes,
    tags: o.tags,
    includeInactive: o.includeInactive,
    now: o.now,
  };
}

export interface EmbedderFactoryOptions {
  log?: ((msg: string) => void) | undefined;
}

/** Embedder Gemini din configurație, sau null dacă lipsește cheia (modul lexical rămâne funcțional). */
export function createEmbedderFromConfig(cfg: Config, opts: EmbedderFactoryOptions = {}): Embedder | null {
  if (!cfg.apiKey) return null;
  return new GeminiEmbedder({
    apiKey: cfg.apiKey,
    model: cfg.embedModel,
    dimensions: cfg.embedDims,
    rpm: cfg.rpm,
    tpm: cfg.tpm,
    log: opts.log,
  });
}

export { Store } from './db/store.ts';
export type { Embedder, EmbedItem, EmbedTask } from './embed/embedder.ts';
export { HashEmbedder } from './embed/fake.ts';
export { GeminiEmbedder, GeminiApiError, listEmbeddingModels } from './embed/gemini.ts';
export { embedPending } from './embed/pipeline.ts';
export type { EmbedProgress } from './embed/pipeline.ts';
export { indexPaths, DEFAULT_EXTENSIONS, DEFAULT_EXCLUDED_DIRS, isForbiddenFile } from './ingest/files.ts';
export type { IndexOptions, IndexReport, FileEvent } from './ingest/files.ts';
export type { HandoffItem } from './memory/memory.ts';
export { chunkText, toChunkInputs } from './text/chunker.ts';
export type { ChunkerOptions, TextChunk } from './text/chunker.ts';
export { buildFtsQuery, sha256, tokenize, contentTokens } from './text/normalize.ts';
export { loadEnvFiles, readConfig, defaultDbPath, DEFAULTS } from './config.ts';
export type { Config } from './config.ts';
export * from './types.ts';
