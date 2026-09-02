import { createHash } from 'node:crypto';

/** SHA-256 hex peste mai multe părți, fiecare prefixată cu lungimea ei (fără coliziuni de concatenare). */
export function sha256(...parts: readonly string[]): string {
  const h = createHash('sha256');
  for (const p of parts) {
    h.update(`${Buffer.byteLength(p, 'utf8')}:`, 'utf8');
    h.update(p, 'utf8');
  }
  return h.digest('hex');
}

/** Estimare grosieră de tokeni: ≈3,5 caractere/token pentru română + engleză cu diacritice. */
export function estimateTokens(text: string): number {
  return Math.max(1, Math.ceil(text.length / 3.5));
}

/** ă→a, ș→s, ț→t etc. Aceeași pliere pe care o face tokenizatorul FTS5 (remove_diacritics 2). */
export function foldDiacritics(s: string): string {
  return s.normalize('NFD').replace(/\p{M}+/gu, '');
}

export function tokenize(s: string): string[] {
  return foldDiacritics(s).toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? [];
}

/** Cuvinte de legătură RO + EN, deja pliate (fără diacritice). */
export const STOPWORDS: ReadonlySet<string> = new Set([
  // română
  'de', 'la', 'in', 'si', 'cu', 'pe', 'din', 'un', 'o', 'al', 'a', 'ai', 'ale', 'ca', 'sa', 'se', 'ce', 'care',
  'este', 'sunt', 'era', 'erau', 'fi', 'fie', 'fost', 'mai', 'nu', 'da', 'dar', 'sau', 'pentru', 'prin', 'dupa',
  'pana', 'spre', 'fara', 'catre', 'asupra', 'despre', 'intre', 'sub', 'peste', 'lui', 'ei', 'lor', 'le', 'il',
  'ii', 'ne', 'va', 'vom', 'am', 'ati', 'au', 'are', 'avea', 'avem', 'aceasta', 'acest', 'acesta', 'aceste',
  'acel', 'acea', 'acele', 'unde', 'cand', 'cum', 'cat', 'toate', 'tot', 'toti', 'foarte', 'doar', 'insa',
  'deci', 'iar', 'ori', 'daca', 'atunci', 'aici', 'acolo', 'asa', 'e', 's', 'l', 'i', 'imi', 'iti', 'isi',
  'mi', 'ti', 'te', 'ma', 'eu', 'tu', 'el', 'ea', 'noi', 'voi', 'ele', 'meu', 'mea', 'tau', 'ta',
  // engleză
  'the', 'an', 'of', 'to', 'on', 'for', 'and', 'or', 'is', 'are', 'was', 'were', 'be', 'been', 'it', 'this',
  'that', 'these', 'those', 'with', 'as', 'at', 'by', 'from', 'not', 'but', 'if', 'then', 'so', 'we', 'you',
  'they', 'he', 'she', 'my', 'our', 'your', 'their', 'its', 'do', 'does', 'did', 'have', 'has', 'had', 'will',
  'would', 'can', 'could', 'should', 'what', 'which', 'who', 'how', 'when', 'where', 'why', 'into', 'than',
]);

/** Tokeni purtători de sens: fără stopwords și fără fragmente de un caracter. */
export function contentTokens(s: string): string[] {
  return tokenize(s).filter((t) => t.length >= 2 && !STOPWORDS.has(t));
}

export interface FtsQueryOptions {
  operator?: 'AND' | 'OR' | undefined;
  /** Tokenii cel puțin atât de lungi devin căutări de prefix („atelier"* prinde „atelierele"). */
  prefixMinLength?: number | undefined;
  maxTokens?: number | undefined;
}

/**
 * Construiește o interogare FTS5 sigură din text liber: fiecare token e citat (deci operatorii
 * utilizatorului nu sunt interpretați), tokenii lungi devin prefixe pentru a prinde flexiunile.
 */
export function buildFtsQuery(text: string, opts: FtsQueryOptions = {}): string | null {
  const prefixMin = opts.prefixMinLength ?? 5;
  const max = opts.maxTokens ?? 32;
  let tokens = contentTokens(text);
  if (tokens.length === 0) tokens = tokenize(text);
  const seen = new Set<string>();
  const parts: string[] = [];
  for (const t of tokens) {
    if (seen.has(t)) continue;
    seen.add(t);
    parts.push(t.length >= prefixMin ? `"${t}"*` : `"${t}"`);
    if (parts.length >= max) break;
  }
  if (parts.length === 0) return null;
  return parts.join(opts.operator === 'OR' ? ' OR ' : ' ');
}
