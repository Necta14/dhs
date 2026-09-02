import type { Store } from '../db/store.ts';
import { sha256 } from '../text/normalize.ts';
import type { PendingChunk } from '../types.ts';
import type { EmbedItem, Embedder } from './embedder.ts';

export interface EmbedProgress {
  total: number;
  done: number;
  fromCache: number;
  fromApi: number;
}

export interface EmbedPendingOptions {
  pageSize?: number | undefined;
  onProgress?: ((p: EmbedProgress) => void) | undefined;
}

/** Textul trimis la embedding: calea de titluri + fragmentul. Titlul documentului merge separat. */
export function embedInputFor(chunk: Pick<PendingChunk, 'headingPath' | 'text' | 'title'>): EmbedItem {
  const text = chunk.headingPath ? `${chunk.headingPath}\n\n${chunk.text}` : chunk.text;
  const title = chunk.title ?? chunk.headingPath ?? undefined;
  return title ? { text, title } : { text };
}

export function cacheKeyFor(embedderId: string, item: EmbedItem): string {
  return sha256(embedderId, 'document', item.title ?? '', item.text);
}

/**
 * Vectorizează tot ce e în așteptare, în pagini: cache → API doar pentru lipsuri → o singură tranzacție
 * per pagină. Idempotent și reluabil: dacă procesul moare, fragmentele rămase sunt încă „pending".
 */
export async function embedPending(
  store: Store,
  embedder: Embedder,
  opts: EmbedPendingOptions = {},
): Promise<EmbedProgress> {
  const pageSize = Math.max(1, opts.pageSize ?? 200);
  const progress: EmbedProgress = { total: store.countPending(embedder.id), done: 0, fromCache: 0, fromApi: 0 };
  opts.onProgress?.(progress);
  let lastFirstId = -1;

  for (;;) {
    const page = store.pendingChunks(embedder.id, pageSize);
    if (page.length === 0) break;
    const firstId = page[0]?.id ?? -1;
    if (firstId === lastFirstId) throw new Error('embedPending: aceleași fragmente revin ca „pending" — scrierea nu s-a aplicat');
    lastFirstId = firstId;

    const items = page.map(embedInputFor);
    const keys = items.map((it) => cacheKeyFor(embedder.id, it));
    const cached = store.cacheGet(keys);
    const vectors: (Float32Array | undefined)[] = keys.map((k) => cached.get(k));
    const missing: number[] = [];
    for (let i = 0; i < vectors.length; i++) if (vectors[i] === undefined) missing.push(i);

    if (missing.length > 0) {
      const fresh = await embedder.embed(missing.map((i) => items[i]!), 'document');
      if (fresh.length !== missing.length) {
        throw new Error(`embedPending: embedder-ul a întors ${fresh.length} vectori pentru ${missing.length} texte`);
      }
      missing.forEach((i, j) => {
        const v = fresh[j]!;
        if (v.length !== embedder.dimensions) {
          throw new Error(`embedPending: vector cu ${v.length} dimensiuni, așteptam ${embedder.dimensions}`);
        }
        vectors[i] = v;
      });
    }

    store.transaction(() => {
      store.setEmbeddings(page.map((c, i) => ({ chunkId: c.id, vector: vectors[i]! })), embedder.id);
      store.cachePut(missing.map((i) => ({ key: keys[i]!, vector: vectors[i]! })), embedder.id);
    });

    progress.done += page.length;
    progress.fromCache += page.length - missing.length;
    progress.fromApi += missing.length;
    opts.onProgress?.(progress);
  }
  return progress;
}
