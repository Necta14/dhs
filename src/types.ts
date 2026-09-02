/** Tipuri publice ale suitei. Fără logică — doar contracte. */

export type DocumentKind = 'file' | 'memory' | 'note';
export const DOCUMENT_KINDS: readonly DocumentKind[] = ['file', 'memory', 'note'];

export type MemoryType = 'fact' | 'decision' | 'preference' | 'episode' | 'task' | 'problem' | 'handoff';
export const MEMORY_TYPES: readonly MemoryType[] = [
  'fact', 'decision', 'preference', 'episode', 'task', 'problem', 'handoff',
];

export type Metadata = Record<string, unknown>;

export interface DocumentInput {
  uri: string;
  kind: DocumentKind;
  namespace: string;
  title: string | null;
  contentHash: string;
  metadata: Metadata;
}

export interface DocumentRecord extends DocumentInput {
  id: number;
  createdAt: number;
  updatedAt: number;
  active: boolean;
}

export interface ChunkInput {
  ordinal: number;
  headingPath: string | null;
  text: string;
  contentHash: string;
  tokenEstimate: number;
}

export interface PendingChunk {
  id: number;
  documentId: number;
  headingPath: string | null;
  text: string;
  title: string | null;
}

export interface MemoryRecord {
  documentId: number;
  memoryType: MemoryType;
  importance: number;
  tags: string[];
  supersedes: number | null;
  supersededBy: number | null;
  lastAccessedAt: number | null;
  accessCount: number;
  expiresAt: number | null;
}

export interface MemoryListItem extends MemoryRecord {
  uri: string;
  namespace: string;
  title: string | null;
  text: string;
  createdAt: number;
  updatedAt: number;
  active: boolean;
  metadata: Metadata;
}

/** Filtre aplicabile la nivel de document, comune căutării și listărilor. */
export interface DocFilter {
  namespace?: string | readonly string[] | undefined;
  kinds?: readonly DocumentKind[] | undefined;
  uriPrefix?: string | undefined;
  documentIds?: readonly number[] | undefined;
  memoryTypes?: readonly MemoryType[] | undefined;
  /** Oricare dintre etichete (OR). */
  tags?: readonly string[] | undefined;
  includeInactive?: boolean | undefined;
  /** Momentul „acum" pentru expirări (ms). Implicit Date.now(). */
  now?: number | undefined;
}

export type SearchMode = 'hybrid' | 'vector' | 'lexical';

export interface SearchOptions extends DocFilter {
  limit?: number | undefined;
  mode?: SearchMode | undefined;
  /** Câți candidați cere fiecare retriever înainte de fuziune. Implicit max(limit·4, 40). */
  candidates?: number | undefined;
  /** Constanta k din Reciprocal Rank Fusion. Implicit 60. */
  rrfK?: number | undefined;
  vectorWeight?: number | undefined;
  lexicalWeight?: number | undefined;
  /** 0..1 — activează re-ordonarea MMR (diversitate). Nedefinit = dezactivat. */
  mmrLambda?: number | undefined;
}

export interface SearchHit {
  chunkId: number;
  documentId: number;
  uri: string;
  kind: DocumentKind;
  namespace: string;
  title: string | null;
  headingPath: string | null;
  text: string;
  /** Scorul folosit la ordonare (RRF în hibrid, cosinus în vector, −BM25 în lexical). */
  score: number;
  vectorRank: number | null;
  lexicalRank: number | null;
  vectorScore: number | null;
  lexicalScore: number | null;
  metadata: Metadata;
  updatedAt: number;
}

export interface RecallOptions extends SearchOptions {
  /** Alias prietenos pentru memoryTypes. */
  types?: readonly MemoryType[] | undefined;
  /** Timpul de înjumătățire al prospețimii, în zile. Implicit 30. */
  halfLifeDays?: number | undefined;
  /** 0..1 — cât de mult penalizează vechimea. Implicit 0,3. */
  recencyWeight?: number | undefined;
  /** Cât de mult bonifică importanța (×(1 + w·importanță)). Implicit 0,5. */
  importanceWeight?: number | undefined;
  /** Actualizează last_accessed/access_count. Implicit true. */
  touch?: boolean | undefined;
}

export interface MemoryHit extends SearchHit {
  memory: MemoryRecord;
  /** Relevanța pură, normalizată 0..1 în cadrul setului. */
  relevance: number;
  /** Relevanță × importanță × prospețime — ordinea finală. */
  finalScore: number;
}

export interface RememberInput {
  text: string;
  type?: MemoryType | undefined;
  namespace?: string | undefined;
  title?: string | undefined;
  tags?: readonly string[] | undefined;
  /** 0..1, implicit 0,5. */
  importance?: number | undefined;
  /** ID-ul memoriei pe care o înlocuiește (aceea devine inactivă, istoricul rămâne). */
  supersedes?: number | undefined;
  /** Timestamp ms după care memoria nu mai e returnată. */
  expiresAt?: number | null | undefined;
  metadata?: Metadata | undefined;
}

export interface RememberResult {
  id: number;
  uri: string;
  chunks: number;
  embedded: boolean;
  superseded: number | null;
}

export interface StoreStats {
  dbPath: string;
  dbBytes: number;
  documents: { total: number; active: number; byKind: Record<string, number>; byNamespace: Record<string, number> };
  chunks: { total: number; embedded: number; pending: number; models: string[] };
  memories: { total: number; active: number; byType: Record<string, number> };
  cache: { entries: number };
}
