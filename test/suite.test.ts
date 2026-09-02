import assert from 'node:assert/strict';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { HashEmbedder } from '../src/embed/fake.ts';
import { Suite } from '../src/index.ts';
import { isForbiddenFile } from '../src/ingest/files.ts';

async function corpus(): Promise<string> {
  const dir = await mkdtemp(join(tmpdir(), 'dhs-'));
  await writeFile(join(dir, 'secrete.md'), '# Secrete\n\nRegula de aur despre secrete: fișierele .env nu se citesc și nu se comit. Secretele vin din process.env.\n');
  await writeFile(join(dir, 'roluri.md'), '# Roluri\n\nManagerul vede parolele profesorilor și elevilor, dar nu pe ale adminului.\n');
  await writeFile(join(dir, 'deploy.md'), '# Deploy\n\nProducția rulează pe VPS cu Postgres și MinIO; publicarea se face cu infra/deploy.sh.\n');
  await mkdir(join(dir, 'node_modules', 'x'), { recursive: true });
  await writeFile(join(dir, 'node_modules', 'x', 'README.md'), '# ignorat\n\nsecrete secrete secrete\n');
  await writeFile(join(dir, '.env.md'), '# capcană\n\nGEMINI_API_KEY=nu-ma-indexa\n');
  await writeFile(join(dir, 'poza.md'), Buffer.from([0x23, 0x20, 0x00, 0x01, 0x02]));
  return dir;
}

test('fișierele de secrete sunt refuzate după nume', () => {
  for (const name of ['.env', '.env.local', '.env.production', 'server.pem', 'id_rsa', 'id_ed25519.pub', 'gcp-credentials.json', 'secrets.yaml']) {
    assert.equal(isForbiddenFile(name), true, name);
  }
  for (const name of ['README.md', 'env.md', 'environment.txt', 'notes.txt']) assert.equal(isForbiddenFile(name), false, name);
});

test('index → search (hibrid/lexical/vector) → reindexare incrementală → prune', async () => {
  const dir = await corpus();
  const embedder = new HashEmbedder(256); // destule coșuri ca „sacul de cuvinte" hash-uit să nu se ciocnească
  const suite = Suite.open({ embedder });
  try {
    const r1 = await suite.index([dir], { namespace: 'test' });
    assert.equal(r1.added, 3, 'trei documente reale; node_modules și .env.md nu sunt nici măcar deschise, binarul e sărit');
    assert.equal(r1.skipped, 1);
    assert.equal(r1.scanned, 4);
    assert.ok(suite.store.listDocuments({ includeInactive: true }).every((d) => !d.uri.endsWith('.env.md') && !d.uri.includes('node_modules')));
    assert.equal(r1.embed?.fromApi, r1.chunks);
    assert.equal(embedder.items, r1.chunks);

    const hybrid = await suite.search('cum protejăm secretele din fișierele .env', { namespace: 'test' });
    assert.ok(hybrid.length >= 1);
    assert.ok(hybrid[0]?.uri.endsWith('secrete.md'), `primul rezultat: ${hybrid[0]?.uri}`);
    assert.equal(hybrid[0]?.kind, 'file');
    assert.ok(hybrid[0]?.vectorRank !== null && hybrid[0]?.lexicalRank !== null, 'găsit de ambele motoare');

    const lexical = await suite.search('parolele profesorilor', { namespace: 'test', mode: 'lexical' });
    assert.ok(lexical[0]?.uri.endsWith('roluri.md'));
    assert.equal(lexical[0]?.vectorRank, null);

    const vector = await suite.search('VPS MinIO deploy infra', { namespace: 'test', mode: 'vector' });
    assert.ok(vector[0]?.uri.endsWith('deploy.md'), `vector: ${vector[0]?.uri}`);
    assert.equal(vector[0]?.lexicalRank, null);
    assert.ok((vector[0]?.vectorScore ?? 0) > (vector[1]?.vectorScore ?? 0));

    assert.deepEqual(await suite.search('secrete', { namespace: 'altul' }), [], 'alt spațiu → nimic');
    assert.deepEqual(await suite.search('   ', { namespace: 'test' }), []);

    const itemsBeforeReindex = embedder.items; // include și cele 3 interogări vectorizate mai sus
    const r2 = await suite.index([dir], { namespace: 'test' });
    assert.equal(r2.unchanged, 3);
    assert.equal(r2.embed?.done, 0, 'nimic de vectorizat a doua oară');
    assert.equal(r2.embed?.fromApi, 0);
    assert.equal(embedder.items, itemsBeforeReindex, 'embedder-ul nu a mai fost chemat pentru documente');

    await writeFile(join(dir, 'roluri.md'), '# Roluri\n\nManagerul vede parolele profesorilor și elevilor, dar nu pe ale adminului.\n\nAdminul e universal.\n');
    await rm(join(dir, 'deploy.md'));
    const r3 = await suite.index([dir], { namespace: 'test' });
    assert.equal(r3.updated, 1);
    assert.equal(r3.pruned, 1);
    assert.deepEqual(await suite.search('VPS MinIO deploy infra', { namespace: 'test', mode: 'lexical' }), [], 'documentul dezactivat nu mai apare');
    assert.equal((await suite.search('admin universal', { namespace: 'test' }))[0]?.uri.endsWith('roluri.md'), true);

    const stats = suite.stats();
    assert.equal(stats.documents.active, 2);
    assert.equal(stats.documents.total, 3);
    assert.equal(stats.chunks.pending, 0);
  } finally {
    suite.close();
    await rm(dir, { recursive: true, force: true });
  }
});

test('embedPending folosește cache-ul când vectorii lipsesc din chunks', async () => {
  const dir = await corpus();
  const embedder = new HashEmbedder(32);
  const suite = Suite.open({ embedder });
  try {
    const r = await suite.index([dir]);
    const before = embedder.items;
    suite.store.db.exec('UPDATE chunks SET embedding = NULL, embedding_model = NULL');
    assert.equal(suite.store.countPending(embedder.id), r.chunks);
    const p = await suite.embedPending();
    assert.equal(p.fromCache, r.chunks);
    assert.equal(p.fromApi, 0);
    assert.equal(embedder.items, before, 'zero apeluri noi');
  } finally {
    suite.close();
    await rm(dir, { recursive: true, force: true });
  }
});

test('remember / recall / supersede / history / forget / handoff', async () => {
  const suite = Suite.open({ embedder: new HashEmbedder(64) });
  try {
    const a = await suite.remember({ text: 'Folosim Neon pentru baza de date.', type: 'decision', namespace: 'atm', tags: ['db'], importance: 0.6 });
    const b = await suite.remember({ text: 'Am migrat baza de date pe Postgres 17, pe VPS-ul propriu.', type: 'decision', namespace: 'atm', tags: ['db', 'infra'], importance: 0.9, supersedes: a.id });
    const c = await suite.remember({ text: 'Userul preferă răspunsuri scurte, concluzia la început.', type: 'preference', namespace: 'atm', importance: 0.8 });
    await suite.remember({ text: 'Memorie din alt proiect despre baza de date.', type: 'fact', namespace: 'altul' });
    assert.equal(b.superseded, a.id);
    assert.equal(a.embedded, true);

    const hits = await suite.recall('unde e baza de date', { namespace: 'atm' });
    assert.ok(hits.length >= 1);
    assert.equal(hits[0]?.documentId, b.id, 'memoria nouă câștigă, cea înlocuită nu apare');
    assert.ok(hits.every((h) => h.documentId !== a.id));
    assert.ok(hits.every((h) => h.namespace === 'atm'));
    assert.equal(hits[0]?.memory.memoryType, 'decision');
    assert.equal(suite.store.getMemory(b.id)?.accessCount, 1, 'recall marchează accesul');

    const byTag = await suite.recall('baza de date', { namespace: 'atm', tags: ['infra'] });
    assert.deepEqual(byTag.map((h) => h.documentId), [b.id]);
    const byType = await suite.recall('răspunsuri', { namespace: 'atm', types: ['preference'] });
    assert.deepEqual(byType.map((h) => h.documentId), [c.id]);

    const chain = suite.history(a.id);
    assert.deepEqual(chain.map((m) => [m.documentId, m.active]), [[a.id, false], [b.id, true]]);

    const handoff = await suite.handoff({ namespace: 'atm' });
    assert.ok(handoff.markdown.startsWith('# Handoff · atm'));
    assert.ok(handoff.markdown.includes('## Decizii'));
    assert.ok(handoff.markdown.includes('## Preferințe'));
    assert.ok(handoff.markdown.includes(`#${b.id}`));
    assert.ok(!handoff.markdown.includes('Neon'), 'memoria înlocuită nu intră în handoff');
    assert.equal(handoff.items.length, 2);

    const focused = await suite.handoff({ namespace: 'atm', query: 'baza de date' });
    assert.equal(focused.items[0]?.id, b.id);

    assert.equal(suite.forget(b.id), true);
    assert.equal(suite.forget(b.id), false, 'a doua dată nu mai e activă');
    assert.equal(suite.forget(999_999), false);
    const after = await suite.recall('unde e baza de date', { namespace: 'atm' });
    assert.ok(after.every((h) => h.documentId !== b.id));

    await assert.rejects(suite.remember({ text: '   ' }), /gol/);
    await assert.rejects(suite.remember({ text: 'x', supersedes: 424242 }), /nu există/);
  } finally {
    suite.close();
  }
});

test('memoriile expirate nu mai sunt returnate; scorul ține cont de importanță și prospețime', async () => {
  const suite = Suite.open({ embedder: new HashEmbedder(64) });
  try {
    const now = Date.now();
    await suite.remember({ text: 'Token temporar pentru deploy, expiră azi.', type: 'fact', expiresAt: now - 1000 });
    const kept = await suite.remember({ text: 'Token permanent pentru deploy.', type: 'fact', importance: 0.9 });
    const hits = await suite.recall('token deploy', { now });
    assert.deepEqual(hits.map((h) => h.documentId), [kept.id]);

    const old = await suite.remember({ text: 'Notă veche despre cache-ul de embeddings.', type: 'fact', importance: 0.5 });
    suite.store.db.prepare('UPDATE documents SET updated_at = ? WHERE id = ?').run(now - 365 * 86_400_000, old.id);
    const fresh = await suite.remember({ text: 'Notă nouă despre cache-ul de embeddings.', type: 'fact', importance: 0.5 });
    const ranked = await suite.recall('cache embeddings', { now, recencyWeight: 0.9, halfLifeDays: 30 });
    assert.equal(ranked[0]?.documentId, fresh.id, 'la relevanță egală, prospețimea decide');
    assert.ok((ranked[0]?.finalScore ?? 0) > (ranked[1]?.finalScore ?? 0));
  } finally {
    suite.close();
  }
});

test('fără embedder: hibrid cade pe lexical, vector aruncă eroare clară', async () => {
  const dir = await corpus();
  const suite = Suite.open();
  try {
    const r = await suite.index([dir]);
    assert.equal(r.embed, null);
    const hits = await suite.search('parolele profesorilor');
    assert.ok(hits[0]?.uri.endsWith('roluri.md'));
    await assert.rejects(suite.search('x', { mode: 'vector' }), /embedder/);
    const m = await suite.remember({ text: 'fără vector' });
    assert.equal(m.embedded, false);
  } finally {
    suite.close();
    await rm(dir, { recursive: true, force: true });
  }
});
