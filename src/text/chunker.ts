import type { ChunkInput } from '../types.ts';
import { estimateTokens, sha256 } from './normalize.ts';

export interface ChunkerOptions {
  /** Lungimea maximă a unui fragment, în caractere. Implicit 1500 (≈400 tokeni). */
  maxChars?: number | undefined;
  /** Sub această lungime, fragmentele mici se lipesc de vecinii lor. Implicit 400. */
  minChars?: number | undefined;
  /** Text repetat la începutul fragmentului următor când o secțiune e tăiată. Implicit 150. */
  overlapChars?: number | undefined;
  /** Titlul documentului — devine rădăcina căii de titluri. */
  title?: string | null | undefined;
  /** Interpretează titluri Markdown și blocuri de cod. Implicit true. */
  markdown?: boolean | undefined;
}

export interface TextChunk {
  ordinal: number;
  headingPath: string | null;
  text: string;
}

interface Block {
  text: string;
  path: string | null;
}

const MARKDOWN_EXTENSIONS = new Set(['.md', '.mdx', '.markdown', '.mkd']);

export function isMarkdownExtension(ext: string): boolean {
  return MARKDOWN_EXTENSIONS.has(ext.toLowerCase());
}

/**
 * Împarte un text în fragmente conștiente de structură:
 *  1. blocuri (paragrafe, liste, blocuri de cod atomice), fiecare cu calea de titluri curentă;
 *  2. împachetare lacomă în fragmente ≤ maxChars, cu tăiere de preferință la granițe de secțiune;
 *  3. blocurile prea lungi se taie pe linii → propoziții → cuvinte;
 *  4. suprapunere (overlap) doar când tăietura e în interiorul aceleiași secțiuni.
 */
export function chunkText(source: string, opts: ChunkerOptions = {}): TextChunk[] {
  const maxChars = Math.max(200, opts.maxChars ?? 1500);
  const minChars = Math.min(opts.minChars ?? 400, maxChars);
  const overlap = Math.min(opts.overlapChars ?? 150, Math.floor(maxChars / 3));
  const blocks = splitBlocks(source, opts.markdown ?? true, opts.title ?? null);
  return packBlocks(blocks, maxChars, minChars, overlap);
}

/** Adaugă hash + estimare de tokeni — forma pe care o stochează baza. */
export function toChunkInputs(chunks: readonly TextChunk[]): ChunkInput[] {
  return chunks.map((c) => ({
    ordinal: c.ordinal,
    headingPath: c.headingPath,
    text: c.text,
    contentHash: sha256(c.headingPath ?? '', c.text),
    tokenEstimate: estimateTokens(c.text),
  }));
}

function splitBlocks(source: string, markdown: boolean, title: string | null): Block[] {
  const lines = source.replace(/\r\n?/g, '\n').split('\n');
  const headings: (string | undefined)[] = [];
  const blocks: Block[] = [];
  let para: string[] = [];
  let fence: string | null = null;
  let fenceLines: string[] = [];

  const currentPath = (): string | null => {
    const list = [...headings];
    // Titlul documentului e de obicei chiar H1-ul; nu-l repetăm („Titlu > Titlu > H2").
    if (title !== null && list[0] !== undefined && list[0].trim() === title.trim()) list.shift();
    const parts = [title, ...list].filter((x): x is string => typeof x === 'string' && x.trim() !== '');
    return parts.length > 0 ? parts.join(' > ') : null;
  };
  const flushPara = (): void => {
    if (para.length === 0) return;
    const text = para.join('\n').trim();
    if (text !== '') blocks.push({ text, path: currentPath() });
    para = [];
  };

  for (const line of lines) {
    if (fence !== null) {
      fenceLines.push(line);
      if (line.trim().startsWith(fence)) {
        blocks.push({ text: fenceLines.join('\n'), path: currentPath() });
        fence = null;
        fenceLines = [];
      }
      continue;
    }
    const fenceOpen = markdown ? /^\s*(`{3,}|~{3,})/.exec(line) : null;
    if (fenceOpen) {
      flushPara();
      fence = fenceOpen[1] ?? '```';
      fenceLines = [line];
      continue;
    }
    const heading = markdown ? /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line) : null;
    if (heading) {
      flushPara();
      const level = heading[1]?.length ?? 1;
      headings.length = level - 1;
      headings[level - 1] = (heading[2] ?? '').trim();
      para.push(line); // titlul rămâne și în textul fragmentului, ca să fie lizibil de sine stătător
      continue;
    }
    if (line.trim() === '') {
      flushPara();
      continue;
    }
    para.push(line);
  }
  if (fence !== null && fenceLines.length > 0) blocks.push({ text: fenceLines.join('\n'), path: currentPath() });
  flushPara();
  return blocks;
}

function packBlocks(blocks: readonly Block[], maxChars: number, minChars: number, overlap: number): TextChunk[] {
  const out: TextChunk[] = [];
  let cur: string[] = [];
  let curLen = 0;
  let curPath: string | null = null;
  let carry = '';

  const flush = (continuation: boolean): void => {
    if (cur.length === 0) return;
    const text = cur.join('\n\n');
    out.push({ ordinal: out.length, headingPath: curPath, text });
    carry = continuation && overlap > 0 ? tail(text, overlap) : '';
    cur = [];
    curLen = 0;
  };

  for (const block of blocks) {
    // Graniță de secțiune: tăiem curat (fără suprapunere) dacă fragmentul curent e destul de mare.
    if (cur.length > 0 && block.path !== curPath && curLen >= minChars) flush(false);
    const pieces = block.text.length > maxChars ? splitLong(block.text, maxChars) : [block.text];
    for (const piece of pieces) {
      if (cur.length > 0 && curLen + 2 + piece.length > maxChars) flush(true);
      if (cur.length === 0) {
        curPath = block.path;
        if (carry !== '' && carry.length + 2 + piece.length <= maxChars) {
          cur.push(carry);
          curLen = carry.length;
        }
        carry = '';
      }
      cur.push(piece);
      curLen += (cur.length > 1 ? 2 : 0) + piece.length;
    }
  }
  flush(false);
  return out;
}

/** Ultimele ≤ n caractere, începând de la o graniță de cuvânt. */
function tail(text: string, n: number): string {
  if (text.length <= n) return text.trim();
  const t = text.slice(-n);
  const i = t.search(/\s/);
  return (i >= 0 ? t.slice(i + 1) : t).trim();
}

/** Taie un bloc prea lung: pe linii → pe propoziții → pe cuvinte → dur (URL-uri, base64). */
export function splitLong(text: string, maxChars: number): string[] {
  if (text.length <= maxChars) return [text];
  if (text.includes('\n')) return pack(text.split('\n'), '\n', maxChars);
  const sentences = text.split(/(?<=[.!?…])\s+/u);
  if (sentences.length > 1) return pack(sentences, ' ', maxChars);
  const words = text.split(/\s+/);
  if (words.length > 1) return pack(words, ' ', maxChars);
  const out: string[] = [];
  for (let i = 0; i < text.length; i += maxChars) out.push(text.slice(i, i + maxChars));
  return out;
}

function pack(units: readonly string[], sep: string, maxChars: number): string[] {
  const out: string[] = [];
  let buf = '';
  for (const unit of units) {
    for (const piece of splitLong(unit, maxChars)) {
      if (buf !== '' && buf.length + sep.length + piece.length > maxChars) {
        out.push(buf);
        buf = piece;
      } else {
        buf = buf === '' ? piece : buf + sep + piece;
      }
    }
  }
  if (buf !== '') out.push(buf);
  return out;
}
