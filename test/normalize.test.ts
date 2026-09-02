import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildFtsQuery, contentTokens, sha256, tokenize } from '../src/text/normalize.ts';

test('tokenize pliază diacriticele și trece la litere mici', () => {
  assert.deepEqual(tokenize('Școala Ș.T. ține 2 ore'), ['scoala', 's', 't', 'tine', '2', 'ore']);
  assert.deepEqual(tokenize('ă î â ș ț Ă Î Â Ș Ț'), ['a', 'i', 'a', 's', 't', 'a', 'i', 'a', 's', 't']);
});

test('contentTokens elimină stopwords RO/EN și fragmentele de un caracter', () => {
  assert.deepEqual(contentTokens('Cum tratăm secretele în the .env?'), ['tratam', 'secretele', 'env']);
});

test('buildFtsQuery citează tokenii, pune prefix la cei lungi, sare stopwords', () => {
  assert.equal(buildFtsQuery('Cum tratăm secretele în .env?'), '"tratam"* "secretele"* "env"');
  assert.equal(buildFtsQuery('Cum tratăm secretele în .env?', { operator: 'OR' }), '"tratam"* OR "secretele"* OR "env"');
});

test('buildFtsQuery revine la toți tokenii când totul e stopword', () => {
  assert.equal(buildFtsQuery('cum să'), '"cum" "sa"');
  assert.equal(buildFtsQuery('   '), null);
});

test('buildFtsQuery neutralizează operatorii FTS și duplicatele', () => {
  assert.equal(buildFtsQuery('a OR b NOT "c" c'), '"a" "or" "b" "not" "c"');
  assert.equal(buildFtsQuery('deploy deploy DEPLOY'), '"deploy"*');
});

test('sha256 e prefixat cu lungimea (fără coliziuni de concatenare) și stabil', () => {
  assert.notEqual(sha256('ab', 'c'), sha256('a', 'bc'));
  assert.notEqual(sha256('', 'x'), sha256('x', ''));
  assert.equal(sha256('x'), sha256('x'));
  assert.match(sha256('x'), /^[0-9a-f]{64}$/);
});
