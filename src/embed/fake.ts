import { contentTokens } from '../text/normalize.ts';
import { l2normalize, type EmbedItem, type EmbedTask, type Embedder } from './embedder.ts';

/**
 * Embedder determinist, fără rețea: „sac de cuvinte" hash-uit în `dimensions` coșuri.
 * Texte cu vocabular comun ies apropiate — suficient pentru teste și pentru modul offline.
 */
export class HashEmbedder implements Embedder {
  readonly id: string;
  readonly dimensions: number;
  calls = 0;
  items = 0;

  constructor(dimensions = 64) {
    this.dimensions = dimensions;
    this.id = `hash:${dimensions}`;
  }

  async embed(items: readonly EmbedItem[], _task: EmbedTask): Promise<Float32Array[]> {
    this.calls++;
    this.items += items.length;
    return items.map((it) => this.vectorFor(it.title ? `${it.title}\n${it.text}` : it.text));
  }

  vectorFor(text: string): Float32Array {
    const v = new Float32Array(this.dimensions);
    for (const tok of contentTokens(text)) {
      let h = fnv1a(tok);
      for (let k = 0; k < 3; k++) {
        h = xorshift32(h);
        const idx = h % this.dimensions;
        v[idx] = (v[idx] ?? 0) + ((h & 0x8000) !== 0 ? 1 : -1);
      }
    }
    if (!v.some((x) => x !== 0)) v[0] = 1;
    return l2normalize(v);
  }
}

function fnv1a(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

function xorshift32(x: number): number {
  x ^= x << 13;
  x >>>= 0;
  x ^= x >>> 17;
  x ^= x << 5;
  return x >>> 0;
}
