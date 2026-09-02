import { randomUUID } from 'node:crypto';
import type { Store } from '../db/store.ts';
import type { Embedder } from '../embed/embedder.ts';
import { embedPending } from '../embed/pipeline.ts';
import { chunkText, toChunkInputs } from '../text/chunker.ts';
import { sha256 } from '../text/normalize.ts';
import {
  MEMORY_TYPES,
  type MemoryHit, type MemoryListItem, type MemoryRecord, type MemoryType,
  type RecallOptions, type RememberInput, type RememberResult, type SearchHit,
} from '../types.ts';

export const DAY_MS = 86_400_000;

export function clamp01(x: number): number {
  return Math.min(1, Math.max(0, Number.isFinite(x) ? x : 0));
}

export function isMemoryType(x: string): x is MemoryType {
  return (MEMORY_TYPES as readonly string[]).includes(x);
}

/**
 * Salvează o memorie: document de tip „memory" + fragmente + rândul din `memories`, într-o singură
 * tranzacție. Dacă înlocuiește o memorie mai veche, aceea devine inactivă, dar rămâne în istoric.
 */
export async function remember(
  store: Store,
  embedder: Embedder | null,
  input: RememberInput,
  now = Date.now(),
): Promise<RememberResult> {
  const text = input.text.trim();
  if (text === '') throw new Error('remember: textul e gol');
  const type = input.type ?? 'fact';
  if (!isMemoryType(type)) throw new Error(`remember: tip necunoscut „${type}" (permise: ${MEMORY_TYPES.join(', ')})`);
  const namespace = (input.namespace ?? 'default').trim() || 'default';
  const importance = clamp01(input.importance ?? 0.5);
  const tags = [...new Set((input.tags ?? []).map((t) => t.trim().toLowerCase()).filter((t) => t !== ''))];
  const title = (input.title ?? firstLine(text)).slice(0, 120);
  const uri = `memory://${namespace}/${randomUUID()}`;

  let superseded: number | null = null;
  if (input.supersedes !== undefined) {
    const old = store.getDocument(input.supersedes);
    if (!old || old.kind !== 'memory') throw new Error(`remember: memoria #${input.supersedes} nu există`);
    superseded = old.id;
  }

  const chunks = toChunkInputs(chunkText(text, { title: null, markdown: true }));
  const id = store.transaction(() => {
    const doc = store.upsertDocument(
      {
        uri,
        kind: 'memory',
        namespace,
        title,
        contentHash: sha256(text),
        metadata: { ...(input.metadata ?? {}), source: 'remember' },
      },
      now,
    );
    store.replaceChunks(doc.id, chunks);
    store.insertMemory({
      documentId: doc.id,
      memoryType: type,
      importance,
      tags,
      supersedes: superseded,
      expiresAt: input.expiresAt ?? null,
    });
    if (superseded !== null) store.supersede(superseded, doc.id, now);
    return doc.id;
  });

  let embedded = false;
  if (embedder) {
    await embedPending(store, embedder);
    embedded = true;
  }
  return { id, uri, chunks: chunks.length, embedded, superseded };
}

function firstLine(text: string): string {
  const line = text.split('\n').find((l) => l.trim() !== '') ?? text;
  return line.replace(/^#+\s*/, '').replace(/^[-*]\s+/, '').trim();
}

export interface MemoryScoring {
  halfLifeDays: number;
  recencyWeight: number;
  importanceWeight: number;
}

export function scoringFrom(opts: Pick<RecallOptions, 'halfLifeDays' | 'recencyWeight' | 'importanceWeight'>): MemoryScoring {
  return {
    halfLifeDays: opts.halfLifeDays ?? 30,
    recencyWeight: clamp01(opts.recencyWeight ?? 0.3),
    importanceWeight: Math.max(0, opts.importanceWeight ?? 0.5),
  };
}

/** 1 pentru „acum", 0,5 după `halfLifeDays` zile, 0,25 după dublul lor… */
export function recencyFactor(updatedAt: number, now: number, halfLifeDays: number): number {
  if (halfLifeDays <= 0) return 1;
  const ageDays = Math.max(0, now - updatedAt) / DAY_MS;
  return 0.5 ** (ageDays / halfLifeDays);
}

/** relevanță × (1 + w_imp·importanță) × (1 − w_rec + w_rec·prospețime). Relevanța rămâne factorul principal. */
export function finalScore(relevance: number, importance: number, recency: number, s: MemoryScoring): number {
  return relevance * (1 + s.importanceWeight * importance) * (1 - s.recencyWeight + s.recencyWeight * recency);
}

/** Re-notează rezultatele căutării; fiecare memorie apare o singură dată (cel mai bun fragment al ei). */
export function rankMemoryHits(
  hits: readonly SearchHit[],
  memories: ReadonlyMap<number, MemoryRecord>,
  s: MemoryScoring,
  now: number,
): MemoryHit[] {
  const max = hits.reduce((m, h) => Math.max(m, h.score), 0) || 1;
  const best = new Map<number, MemoryHit>();
  for (const h of hits) {
    const memory = memories.get(h.documentId);
    if (!memory) continue;
    const relevance = h.score / max;
    const score = finalScore(relevance, memory.importance, recencyFactor(h.updatedAt, now, s.halfLifeDays), s);
    const prev = best.get(h.documentId);
    if (!prev || score > prev.finalScore) best.set(h.documentId, { ...h, memory, relevance, finalScore: score });
  }
  return [...best.values()].sort((a, b) => b.finalScore - a.finalScore);
}

export interface HandoffItem {
  id: number;
  type: MemoryType;
  title: string | null;
  text: string;
  tags: string[];
  importance: number;
  updatedAt: number;
  score: number;
}

const HANDOFF_ORDER: readonly MemoryType[] = ['handoff', 'decision', 'problem', 'task', 'preference', 'fact', 'episode'];
const HANDOFF_LABELS: Record<MemoryType, string> = {
  handoff: 'Predări anterioare',
  decision: 'Decizii',
  problem: 'Probleme cunoscute',
  task: 'Sarcini deschise',
  preference: 'Preferințe',
  fact: 'Fapte',
  episode: 'Episoade',
};

/** Fără interogare, ordinea e importanță × prospețime. */
export function rankForHandoff(items: readonly MemoryListItem[], s: MemoryScoring, now: number, limit: number): HandoffItem[] {
  return items
    .map((m) => ({
      id: m.documentId,
      type: m.memoryType,
      title: m.title,
      text: m.text,
      tags: m.tags,
      importance: m.importance,
      updatedAt: m.updatedAt,
      score: finalScore(1, m.importance, recencyFactor(m.updatedAt, now, s.halfLifeDays), s),
    }))
    .sort((a, b) => b.score - a.score)
    .slice(0, limit);
}

export function handoffMarkdown(items: readonly HandoffItem[], namespace: string, now: number): string {
  const stamp = new Date(now).toISOString().slice(0, 16).replace('T', ' ');
  const lines: string[] = [`# Handoff · ${namespace} · ${stamp} UTC`, ''];
  if (items.length === 0) {
    lines.push('_Nicio memorie activă în acest spațiu._');
    return `${lines.join('\n')}\n`;
  }
  for (const type of HANDOFF_ORDER) {
    const group = items.filter((i) => i.type === type);
    if (group.length === 0) continue;
    lines.push(`## ${HANDOFF_LABELS[type]}`, '');
    for (const it of group) {
      const date = new Date(it.updatedAt).toISOString().slice(0, 10);
      const tags = it.tags.length > 0 ? ` · ${it.tags.map((t) => `#${t}`).join(' ')}` : '';
      lines.push(`- **#${it.id}** · ${date} · importanță ${it.importance.toFixed(2)}${tags}`);
      lines.push(indent(it.text));
    }
    lines.push('');
  }
  return `${lines.join('\n').trimEnd()}\n`;
}

function indent(text: string): string {
  return text
    .split('\n')
    .map((l) => `  ${l}`)
    .join('\n');
}
