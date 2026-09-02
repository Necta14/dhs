import assert from 'node:assert/strict';
import { test } from 'node:test';
import { chunkText, splitLong, toChunkInputs } from '../src/text/chunker.ts';

test('calea de titluri urmează structura Markdown', () => {
  const md = '# Titlu\n\nIntro.\n\n## Secțiunea A\n\nText A.\n\n### Sub A1\n\nText A1.\n\n## Secțiunea B\n\nText B.';
  const chunks = chunkText(md, { minChars: 1, maxChars: 200 });
  assert.deepEqual(
    chunks.map((c) => c.headingPath),
    ['Titlu', 'Titlu > Secțiunea A', 'Titlu > Secțiunea A > Sub A1', 'Titlu > Secțiunea B'],
  );
  assert.ok(chunks[1]?.text.startsWith('## Secțiunea A'), 'titlul rămâne în textul fragmentului');
  assert.ok(chunks[0]?.text.includes('Intro.'));
});

test('titlul documentului devine rădăcina căii', () => {
  const chunks = chunkText('## Doar H2\n\ntext', { title: 'Fișier' });
  assert.equal(chunks[0]?.headingPath, 'Fișier > Doar H2');
});

test('blocurile de cod rămân atomice, cu liniile goale din interior', () => {
  const md = '# T\n\n```js\nconst a = 1;\n\nconst b = 2;\n```\n\nDupă.';
  const chunks = chunkText(md, { minChars: 1, maxChars: 500 });
  assert.equal(chunks.length, 1);
  assert.ok(chunks[0]?.text.includes('const a = 1;\n\nconst b = 2;'));
  assert.ok(chunks[0]?.text.endsWith('După.'));
});

test('secțiunile mici se lipesc până la minChars, cele mari se separă curat', () => {
  const md = '# A\n\nscurt\n\n# B\n\nscurt\n\n# C\n\n' + 'lung '.repeat(120);
  const chunks = chunkText(md, { minChars: 200, maxChars: 2000 });
  assert.equal(chunks.length, 1, 'totul intră într-un fragment când e sub maxChars');
  const chunks2 = chunkText(md, { minChars: 10, maxChars: 2000 });
  assert.equal(chunks2.length, 3, 'cu minChars mic, fiecare secțiune e separată');
});

test('paragrafele lungi se taie sub maxChars, cu suprapunere și ordinale consecutive', () => {
  const sentence = 'Aceasta este o propoziție suficient de lungă pentru testul de fragmentare. ';
  const md = `# Lung\n\n${sentence.repeat(60)}`;
  const chunks = chunkText(md, { maxChars: 800, overlapChars: 100 });
  assert.ok(chunks.length >= 4, `aștept ≥4 fragmente, am ${chunks.length}`);
  chunks.forEach((c, i) => {
    assert.equal(c.ordinal, i);
    assert.ok(c.text.length <= 800, `fragmentul ${i} are ${c.text.length} caractere`);
    assert.equal(c.headingPath, 'Lung');
  });
  const lastWords = chunks[0]?.text.trim().split(' ').slice(-3).join(' ') ?? '';
  assert.ok(chunks[1]?.text.startsWith(lastWords.split(' ')[0] ?? '') || chunks[1]?.text.includes(lastWords), 'fragmentul 2 începe cu coada fragmentului 1');
});

test('modul text simplu ignoră titlurile Markdown', () => {
  const chunks = chunkText('# nu e titlu\n\ntext', { markdown: false, title: 'F' });
  assert.equal(chunks.length, 1);
  assert.equal(chunks[0]?.headingPath, 'F');
});

test('splitLong: linii → propoziții → cuvinte → tăiere dură', () => {
  assert.deepEqual(splitLong('x'.repeat(2500), 1000).map((p) => p.length), [1000, 1000, 500]);
  const words = splitLong(Array.from({ length: 50 }, (_, i) => `cuvant${i}`).join(' '), 60);
  assert.ok(words.every((w) => w.length <= 60));
  assert.equal(words.join(' ').split(' ').length, 50, 'niciun cuvânt pierdut');
  const lines = splitLong(Array.from({ length: 20 }, (_, i) => `linia ${i}`).join('\n'), 40);
  assert.ok(lines.every((l) => l.length <= 40));
  assert.ok(lines.every((l) => !l.startsWith('\n')));
});

test('toChunkInputs: hash-ul depinde de calea de titluri și de text', () => {
  const a = toChunkInputs([{ ordinal: 0, headingPath: 'A', text: 'x' }])[0];
  const b = toChunkInputs([{ ordinal: 0, headingPath: 'B', text: 'x' }])[0];
  const c = toChunkInputs([{ ordinal: 5, headingPath: 'A', text: 'x' }])[0];
  assert.notEqual(a?.contentHash, b?.contentHash);
  assert.equal(a?.contentHash, c?.contentHash, 'ordinalul nu intră în hash');
  assert.ok((a?.tokenEstimate ?? 0) >= 1);
});
