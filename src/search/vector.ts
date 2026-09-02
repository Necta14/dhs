/** Rândurile încărcate din bază: o matrice contiguă rând-major, `count × dims`, plus id-urile. */
export interface VectorRows {
  ids: Int32Array;
  docIds: Int32Array;
  matrix: Float32Array;
  dims: number;
  count: number;
}

export interface VectorHit {
  chunkId: number;
  score: number;
}

export type AllowFn = (chunkId: number, documentId: number) => boolean;

/** Min-heap mărginit: păstrează cele mai mari k scoruri în O(n log k). */
export class TopK {
  readonly k: number;
  size = 0;
  readonly #scores: Float64Array;
  readonly #ids: Int32Array;

  constructor(k: number) {
    this.k = Math.max(1, Math.floor(k));
    this.#scores = new Float64Array(this.k);
    this.#ids = new Int32Array(this.k);
  }

  /** Scorul minim care mai intră; −∞ cât timp heap-ul nu e plin. */
  get threshold(): number {
    return this.size < this.k ? Number.NEGATIVE_INFINITY : this.#scores[0]!;
  }

  offer(score: number, id: number): void {
    if (this.size < this.k) {
      let i = this.size++;
      this.#scores[i] = score;
      this.#ids[i] = id;
      while (i > 0) {
        const p = (i - 1) >> 1;
        if (this.#scores[p]! <= this.#scores[i]!) break;
        this.#swap(i, p);
        i = p;
      }
      return;
    }
    if (score <= this.#scores[0]!) return;
    this.#scores[0] = score;
    this.#ids[0] = id;
    this.#siftDown(0);
  }

  #siftDown(start: number): void {
    let i = start;
    const n = this.size;
    for (;;) {
      const l = 2 * i + 1;
      const r = l + 1;
      let m = i;
      if (l < n && this.#scores[l]! < this.#scores[m]!) m = l;
      if (r < n && this.#scores[r]! < this.#scores[m]!) m = r;
      if (m === i) return;
      this.#swap(i, m);
      i = m;
    }
  }

  #swap(a: number, b: number): void {
    const s = this.#scores[a]!;
    this.#scores[a] = this.#scores[b]!;
    this.#scores[b] = s;
    const d = this.#ids[a]!;
    this.#ids[a] = this.#ids[b]!;
    this.#ids[b] = d;
  }

  /** Rezultatele, descrescător după scor. */
  drain(): VectorHit[] {
    const out: VectorHit[] = [];
    for (let i = 0; i < this.size; i++) out.push({ chunkId: this.#ids[i]!, score: this.#scores[i]! });
    out.sort((a, b) => b.score - a.score);
    return out;
  }
}

/**
 * Index vectorial în memorie: produs scalar exact peste o matrice Float32 contiguă (vectorii sunt
 * normalizați, deci produsul scalar = cosinus). La 100k fragmente × 768 dimensiuni o căutare durează
 * zeci de milisecunde, fără dependențe native. Reconstruit leneș când baza se schimbă.
 */
export class VectorIndex {
  readonly rows: VectorRows;
  readonly #rowOf = new Map<number, number>();

  constructor(rows: VectorRows) {
    this.rows = rows;
    for (let i = 0; i < rows.count; i++) this.#rowOf.set(rows.ids[i]!, i);
  }

  static empty(dims = 0): VectorIndex {
    return new VectorIndex({ ids: new Int32Array(0), docIds: new Int32Array(0), matrix: new Float32Array(0), dims, count: 0 });
  }

  get size(): number {
    return this.rows.count;
  }

  get dims(): number {
    return this.rows.dims;
  }

  vectorOf(chunkId: number): Float32Array | undefined {
    const r = this.#rowOf.get(chunkId);
    if (r === undefined) return undefined;
    const { matrix, dims } = this.rows;
    return matrix.subarray(r * dims, (r + 1) * dims);
  }

  search(query: Float32Array, k: number, allow?: AllowFn): VectorHit[] {
    const { matrix, dims, count, ids, docIds } = this.rows;
    if (count === 0 || k <= 0) return [];
    if (query.length !== dims) {
      throw new Error(`Interogarea are ${query.length} dimensiuni, indexul are ${dims}`);
    }
    const top = new TopK(Math.min(k, count));
    for (let r = 0; r < count; r++) {
      const id = ids[r]!;
      if (allow && !allow(id, docIds[r]!)) continue;
      const base = r * dims;
      let s0 = 0;
      let s1 = 0;
      let s2 = 0;
      let s3 = 0;
      let j = 0;
      for (; j + 3 < dims; j += 4) {
        s0 += matrix[base + j]! * query[j]!;
        s1 += matrix[base + j + 1]! * query[j + 1]!;
        s2 += matrix[base + j + 2]! * query[j + 2]!;
        s3 += matrix[base + j + 3]! * query[j + 3]!;
      }
      for (; j < dims; j++) s0 += matrix[base + j]! * query[j]!;
      top.offer(s0 + s1 + s2 + s3, id);
    }
    return top.drain();
  }
}
