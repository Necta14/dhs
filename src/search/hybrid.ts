export interface RankedList {
  /** Id-uri în ordinea relevanței (primul = cel mai bun). */
  ids: readonly number[];
  weight?: number | undefined;
}

export interface FusedScore {
  score: number;
  /** Poziția (1-based) în fiecare listă, sau null dacă lipsește din ea. */
  ranks: (number | null)[];
}

/** Reciprocal Rank Fusion: Σ w / (k + rang). Robust la scale diferite (cosinus vs BM25). */
export function rrfFuse(lists: readonly RankedList[], k = 60): Map<number, FusedScore> {
  const out = new Map<number, FusedScore>();
  lists.forEach((list, li) => {
    const w = list.weight ?? 1;
    list.ids.forEach((id, idx) => {
      let entry = out.get(id);
      if (!entry) {
        entry = { score: 0, ranks: lists.map(() => null) };
        out.set(id, entry);
      }
      entry.score += w / (k + idx + 1);
      entry.ranks[li] = idx + 1;
    });
  });
  return out;
}

export function dot(a: Float32Array, b: Float32Array): number {
  const n = Math.min(a.length, b.length);
  let s = 0;
  for (let i = 0; i < n; i++) s += a[i]! * b[i]!;
  return s;
}

export interface Scored {
  id: number;
  score: number;
}

/**
 * Maximal Marginal Relevance: alege iterativ candidatul care maximizează
 * λ·relevanță − (1−λ)·(similaritate maximă cu ce s-a ales deja). Candidații fără vector
 * (găsiți doar lexical) contează ca perfect diverși.
 */
export function mmrSelect(
  candidates: readonly Scored[],
  vectorOf: (id: number) => Float32Array | undefined,
  lambda: number,
  k: number,
): Scored[] {
  if (candidates.length === 0 || k <= 0) return [];
  const l = Math.min(1, Math.max(0, lambda));
  const max = candidates[0]?.score ?? 0;
  const min = candidates[candidates.length - 1]?.score ?? 0;
  const span = max - min || 1;
  const relevance = candidates.map((c) => (c.score - min) / span);
  const vectors = candidates.map((c) => vectorOf(c.id));
  const chosen: number[] = [];
  const used = new Set<number>();
  const target = Math.min(k, candidates.length);

  while (chosen.length < target) {
    let best = -1;
    let bestValue = Number.NEGATIVE_INFINITY;
    for (let i = 0; i < candidates.length; i++) {
      if (used.has(i)) continue;
      const v = vectors[i];
      let maxSim = 0;
      if (v) {
        for (const s of chosen) {
          const sv = vectors[s];
          if (sv) maxSim = Math.max(maxSim, dot(v, sv));
        }
      }
      const value = l * relevance[i]! - (1 - l) * maxSim;
      if (value > bestValue) {
        bestValue = value;
        best = i;
      }
    }
    if (best < 0) break;
    used.add(best);
    chosen.push(best);
  }
  return chosen.map((i) => candidates[i]!);
}
