import { estimateTokens } from '../text/normalize.ts';
import { l2normalize, type EmbedItem, type EmbedTask, type Embedder } from './embedder.ts';
import { SlidingWindowLimiter } from './ratelimit.ts';

export const GEMINI_BASE_URL = 'https://generativelanguage.googleapis.com/v1beta';

export interface GeminiEmbedderOptions {
  apiKey: string;
  /** Implicit gemini-embedding-2. */
  model?: string | undefined;
  /** Dimensiune MRL (768 / 1536 / 3072). Implicit 768. */
  dimensions?: number | undefined;
  rpm?: number | undefined;
  tpm?: number | undefined;
  /** Maxim 100 la batchEmbedContents. */
  maxBatchItems?: number | undefined;
  /** Plafon de tokeni estimați per cerere, ca o singură cerere să nu consume tot TPM-ul. */
  maxBatchTokens?: number | undefined;
  maxRetries?: number | undefined;
  baseUrl?: string | undefined;
  fetch?: typeof fetch | undefined;
  sleep?: ((ms: number) => Promise<void>) | undefined;
  now?: (() => number) | undefined;
  log?: ((msg: string) => void) | undefined;
}

export class GeminiApiError extends Error {
  readonly status: number;
  readonly body: string;
  constructor(status: number, body: string) {
    super(`Gemini API ${status}: ${body.slice(0, 300)}`);
    this.name = 'GeminiApiError';
    this.status = status;
    this.body = body;
  }
}

interface BatchEmbedResponse {
  embeddings?: { values?: number[] }[];
  usageMetadata?: { totalTokenCount?: number };
}

export interface GeminiStats {
  requests: number;
  items: number;
  retries: number;
  estimatedTokens: number;
  reportedTokens: number;
}

/**
 * Embeddings prin Gemini API (Google AI Studio): loturi de până la 100 de texte, limitator RPM/TPM,
 * reîncercare cu backoff care respectă `retryDelay` din răspunsurile 429, vectori normalizați.
 */
export class GeminiEmbedder implements Embedder {
  readonly id: string;
  readonly dimensions: number;
  readonly model: string;
  readonly stats: GeminiStats = { requests: 0, items: 0, retries: 0, estimatedTokens: 0, reportedTokens: 0 };
  readonly limiter: SlidingWindowLimiter;
  readonly #apiKey: string;
  readonly #fetch: typeof fetch;
  readonly #sleep: (ms: number) => Promise<void>;
  readonly #maxBatchItems: number;
  readonly #maxBatchTokens: number;
  readonly #maxRetries: number;
  readonly #baseUrl: string;
  readonly #log: ((msg: string) => void) | undefined;

  constructor(o: GeminiEmbedderOptions) {
    if (!o.apiKey) throw new Error('GeminiEmbedder: lipsește apiKey');
    this.model = o.model ?? 'gemini-embedding-2';
    this.dimensions = o.dimensions ?? 768;
    this.id = `gemini:${this.model}:${this.dimensions}`;
    this.#apiKey = o.apiKey;
    this.#fetch = o.fetch ?? fetch;
    this.#sleep = o.sleep ?? ((ms) => new Promise((r) => setTimeout(r, ms)));
    this.#maxBatchItems = Math.min(100, Math.max(1, o.maxBatchItems ?? 100));
    this.#maxBatchTokens = Math.max(256, o.maxBatchTokens ?? 20_000);
    this.#maxRetries = Math.max(0, o.maxRetries ?? 6);
    this.#baseUrl = (o.baseUrl ?? GEMINI_BASE_URL).replace(/\/+$/, '');
    this.#log = o.log;
    this.limiter = new SlidingWindowLimiter({
      rpm: o.rpm ?? 90,
      tpm: o.tpm ?? 28_000,
      now: o.now,
      sleep: this.#sleep,
    });
  }

  async embed(items: readonly EmbedItem[], task: EmbedTask): Promise<Float32Array[]> {
    const out: Float32Array[] = new Array<Float32Array>(items.length);
    let start = 0;
    while (start < items.length) {
      let end = start;
      let tokens = 0;
      while (end < items.length && end - start < this.#maxBatchItems) {
        const item = items[end];
        const t = estimateTokens((item?.title ?? '') + (item?.text ?? '')) + 8;
        if (end > start && tokens + t > this.#maxBatchTokens) break;
        tokens += t;
        end++;
      }
      const vectors = await this.#embedBatch(items.slice(start, end), task, tokens);
      for (let i = 0; i < vectors.length; i++) out[start + i] = vectors[i]!;
      start = end;
    }
    return out;
  }

  async #embedBatch(batch: readonly EmbedItem[], task: EmbedTask, tokens: number): Promise<Float32Array[]> {
    const taskType = task === 'query' ? 'RETRIEVAL_QUERY' : 'RETRIEVAL_DOCUMENT';
    const body = JSON.stringify({
      requests: batch.map((it) => {
        const req: Record<string, unknown> = {
          model: `models/${this.model}`,
          content: { parts: [{ text: it.text }] },
          taskType,
          outputDimensionality: this.dimensions,
        };
        if (task === 'document' && it.title) req.title = it.title;
        return req;
      }),
    });
    const url = `${this.#baseUrl}/models/${this.model}:batchEmbedContents`;

    for (let attempt = 0; ; attempt++) {
      await this.limiter.acquire(tokens);
      this.stats.requests++;
      this.stats.estimatedTokens += tokens;
      let res: Response;
      try {
        res = await this.#fetch(url, {
          method: 'POST',
          headers: { 'content-type': 'application/json', 'x-goog-api-key': this.#apiKey },
          body,
        });
      } catch (err) {
        if (attempt >= this.#maxRetries) throw err;
        await this.#backoff(attempt, null, `rețea: ${(err as Error).message}`);
        continue;
      }

      if (res.ok) {
        const json = (await res.json()) as BatchEmbedResponse;
        const embeddings = json.embeddings ?? [];
        if (embeddings.length !== batch.length) {
          throw new GeminiApiError(res.status, `am cerut ${batch.length} vectori, am primit ${embeddings.length}`);
        }
        this.stats.items += batch.length;
        this.stats.reportedTokens += json.usageMetadata?.totalTokenCount ?? 0;
        return embeddings.map((e, i) => {
          const values = e.values ?? [];
          if (values.length !== this.dimensions) {
            throw new GeminiApiError(res.status, `vectorul ${i} are ${values.length} dimensiuni, așteptam ${this.dimensions}`);
          }
          return l2normalize(Float32Array.from(values));
        });
      }

      const text = await res.text();
      const retryable = res.status === 429 || res.status === 408 || res.status >= 500;
      if (!retryable || attempt >= this.#maxRetries) throw new GeminiApiError(res.status, text);
      await this.#backoff(attempt, parseRetryDelayMs(text), `HTTP ${res.status}`);
    }
  }

  async #backoff(attempt: number, hintMs: number | null, reason: string): Promise<void> {
    const ms = hintMs ?? Math.min(60_000, 1000 * 2 ** attempt) + Math.floor(Math.random() * 250);
    this.stats.retries++;
    this.#log?.(`Gemini: reîncerc în ${ms} ms (${reason})`);
    await this.#sleep(ms);
  }
}

/** Extrage `retryDelay: "23s"` (google.rpc.RetryInfo) sau „retry in 23s" din corpul unei erori. */
export function parseRetryDelayMs(body: string): number | null {
  const m = /"retryDelay"\s*:\s*"(\d+(?:\.\d+)?)s"/.exec(body) ?? /retry in (\d+(?:\.\d+)?)\s*s/i.exec(body);
  if (!m) return null;
  return Math.max(250, Math.ceil(Number.parseFloat(m[1] ?? '1') * 1000));
}

export interface ModelInfo {
  name: string;
  displayName: string;
  inputTokenLimit: number;
  methods: string[];
}

/** Modelele de embedding vizibile pentru cheia dată. */
export async function listEmbeddingModels(
  apiKey: string,
  fetchImpl: typeof fetch = fetch,
  baseUrl: string = GEMINI_BASE_URL,
): Promise<ModelInfo[]> {
  const res = await fetchImpl(`${baseUrl}/models?pageSize=200`, { headers: { 'x-goog-api-key': apiKey } });
  if (!res.ok) throw new GeminiApiError(res.status, await res.text());
  const json = (await res.json()) as {
    models?: { name?: string; displayName?: string; inputTokenLimit?: number; supportedGenerationMethods?: string[] }[];
  };
  return (json.models ?? [])
    .filter((m) => (m.supportedGenerationMethods ?? []).some((x) => /embed/i.test(x)))
    .map((m) => ({
      name: (m.name ?? '').replace(/^models\//, ''),
      displayName: m.displayName ?? '',
      inputTokenLimit: m.inputTokenLimit ?? 0,
      methods: m.supportedGenerationMethods ?? [],
    }));
}
