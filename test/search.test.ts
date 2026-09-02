import assert from 'node:assert/strict';
import { test } from 'node:test';
import { l2normalize } from '../src/embed/embedder.ts';
import { dot, mmrSelect, rrfFuse } from '../src/search/hybrid.ts';
import { TopK, VectorIndex } from '../src/search/vector.ts';

test('rrfFuse premiază elementele prezente în ambele liste', () => {
  const fused = rrfFuse([{ ids: [1, 2, 3] }, { ids: [3, 1, 4] }], 60);
  assert.ok((fused.get(1)?.score ?? 0) > (fused.get(2)?.score ?? 0));
  assert.ok((fused.get(3)?.score ?? 0) > (fused.get(4)?.score ?? 0));
  assert.deepEqual(fused.get(1)?.ranks, [1, 2]);
  assert.deepEqual(fused.get(2)?.ranks, [2, null]);
  assert.equal(fused.get(1)?.score, 1 / 61 + 1 / 62);
});

test('rrfFuse respectă ponderile', () => {
  const fused = rrfFuse([{ ids: [1], weight: 2 }, { ids: [2], weight: 1 }], 60);
  assert.ok((fused.get(1)?.score ?? 0) > (fused.get(2)?.score ?? 0));
});

test('TopK păstrează cele mai mari k scoruri, descrescător', () => {
  const top = new TopK(3);
  [5, 1, 9, 3, 7, 2].forEach((s, i) => top.offer(s, i));
  assert.deepEqual(top.drain().map((h) => h.score), [9, 7, 5]);
  assert.deepEqual(top.drain().map((h) => h.chunkId), [2, 4, 0]);
  assert.equal(top.threshold, 5);
});

function randomUnit(dims: number, seed: number): Float32Array {
  const v = new Float32Array(dims);
  let x = seed || 1;
  for (let i = 0; i < dims; i++) {
    x = (x * 1103515245 + 12345) >>> 0;
    v[i] = (x / 2 ** 32) * 2 - 1;
  }
  return l2normalize(v);
}

test('VectorIndex.search coincide cu sortarea brută, inclusiv cu filtru', () => {
  const dims = 16;
  const count = 300;
  const matrix = new Float32Array(count * dims);
  const ids = new Int32Array(count);
  const docIds = new Int32Array(count);
  for (let i = 0; i < count; i++) {
    matrix.set(randomUnit(dims, i + 7), i * dims);
    ids[i] = 1000 + i;
    docIds[i] = i % 10;
  }
  const index = new VectorIndex({ ids, docIds, matrix, dims, count });
  const q = randomUnit(dims, 999);
  const brute = Array.from({ length: count }, (_, i) => ({ id: 1000 + i, doc: i % 10, score: dot(q, matrix.subarray(i * dims, (i + 1) * dims)) }))
    .sort((a, b) => b.score - a.score);

  const hits = index.search(q, 5);
  assert.deepEqual(hits.map((h) => h.chunkId), brute.slice(0, 5).map((b) => b.id));
  hits.forEach((h, i) => assert.ok(Math.abs(h.score - (brute[i]?.score ?? 0)) < 1e-6));

  const filtered = index.search(q, 5, (_c, doc) => doc === 3);
  assert.deepEqual(filtered.map((h) => h.chunkId), brute.filter((b) => b.doc === 3).slice(0, 5).map((b) => b.id));
  assert.equal(index.vectorOf(1042)?.length, dims);
  assert.equal(index.vectorOf(1), undefined);
});

test('VectorIndex refuză interogări cu altă dimensiune și tolerează indexul gol', () => {
  assert.deepEqual(VectorIndex.empty().search(new Float32Array(4), 3), []);
  const index = new VectorIndex({ ids: new Int32Array([1]), docIds: new Int32Array([1]), matrix: new Float32Array([1, 0]), dims: 2, count: 1 });
  assert.throws(() => index.search(new Float32Array(3), 1), /dimensiuni/);
});

test('mmrSelect preferă diversitatea față de duplicatele apropiate', () => {
  const vectors = new Map<number, Float32Array>([
    [1, l2normalize(new Float32Array([1, 0, 0]))],
    [2, l2normalize(new Float32Array([0.98, 0.2, 0]))],
    [3, l2normalize(new Float32Array([0, 0, 1]))],
  ]);
  const picked = mmrSelect([{ id: 1, score: 1 }, { id: 2, score: 0.9 }, { id: 3, score: 0.5 }], (id) => vectors.get(id), 0.5, 2);
  assert.deepEqual(picked.map((p) => p.id), [1, 3]);
  const relevanceOnly = mmrSelect([{ id: 1, score: 1 }, { id: 2, score: 0.9 }, { id: 3, score: 0.5 }], (id) => vectors.get(id), 1, 2);
  assert.deepEqual(relevanceOnly.map((p) => p.id), [1, 2]);
});
