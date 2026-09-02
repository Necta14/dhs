import { existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { join, resolve } from 'node:path';

export interface Config {
  dbPath: string;
  embedModel: string;
  embedDims: number;
  /** Cereri pe minut permise către API-ul de embeddings. */
  rpm: number;
  /** Tokeni pe minut (estimare locală) permiși către API. */
  tpm: number;
  apiKey: string | null;
}

export const DEFAULTS = {
  embedModel: 'gemini-embedding-2',
  embedDims: 768,
  // Free tier Gemini: ~100 RPM / 30 000 TPM pentru embeddings. Stăm puțin sub.
  rpm: 90,
  tpm: 28_000,
} as const;

/** Baza implicită e globală (XDG), ca aceeași memorie să fie văzută din orice proiect. */
export function defaultDbPath(env: NodeJS.ProcessEnv = process.env): string {
  const xdg = env.XDG_DATA_HOME && env.XDG_DATA_HOME !== '' ? env.XDG_DATA_HOME : join(homedir(), '.local', 'share');
  return join(xdg, 'dhs', 'dhs.sqlite');
}

/**
 * Încarcă .env.local și .env din cwd în process.env (fără a suprascrie variabile existente).
 * Fișierele sunt citite de runtime, nu de agent — regula „nu deschide .env" rămâne intactă.
 */
export function loadEnvFiles(cwd = process.cwd(), names: readonly string[] = ['.env.local', '.env']): string[] {
  const loaded: string[] = [];
  for (const name of names) {
    const path = resolve(cwd, name);
    if (!existsSync(path)) continue;
    try {
      process.loadEnvFile(path);
      loaded.push(name);
    } catch {
      // fișier ilizibil sau malformat — îl ignorăm, nu blocăm CLI-ul
    }
  }
  return loaded;
}

export function readConfig(env: NodeJS.ProcessEnv = process.env): Config {
  const num = (raw: string | undefined, fallback: number): number => {
    const n = raw === undefined || raw === '' ? Number.NaN : Number(raw);
    return Number.isFinite(n) && n > 0 ? n : fallback;
  };
  return {
    dbPath: resolve(env.DHS_DB_PATH && env.DHS_DB_PATH !== '' ? env.DHS_DB_PATH : defaultDbPath(env)),
    embedModel: env.DHS_EMBED_MODEL && env.DHS_EMBED_MODEL !== '' ? env.DHS_EMBED_MODEL : DEFAULTS.embedModel,
    embedDims: num(env.DHS_EMBED_DIMS, DEFAULTS.embedDims),
    rpm: num(env.DHS_EMBED_RPM, DEFAULTS.rpm),
    tpm: num(env.DHS_EMBED_TPM, DEFAULTS.tpm),
    apiKey: env.GEMINI_API_KEY ?? env.GOOGLE_API_KEY ?? null,
  };
}
