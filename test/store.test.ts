import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Store } from '../src/db/store.ts';
import { HashEmbedder } from '../src/embed/fake.ts';
import { chunkText, toChunkInputs } from '../src/text/chunker.ts';
import { buildFtsQuery, sha256 } from '../src/text/normalize.ts';
import type { DocumentKind } from '../src/types.ts';

function putDoc(store: Store, uri: string, text: string, namespace = 'default', kind: DocumentKind = 'file') {
  const up = store.upsertDocument({ uri, kind, namespace, title: uri, contentHash: sha256(text), metadata: {} });
  const chunks = toChunkInputs(chunkText(text, { title: uri, minChars: 1 }));
  const rep = store.replaceChunks(up.id, chunks);
  return { ...up, chunks: chunks.length, reused: rep.reusedEmbeddings };
}

test('upsertDocument: creat → neschimbat → schimbat → reactivat', () => {
  const s = Store.open();
  const base = { uri: 'u1', kind: 'file' as const, namespace: 'default', title: 't', metadata: { a: 1 } };
  const a = s.upsertDocument({ ...base, contentHash: 'h1' });
  assert.equal(a.created, true);
  assert.equal(a.changed, true);
  const b = s.upsertDocument({ ...base, contentHash: 'h1' });
  assert.deepEqual([b.created, b.changed, b.id], [false, false, a.id]);
  const c = s.upsertDocument({ ...base, contentHash: 'h2' });
  assert.equal(c.changed, true);
  s.setDocumentActive(a.id, false);
  assert.equal(s.getDocument(a.id)?.active, false);
  const d = s.upsertDocument({ ...base, contentHash: 'h2' });
  assert.equal(d.changed, true, 'reactivarea cere refacerea fragmentelor');
  assert.equal(s.getDocument(a.id)?.active, true);
  assert.deepEqual(s.getDocument(a.id)?.metadata, { a: 1 });
  s.close();
});

test('replaceChunks păstrează vectorii fragmentelor identice', () => {
  const s = Store.open();
  const emb = new HashEmbedder(8);
  const doc = putDoc(s, 'u1', '# A\n\nunu\n\n# B\n\ndoi');
  assert.equal(doc.chunks, 2);
  const pending = s.pendingChunks(emb.id, 10);
  assert.equal(pending.length, 2);
  s.setEmbeddings(pending.map((p) => ({ chunkId: p.id, vector: emb.vectorFor(p.text) })), emb.id);
  assert.equal(s.countPending(emb.id), 0);

  const again = putDoc(s, 'u1', '# A\n\nunu\n\n# B\n\ndoi schimbat');
  assert.equal(again.changed, true);
  assert.equal(again.reused, 1);
  assert.equal(s.countPending(emb.id), 1);
  assert.equal(s.countPending('alt-model'), 2, 'alt model → tot ce există e pending');
  s.close();
});

test('FTS5 rămâne sincron cu chunks și pliază diacriticele', () => {
  const s = Store.open();
  putDoc(s, 'u1', 'Regula de aur: nu comite niciodată fișiere .env cu secrete.');
  putDoc(s, 'u2', 'Rolurile: managerul vede parolele profesorilor.');
  const q = buildFtsQuery('fisiere secrete');
  assert.ok(q);
  let hits = s.lexicalSearch(q, 10);
  assert.equal(hits.length, 1);
  assert.equal(s.getHits([hits[0]?.chunkId ?? -1]).get(hits[0]?.chunkId ?? -1)?.uri, 'u1');
  assert.ok((hits[0]?.score ?? 0) > 0, 'scorul e −bm25, deci pozitiv');

  assert.equal(s.lexicalSearch(buildFtsQuery('profesor') ?? '', 10).length, 1, 'prefixul „profesor"* prinde „profesorilor"');
  assert.equal(s.lexicalSearch(buildFtsQuery('PROFESORÍ') ?? '', 10).length, 1, 'majuscule și diacritice străine sunt pliate');

  putDoc(s, 'u1', 'Text complet diferit despre deploy.');
  hits = s.lexicalSearch(q, 10);
  assert.equal(hits.length, 0, 'textul vechi a dispărut din index');
  assert.equal(s.lexicalSearch(buildFtsQuery('deploy') ?? '', 10).length, 1);

  s.setDocumentActive(s.getDocumentByUri('u2')?.id ?? -1, false);
  assert.equal(s.lexicalSearch(buildFtsQuery('profesor') ?? '', 10).length, 0, 'documentele inactive nu apar');
  s.close();
});

test('filtre: spațiu, tip, etichete, expirare, inactive', () => {
  const s = Store.open();
  const now = 1_000_000_000_000;
  const u1 = putDoc(s, 'u1', 'alfa', 'a');
  const u2 = putDoc(s, 'u2', 'beta', 'b');
  const m1 = putDoc(s, 'memory://a/1', 'memorie expirată', 'a', 'memory');
  const m2 = putDoc(s, 'memory://a/2', 'memorie vie', 'a', 'memory');
  s.insertMemory({ documentId: m1.id, memoryType: 'fact', importance: 0.5, tags: ['x'], supersedes: null, expiresAt: now - 1 });
  s.insertMemory({ documentId: m2.id, memoryType: 'decision', importance: 0.9, tags: ['y', 'z'], supersedes: null, expiresAt: null });

  assert.deepEqual([...s.allowedDocumentIds({ namespace: 'a', kinds: ['file'], now })], [u1.id]);
  assert.deepEqual([...s.allowedDocumentIds({ namespace: ['a', 'b'], kinds: ['file'], now })].sort(), [u1.id, u2.id].sort());
  assert.deepEqual([...s.allowedDocumentIds({ kinds: ['memory'], now })], [m2.id], 'memoria expirată e exclusă');
  assert.deepEqual([...s.allowedDocumentIds({ kinds: ['memory'], now: now - 10 })].sort(), [m1.id, m2.id].sort(), 'înainte de expirare, ambele');
  assert.deepEqual([...s.allowedDocumentIds({ tags: ['z'], now })], [m2.id]);
  assert.deepEqual([...s.allowedDocumentIds({ memoryTypes: ['fact'], now, includeInactive: true })], [m1.id]);
  assert.equal(s.needsDocFilter({}), true, 'există memorii cu expirare → filtrul e necesar');
  assert.equal(s.listMemories({ namespace: 'a', now }).length, 1);
  assert.equal(s.listMemories({ namespace: 'a', now, includeInactive: true }).length, 2);
  assert.equal(s.listMemories({ namespace: 'a', now })[0]?.text, 'memorie vie');
  s.close();
});

test('loadVectors construiește o matrice contiguă doar din documentele active ale modelului', () => {
  const s = Store.open();
  const emb = new HashEmbedder(4);
  putDoc(s, 'u1', '# A\n\nunu\n\n# B\n\ndoi');
  putDoc(s, 'u2', 'trei');
  const pending = s.pendingChunks(emb.id, 10);
  s.setEmbeddings(pending.map((p) => ({ chunkId: p.id, vector: emb.vectorFor(p.text) })), emb.id);

  const rows = s.loadVectors(emb.id);
  assert.equal(rows.count, 3);
  assert.equal(rows.dims, 4);
  assert.equal(rows.matrix.length, 12);
  assert.deepEqual([...rows.ids], pending.map((p) => p.id));
  const first = emb.vectorFor(pending[0]?.text ?? '');
  for (let i = 0; i < 4; i++) assert.ok(Math.abs((rows.matrix[i] ?? 0) - (first[i] ?? 0)) < 1e-7);

  assert.equal(s.loadVectors('alt-model').count, 0);
  s.setDocumentActive(s.getDocumentByUri('u1')?.id ?? -1, false);
  assert.equal(s.loadVectors(emb.id).count, 1);
  s.close();
});

test('cache-ul de vectori: put/get pe loturi', () => {
  const s = Store.open();
  const entries = Array.from({ length: 900 }, (_, i) => ({ key: `k${i}`, vector: new Float32Array([i, i + 1]) }));
  s.cachePut(entries, 'm');
  const got = s.cacheGet(entries.map((e) => e.key).concat(['lipsa']));
  assert.equal(got.size, 900);
  assert.deepEqual([...(got.get('k899') ?? [])], [899, 900]);
  assert.equal(s.stats().cache.entries, 900);
  s.close();
});

test('supersede și memoryChain', () => {
  const s = Store.open();
  const a = putDoc(s, 'memory://d/a', 'folosim Neon', 'd', 'memory');
  const b = putDoc(s, 'memory://d/b', 'am trecut pe Postgres pe VPS', 'd', 'memory');
  s.insertMemory({ documentId: a.id, memoryType: 'decision', importance: 0.5, tags: [], supersedes: null, expiresAt: null });
  s.insertMemory({ documentId: b.id, memoryType: 'decision', importance: 0.8, tags: [], supersedes: a.id, expiresAt: null });
  s.supersede(a.id, b.id);
  assert.equal(s.getDocument(a.id)?.active, false);
  assert.equal(s.getMemory(a.id)?.supersededBy, b.id);
  assert.equal(s.getMemory(b.id)?.supersedes, a.id);
  assert.deepEqual(s.memoryChain(a.id).map((m) => m.documentId), [a.id, b.id]);
  assert.deepEqual(s.memoryChain(b.id).map((m) => m.documentId), [a.id, b.id]);
  const st = s.stats();
  assert.equal(st.memories.active, 1);
  assert.equal(st.memories.total, 2);
  s.close();
});
