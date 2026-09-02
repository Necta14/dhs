import assert from 'node:assert/strict';
import { test } from 'node:test';
import { GeminiApiError, GeminiEmbedder, parseRetryDelayMs } from '../src/embed/gemini.ts';
import { SlidingWindowLimiter } from '../src/embed/ratelimit.ts';

interface SeenRequest {
  model: string;
  taskType: string;
  outputDimensionality: number;
  title?: string;
  content: { parts: { text: string }[] };
}

function fakeFetch(plan: (call: number, body: { requests: SeenRequest[] }) => Response) {
  const seen: { url: string; headers: Record<string, string>; body: { requests: SeenRequest[] } }[] = [];
  const impl: typeof fetch = async (input, init) => {
    const body = JSON.parse(String(init?.body)) as { requests: SeenRequest[] };
    seen.push({ url: String(input), headers: (init?.headers as Record<string, string>) ?? {}, body });
    return plan(seen.length, body);
  };
  return { impl, seen };
}

function okResponse(body: { requests: SeenRequest[] }, dims: number): Response {
  return new Response(
    JSON.stringify({
      embeddings: body.requests.map((_r, i) => ({ values: Array.from({ length: dims }, (_, j) => (i + 1) * (j + 1)) })),
      usageMetadata: { totalTokenCount: body.requests.length * 5 },
    }),
    { status: 200, headers: { 'content-type': 'application/json' } },
  );
}

test('loturi ≤100, reîncercare la 429 cu retryDelay, vectori normalizați, contorizare', async () => {
  const sleeps: number[] = [];
  const { impl, seen } = fakeFetch((call, body) =>
    call === 1
      ? new Response(JSON.stringify({ error: { code: 429, details: [{ '@type': 'type.googleapis.com/google.rpc.RetryInfo', retryDelay: '2s' }] } }), { status: 429 })
      : okResponse(body, 8),
  );
  const e = new GeminiEmbedder({ apiKey: 'cheie-test', dimensions: 8, fetch: impl, sleep: async (ms) => void sleeps.push(ms), rpm: 1000, tpm: 1_000_000 });
  const items = Array.from({ length: 250 }, (_, i) => ({ text: `text ${i}`, title: 'T' }));
  const vectors = await e.embed(items, 'document');

  assert.equal(vectors.length, 250);
  assert.equal(seen.length, 4, '3 loturi + 1 reîncercare');
  assert.deepEqual(seen.map((s) => s.body.requests.length), [100, 100, 100, 50]);
  assert.deepEqual(sleeps, [2000]);
  assert.equal(e.stats.retries, 1);
  assert.equal(e.stats.items, 250);
  assert.equal(e.stats.requests, 4);
  assert.equal(e.stats.reportedTokens, 250 * 5);
  for (const v of vectors) assert.ok(Math.abs(Math.hypot(...v) - 1) < 1e-5);
  assert.equal(seen[0]?.headers['x-goog-api-key'], 'cheie-test');
  assert.ok(seen[0]?.url.endsWith('/models/gemini-embedding-2:batchEmbedContents'));
  const req = seen[1]?.body.requests[0];
  assert.equal(req?.taskType, 'RETRIEVAL_DOCUMENT');
  assert.equal(req?.title, 'T');
  assert.equal(req?.outputDimensionality, 8);
  assert.equal(req?.model, 'models/gemini-embedding-2');
  assert.equal(e.id, 'gemini:gemini-embedding-2:8');
});

test('interogările folosesc RETRIEVAL_QUERY și nu trimit titlu', async () => {
  const { impl, seen } = fakeFetch((_c, body) => okResponse(body, 4));
  const e = new GeminiEmbedder({ apiKey: 'k', dimensions: 4, fetch: impl, sleep: async () => undefined });
  await e.embed([{ text: 'q', title: 'ignorat' }], 'query');
  assert.equal(seen[0]?.body.requests[0]?.taskType, 'RETRIEVAL_QUERY');
  assert.equal(seen[0]?.body.requests[0]?.title, undefined);
});

test('lotul se închide și după plafonul de tokeni, nu doar după numărul de texte', async () => {
  const { impl, seen } = fakeFetch((_c, body) => okResponse(body, 4));
  const e = new GeminiEmbedder({ apiKey: 'k', dimensions: 4, fetch: impl, sleep: async () => undefined, maxBatchTokens: 300, rpm: 1000, tpm: 1_000_000 });
  await e.embed(Array.from({ length: 10 }, () => ({ text: 'x'.repeat(350) })), 'document');
  assert.equal(seen.length, 5, 'fiecare text ≈100 tokeni + 8 → încap două într-un lot de 300');
  assert.ok(seen.every((s) => s.body.requests.length === 2));
  assert.equal(e.stats.items, 10);
});

test('erorile nerecuperabile și dimensiunile greșite aruncă GeminiApiError', async () => {
  const bad = fakeFetch(() => new Response('{"error":{"message":"API key not valid"}}', { status: 400 }));
  const e1 = new GeminiEmbedder({ apiKey: 'k', fetch: bad.impl, sleep: async () => undefined });
  await assert.rejects(e1.embed([{ text: 'x' }], 'query'), (err: unknown) => err instanceof GeminiApiError && err.status === 400);

  const wrongDims = fakeFetch((_c, body) => okResponse(body, 5));
  const e2 = new GeminiEmbedder({ apiKey: 'k', dimensions: 8, fetch: wrongDims.impl, sleep: async () => undefined });
  await assert.rejects(e2.embed([{ text: 'x' }], 'query'), /dimensiuni/);

  let calls = 0;
  const flaky = fakeFetch(() => {
    calls++;
    return new Response('upstream', { status: 503 });
  });
  const e3 = new GeminiEmbedder({ apiKey: 'k', fetch: flaky.impl, sleep: async () => undefined, maxRetries: 2 });
  await assert.rejects(e3.embed([{ text: 'x' }], 'query'), (err: unknown) => err instanceof GeminiApiError && err.status === 503);
  assert.equal(calls, 3, '1 încercare + 2 reîncercări');
});

test('parseRetryDelayMs', () => {
  assert.equal(parseRetryDelayMs('{"retryDelay": "23s"}'), 23_000);
  assert.equal(parseRetryDelayMs('Please retry in 1.5s.'), 1500);
  assert.equal(parseRetryDelayMs('nimic'), null);
  assert.equal(parseRetryDelayMs('{"retryDelay":"0s"}'), 250, 'minim 250 ms');
});

test('limitatorul așteaptă exact cât trebuie (ceas fals)', async () => {
  let t = 0;
  const waits: number[] = [];
  const limiter = new SlidingWindowLimiter({
    rpm: 2,
    tpm: 1000,
    now: () => t,
    sleep: async (ms) => {
      waits.push(ms);
      t += ms;
    },
  });
  await limiter.acquire(10);
  await limiter.acquire(10);
  await limiter.acquire(10);
  assert.deepEqual(waits, [60_001], 'a treia cerere așteaptă golirea ferestrei');
  await limiter.acquire(990);
  await limiter.acquire(20);
  assert.equal(waits.length, 2, 'depășirea de tokeni așteaptă și ea');
  assert.equal(limiter.waits, 2);
  await limiter.acquire(5000);
  assert.equal(waits.length, 3, 'o cerere peste TPM trece după ce fereastra se golește');
});
