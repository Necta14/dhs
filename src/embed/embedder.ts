export type EmbedTask = 'document' | 'query';

export interface EmbedItem {
  text: string;
  /** Titlu/context pentru documente (RETRIEVAL_DOCUMENT); ignorat la interogări. */
  title?: string | undefined;
}

export interface Embedder {
  /** Identitate stabilă model+dimensiune, ex. „gemini:gemini-embedding-2:768". Intră în cheia cache-ului. */
  readonly id: string;
  readonly dimensions: number;
  /** Vectori L2-normalizați, în aceeași ordine cu `items`. */
  embed(items: readonly EmbedItem[], task: EmbedTask): Promise<Float32Array[]>;
}

/** Normalizează în loc și întoarce același vector. Vectorul nul rămâne nul. */
export function l2normalize(v: Float32Array): Float32Array {
  let sum = 0;
  for (let i = 0; i < v.length; i++) sum += v[i]! * v[i]!;
  if (sum === 0) return v;
  const inv = 1 / Math.sqrt(sum);
  for (let i = 0; i < v.length; i++) v[i] = v[i]! * inv;
  return v;
}

/** Float32 little-endian → BLOB (copie proprie, ca să nu ținem referințe la buffere mari). */
export function toBlob(v: Float32Array): Uint8Array {
  const out = new Uint8Array(v.byteLength);
  out.set(new Uint8Array(v.buffer, v.byteOffset, v.byteLength));
  return out;
}

/** BLOB → Float32Array aliniat (copiază; BLOB-urile din SQLite nu sunt garantat aliniate la 4). */
export function fromBlob(u8: Uint8Array): Float32Array {
  const out = new Float32Array(u8.byteLength >>> 2);
  new Uint8Array(out.buffer).set(u8.subarray(0, out.byteLength));
  return out;
}
