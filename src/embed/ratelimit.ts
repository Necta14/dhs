export interface LimiterOptions {
  /** Cereri pe minut. */
  rpm: number;
  /** Tokeni pe minut (estimare). */
  tpm: number;
  now?: (() => number) | undefined;
  sleep?: ((ms: number) => Promise<void>) | undefined;
}

const WINDOW_MS = 60_000;

/**
 * Limitator cu fereastră glisantă de 60 s pe două dimensiuni (cereri și tokeni).
 * `acquire` blochează exact cât trebuie ca să nu depășim niciuna dintre limite.
 */
export class SlidingWindowLimiter {
  readonly #rpm: number;
  readonly #tpm: number;
  readonly #now: () => number;
  readonly #sleep: (ms: number) => Promise<void>;
  #requests: number[] = [];
  #tokens: { at: number; n: number }[] = [];
  #tokenSum = 0;
  waits = 0;
  waitedMs = 0;

  constructor(o: LimiterOptions) {
    this.#rpm = Math.max(1, Math.floor(o.rpm));
    this.#tpm = Math.max(1, Math.floor(o.tpm));
    this.#now = o.now ?? (() => Date.now());
    this.#sleep = o.sleep ?? ((ms) => new Promise((r) => setTimeout(r, ms)));
  }

  async acquire(tokens: number): Promise<void> {
    for (;;) {
      const now = this.#now();
      this.#prune(now - WINDOW_MS);
      const reqOk = this.#requests.length < this.#rpm;
      // O cerere mai mare decât întreaga limită trece oricum când fereastra e goală — altfel ar aștepta la infinit.
      const tokOk = this.#tokenSum + tokens <= this.#tpm || this.#tokens.length === 0;
      if (reqOk && tokOk) {
        this.#requests.push(now);
        this.#tokens.push({ at: now, n: tokens });
        this.#tokenSum += tokens;
        return;
      }
      const oldestReq = reqOk ? Number.POSITIVE_INFINITY : (this.#requests[0] ?? now);
      const oldestTok = tokOk ? Number.POSITIVE_INFINITY : (this.#tokens[0]?.at ?? now);
      const wait = Math.max(50, Math.min(oldestReq, oldestTok) + WINDOW_MS - now + 1);
      this.waits++;
      this.waitedMs += wait;
      await this.#sleep(wait);
    }
  }

  #prune(cutoff: number): void {
    while (this.#requests.length > 0 && (this.#requests[0] ?? 0) <= cutoff) this.#requests.shift();
    while (this.#tokens.length > 0 && (this.#tokens[0]?.at ?? 0) <= cutoff) {
      this.#tokenSum -= this.#tokens.shift()?.n ?? 0;
    }
  }
}
