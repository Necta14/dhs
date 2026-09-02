#!/usr/bin/env node
import { existsSync, statSync } from 'node:fs';
import { parseArgs, styleText } from 'node:util';
import { DEFAULTS, loadEnvFiles, readConfig, type Config } from './config.ts';
import { GeminiEmbedder, listEmbeddingModels } from './embed/gemini.ts';
import { Suite, createEmbedderFromConfig } from './index.ts';
import { isMemoryType } from './memory/memory.ts';
import { DOCUMENT_KINDS, MEMORY_TYPES, type DocumentKind, type MemoryType, type SearchMode } from './types.ts';

const HELP = `dhs — Direct Handoff Suite · memorie și cunoștințe locale (SQLite + Gemini embeddings)

Comenzi
  index <cale...>        indexează fișiere/dosare (incremental) și le vectorizează
  embed                  vectorizează ce a rămas în așteptare
  search <interogare>    căutare hibridă (vector + BM25, fuziune RRF)
  remember <text>        salvează o memorie
  recall <interogare>    caută în memorii (relevanță × importanță × prospețime)
  forget <id>            dezactivează o memorie (rămâne în istoric)
  history <id>           lanțul de înlocuiri al unei memorii
  handoff                rezumat Markdown al memoriilor unui spațiu, pentru agentul următor
  stats                  cifre despre bază
  models                 modelele de embedding disponibile pe cheia curentă
  doctor                 verifică mediul, baza și accesul la API

Opțiuni
  -n, --namespace <ns>   spațiul de nume (implicit „default")
  -k, --limit <n>        câte rezultate (implicit 10; handoff 25)
      --mode <m>         hybrid | vector | lexical (implicit hybrid, sau lexical fără cheie)
      --kind <k>         file | memory | note — restrânge căutarea
  -t, --type <t>         tipul memoriei: ${MEMORY_TYPES.join(' | ')}
      --tags a,b         etichete (remember: setează; recall: filtrează, OR)
      --importance <x>   0..1 (implicit 0,5)
      --supersedes <id>  memoria pe care o înlocuiește
      --title <t>        titlu explicit pentru memorie
      --expires <data>   ISO 8601 — după această dată memoria nu mai e returnată
  -q, --query <text>     (handoff) filtrează după relevanță față de o interogare
      --ext md,txt       (index) extensii acceptate
      --mmr <λ>          re-ordonare MMR pentru diversitate, λ ∈ 0..1
      --no-embed         (index) nu vectoriza acum
      --no-prune         (index) nu dezactiva fișierele dispărute
      --db <cale>        baza SQLite (altfel DHS_DB_PATH sau ~/.local/share/dhs/dhs.sqlite)
      --json             ieșire JSON
  -v, --verbose          detalii pe stderr
  -h, --help

Mediu   GEMINI_API_KEY (sau GOOGLE_API_KEY) · DHS_DB_PATH · DHS_EMBED_MODEL (${DEFAULTS.embedModel})
        DHS_EMBED_DIMS (${DEFAULTS.embedDims}) · DHS_EMBED_RPM (${DEFAULTS.rpm}) · DHS_EMBED_TPM (${DEFAULTS.tpm})
        Se citesc automat .env.local și .env din directorul curent.
`;

type Fmt = Parameters<typeof styleText>[0];
const tty = process.stdout.isTTY === true;
const paint = (fmt: Fmt, s: string): string => (tty ? styleText(fmt, s) : s);
const dim = (s: string): string => paint('dim', s);
const bold = (s: string): string => paint('bold', s);
const ok = (s: string): string => paint('green', s);
const warn = (s: string): string => paint('yellow', s);
const bad = (s: string): string => paint('red', s);

class UsageError extends Error {}

function parseInt10(raw: string | undefined, fallback: number, name: string): number {
  if (raw === undefined) return fallback;
  const n = Number.parseInt(raw, 10);
  if (!Number.isFinite(n) || n <= 0) throw new UsageError(`--${name} trebuie să fie un întreg pozitiv`);
  return n;
}

function parseFloat01(raw: string | undefined, fallback: number, name: string): number {
  if (raw === undefined) return fallback;
  const n = Number.parseFloat(raw);
  if (!Number.isFinite(n) || n < 0 || n > 1) throw new UsageError(`--${name} trebuie să fie între 0 și 1`);
  return n;
}

function parseList(raw: string | undefined): string[] | undefined {
  if (raw === undefined) return undefined;
  return raw.split(',').map((s) => s.trim()).filter((s) => s !== '');
}

function parseMemoryTypes(raw: string | undefined): MemoryType[] | undefined {
  const list = parseList(raw);
  if (!list) return undefined;
  for (const t of list) if (!isMemoryType(t)) throw new UsageError(`tip de memorie necunoscut „${t}"`);
  return list as MemoryType[];
}

function parseKinds(raw: string | undefined): DocumentKind[] | undefined {
  const list = parseList(raw);
  if (!list) return undefined;
  for (const k of list) if (!(DOCUMENT_KINDS as readonly string[]).includes(k)) throw new UsageError(`kind necunoscut „${k}"`);
  return list as DocumentKind[];
}

function parseMode(raw: string | undefined): SearchMode | undefined {
  if (raw === undefined) return undefined;
  if (raw === 'hybrid' || raw === 'vector' || raw === 'lexical') return raw;
  throw new UsageError('--mode trebuie să fie hybrid, vector sau lexical');
}

function parseDate(raw: string | undefined): number | null {
  if (raw === undefined) return null;
  const t = Date.parse(raw);
  if (Number.isNaN(t)) throw new UsageError(`--expires: dată invalidă „${raw}"`);
  return t;
}

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function snippet(text: string, max = 220): string {
  const oneLine = text.replace(/\s+/g, ' ').trim();
  return oneLine.length > max ? `${oneLine.slice(0, max - 1)}…` : oneLine;
}

function progressPrinter(): (p: { done: number; total: number; fromCache: number; fromApi: number }) => void {
  let printed = false;
  return (p) => {
    if (p.total === 0) return;
    process.stderr.write(`\r  vectorizare ${p.done}/${p.total}  ${dim(`(cache ${p.fromCache}, API ${p.fromApi})`)}`);
    printed = true;
    if (p.done >= p.total && printed) process.stderr.write('\n');
  };
}

async function main(argv: readonly string[]): Promise<number> {
  const { values, positionals } = parseArgs({
    args: [...argv],
    allowPositionals: true,
    allowNegative: true,
    strict: true,
    options: {
      db: { type: 'string' },
      namespace: { type: 'string', short: 'n' },
      limit: { type: 'string', short: 'k' },
      mode: { type: 'string' },
      kind: { type: 'string' },
      type: { type: 'string', short: 't' },
      tags: { type: 'string' },
      importance: { type: 'string' },
      supersedes: { type: 'string' },
      title: { type: 'string' },
      expires: { type: 'string' },
      query: { type: 'string', short: 'q' },
      ext: { type: 'string' },
      mmr: { type: 'string' },
      embed: { type: 'boolean', default: true },
      prune: { type: 'boolean', default: true },
      json: { type: 'boolean', default: false },
      verbose: { type: 'boolean', short: 'v', default: false },
      help: { type: 'boolean', short: 'h', default: false },
    },
  });

  const [command, ...rest] = positionals;
  if (values.help || command === undefined) {
    process.stdout.write(HELP);
    return command === undefined && !values.help ? 1 : 0;
  }

  const loadedEnv = loadEnvFiles();
  const cfg: Config = readConfig();
  if (values.db) cfg.dbPath = values.db;
  const log = values.verbose ? (msg: string): void => void process.stderr.write(`${dim(msg)}\n`) : undefined;
  if (values.verbose && loadedEnv.length > 0) log?.(`env încărcat din ${loadedEnv.join(', ')}`);

  if (command === 'models') return cmdModels(cfg, values.json);
  if (command === 'doctor') return cmdDoctor(cfg);

  const embedder = createEmbedderFromConfig(cfg, { log });
  const suite = Suite.open({ dbPath: cfg.dbPath, embedder });
  const namespace = values.namespace ?? 'default';
  const limit = parseInt10(values.limit, command === 'handoff' ? 25 : 10, 'limit');
  const emit = (data: unknown): void => void process.stdout.write(`${JSON.stringify(data, null, 2)}\n`);

  try {
    switch (command) {
      case 'index': {
        if (rest.length === 0) throw new UsageError('index: dă cel puțin o cale');
        for (const p of rest) if (!existsSync(p)) throw new UsageError(`index: nu găsesc „${p}"`);
        if (!embedder && values.embed) process.stderr.write(`${warn('fără GEMINI_API_KEY')}: indexez doar lexical; rulează „dhs embed" după ce setezi cheia.\n`);
        const report = await suite.index(rest, {
          namespace,
          extensions: parseList(values.ext),
          prune: values.prune,
          embed: values.embed,
          onProgress: values.json ? undefined : progressPrinter(),
          onFile: values.verbose ? (e) => process.stderr.write(`${dim(e.status.padEnd(9))} ${e.path}${e.reason ? dim(`  (${e.reason})`) : ''}\n`) : undefined,
        });
        suite.store.optimize();
        if (values.json) {
          emit(report);
        } else {
          const { scanned, added, updated, unchanged, skipped, pruned, chunks, reusedEmbeddings, embed } = report;
          process.stdout.write(
            `${bold('Indexat')} ${scanned} fișiere: ${ok(`+${added}`)} noi, ${updated} actualizate, ${unchanged} neschimbate, ${skipped} sărite, ${pruned} dezactivate.\n` +
              `Fragmente noi: ${chunks}${reusedEmbeddings > 0 ? ` (${reusedEmbeddings} și-au păstrat vectorul)` : ''}.` +
              (embed ? ` Vectorizate: ${embed.done} (cache ${embed.fromCache}, API ${embed.fromApi}).` : '') +
              '\n',
          );
          if (embedder instanceof GeminiEmbedder && embedder.stats.requests > 0) {
            const s = embedder.stats;
            process.stdout.write(dim(`Gemini: ${s.requests} cereri, ${s.items} texte, ~${s.reportedTokens || s.estimatedTokens} tokeni, ${s.retries} reîncercări.\n`));
          }
        }
        return 0;
      }

      case 'embed': {
        const p = await suite.embedPending({ onProgress: values.json ? undefined : progressPrinter() });
        if (values.json) emit(p);
        else process.stdout.write(`Vectorizate ${p.done} fragmente (cache ${p.fromCache}, API ${p.fromApi}).\n`);
        return 0;
      }

      case 'search': {
        const query = rest.join(' ').trim();
        if (query === '') throw new UsageError('search: lipsește interogarea');
        const hits = await suite.search(query, {
          namespace: values.namespace,
          limit,
          mode: parseMode(values.mode),
          kinds: parseKinds(values.kind),
          mmrLambda: values.mmr !== undefined ? parseFloat01(values.mmr, 0.7, 'mmr') : undefined,
        });
        if (values.json) {
          emit(hits);
          return 0;
        }
        if (hits.length === 0) {
          process.stdout.write(`${warn('Niciun rezultat.')}\n`);
          return 0;
        }
        hits.forEach((h, i) => {
          const where = h.headingPath ?? h.title ?? h.uri; // calea include deja titlul documentului
          const ranks = [h.vectorRank !== null ? `v${h.vectorRank}` : null, h.lexicalRank !== null ? `l${h.lexicalRank}` : null].filter(Boolean).join(' ');
          process.stdout.write(`${bold(`${i + 1}.`)} ${where}  ${dim(`[${h.kind} · ${h.score.toFixed(4)} · ${ranks}]`)}\n`);
          process.stdout.write(`   ${dim(h.uri)}${h.chunkId ? dim(`  #${h.chunkId}`) : ''}\n`);
          process.stdout.write(`   ${snippet(h.text)}\n\n`);
        });
        return 0;
      }

      case 'remember': {
        const text = rest.join(' ').trim();
        if (text === '') throw new UsageError('remember: lipsește textul');
        const type = values.type;
        if (type !== undefined && !isMemoryType(type)) throw new UsageError(`tip de memorie necunoscut „${type}" (permise: ${MEMORY_TYPES.join(', ')})`);
        const result = await suite.remember({
          text,
          type,
          namespace,
          tags: parseList(values.tags),
          importance: values.importance !== undefined ? parseFloat01(values.importance, 0.5, 'importance') : undefined,
          supersedes: values.supersedes !== undefined ? parseInt10(values.supersedes, 0, 'supersedes') : undefined,
          title: values.title,
          expiresAt: parseDate(values.expires),
        });
        if (values.json) emit(result);
        else {
          process.stdout.write(`${ok('Memorat')} #${result.id} în „${namespace}"${result.superseded !== null ? `, înlocuiește #${result.superseded}` : ''}${result.embedded ? '' : dim(' (fără vector — lipsește cheia API)')}.\n`);
        }
        return 0;
      }

      case 'recall': {
        const query = rest.join(' ').trim();
        if (query === '') throw new UsageError('recall: lipsește interogarea');
        const hits = await suite.recall(query, {
          namespace: values.namespace,
          limit,
          mode: parseMode(values.mode),
          types: parseMemoryTypes(values.type),
          tags: parseList(values.tags),
        });
        if (values.json) {
          emit(hits);
          return 0;
        }
        if (hits.length === 0) {
          process.stdout.write(`${warn('Nicio memorie potrivită.')}\n`);
          return 0;
        }
        hits.forEach((h, i) => {
          const m = h.memory;
          const date = new Date(h.updatedAt).toISOString().slice(0, 10);
          const tags = m.tags.length > 0 ? ` ${dim(m.tags.map((t) => `#${t}`).join(' '))}` : '';
          process.stdout.write(`${bold(`${i + 1}.`)} #${h.documentId} ${paint('cyan', m.memoryType)} · ${date} · imp ${m.importance.toFixed(2)} · scor ${h.finalScore.toFixed(3)}${tags}\n`);
          process.stdout.write(`   ${snippet(h.text, 400)}\n\n`);
        });
        return 0;
      }

      case 'forget': {
        const id = parseInt10(rest[0], 0, 'id');
        if (id === 0) throw new UsageError('forget: dă id-ul memoriei');
        const done = suite.forget(id);
        if (values.json) emit({ id, forgotten: done });
        else process.stdout.write(done ? `${ok('Dezactivată')} memoria #${id}.\n` : `${warn('Nu există')} o memorie activă #${id}.\n`);
        return done ? 0 : 1;
      }

      case 'history': {
        const id = parseInt10(rest[0], 0, 'id');
        if (id === 0) throw new UsageError('history: dă id-ul memoriei');
        const chain = suite.history(id);
        if (values.json) {
          emit(chain);
          return 0;
        }
        if (chain.length === 0) {
          process.stdout.write(`${warn('Nu există')} memoria #${id}.\n`);
          return 1;
        }
        for (const m of chain) {
          const date = new Date(m.updatedAt).toISOString().slice(0, 10);
          const state = m.active ? ok('activă') : dim('înlocuită');
          process.stdout.write(`#${m.documentId} · ${m.memoryType} · ${date} · ${state}\n   ${snippet(m.text, 300)}\n`);
        }
        return 0;
      }

      case 'handoff': {
        const result = await suite.handoff({ namespace, query: values.query, limit, types: parseMemoryTypes(values.type) });
        if (values.json) emit(result.items);
        else process.stdout.write(result.markdown);
        return 0;
      }

      case 'stats': {
        const s = suite.stats();
        if (values.json) {
          emit(s);
          return 0;
        }
        const kv = (o: Record<string, number>): string => Object.entries(o).map(([k, v]) => `${k} ${v}`).join(', ') || '—';
        process.stdout.write(
          `${bold('Bază')}       ${s.dbPath} (${fmtBytes(s.dbBytes)})\n` +
            `${bold('Documente')}  ${s.documents.active} active / ${s.documents.total}  ·  ${kv(s.documents.byKind)}\n` +
            `${bold('Spații')}     ${kv(s.documents.byNamespace)}\n` +
            `${bold('Fragmente')}  ${s.chunks.total}, vectorizate ${s.chunks.embedded}, în așteptare ${s.chunks.pending}  ·  modele: ${s.chunks.models.join(', ') || '—'}\n` +
            `${bold('Memorii')}    ${s.memories.active} active / ${s.memories.total}  ·  ${kv(s.memories.byType)}\n` +
            `${bold('Cache')}      ${s.cache.entries} vectori\n`,
        );
        return 0;
      }

      default:
        throw new UsageError(`comandă necunoscută „${command}" — vezi dhs --help`);
    }
  } finally {
    suite.close();
  }
}

async function cmdModels(cfg: Config, json: boolean): Promise<number> {
  if (!cfg.apiKey) {
    process.stderr.write(`${bad('Lipsește GEMINI_API_KEY')} — pune-o în .env.local sau în mediu.\n`);
    return 1;
  }
  const models = await listEmbeddingModels(cfg.apiKey);
  if (json) {
    process.stdout.write(`${JSON.stringify(models, null, 2)}\n`);
    return 0;
  }
  for (const m of models) {
    const mark = m.name === cfg.embedModel ? ok(' ◀ configurat') : '';
    process.stdout.write(`${bold(m.name.padEnd(30))} intrare ≤ ${String(m.inputTokenLimit).padStart(5)} tokeni  ${dim(m.displayName)}${mark}\n`);
  }
  return 0;
}

async function cmdDoctor(cfg: Config): Promise<number> {
  let failures = 0;
  const line = (label: string, good: boolean | null, detail: string): void => {
    const mark = good === null ? dim('·') : good ? ok('✔') : bad('✘');
    if (good === false) failures++;
    process.stdout.write(`${mark} ${label.padEnd(22)} ${detail}\n`);
  };
  const [major] = process.versions.node.split('.').map(Number);
  line('Node', (major ?? 0) >= 24, `${process.versions.node} (necesar ≥ 24 pentru node:sqlite + TypeScript nativ)`);
  line('SQLite', true, `${process.versions.sqlite ?? '?'}`);
  try {
    const { DatabaseSync } = await import('node:sqlite');
    const db = new DatabaseSync(':memory:');
    db.exec("CREATE VIRTUAL TABLE t USING fts5(x, tokenize='unicode61 remove_diacritics 2')");
    db.close();
    line('FTS5', true, 'disponibil (BM25, diacritice pliate)');
  } catch (err) {
    line('FTS5', false, (err as Error).message);
  }
  const dbExists = existsSync(cfg.dbPath);
  line('Bază', null, `${cfg.dbPath} ${dbExists ? dim(`(${fmtBytes(statSync(cfg.dbPath).size)})`) : dim('(se creează la prima scriere)')}`);
  line('Model', null, `${cfg.embedModel} · ${cfg.embedDims} dimensiuni · ${cfg.rpm} RPM · ${cfg.tpm} TPM`);
  line('Cheie API', cfg.apiKey !== null, cfg.apiKey ? 'setată (GEMINI_API_KEY / GOOGLE_API_KEY)' : 'lipsă — doar căutare lexicală');
  if (cfg.apiKey) {
    const embedder = new GeminiEmbedder({ apiKey: cfg.apiKey, model: cfg.embedModel, dimensions: cfg.embedDims, maxRetries: 1 });
    const t0 = performance.now();
    try {
      const [v] = await embedder.embed([{ text: 'verificare dhs doctor' }], 'query');
      line('Embedding live', v !== undefined && v.length === cfg.embedDims, `${v?.length ?? 0} dimensiuni în ${Math.round(performance.now() - t0)} ms`);
    } catch (err) {
      line('Embedding live', false, (err as Error).message);
    }
  }
  return failures > 0 ? 1 : 0;
}

main(process.argv.slice(2))
  .then((code) => {
    process.exitCode = code;
  })
  .catch((err: unknown) => {
    const e = err as Error & { code?: string };
    if (e instanceof UsageError || e.code === 'ERR_PARSE_ARGS_UNKNOWN_OPTION' || e.code === 'ERR_PARSE_ARGS_INVALID_OPTION_VALUE') {
      process.stderr.write(`${bad('Eroare')}: ${e.message}\n${dim('dhs --help pentru sintaxă')}\n`);
    } else {
      process.stderr.write(`${bad('Eroare')}: ${e.message}\n`);
      if (process.env.DHS_DEBUG) process.stderr.write(`${e.stack ?? ''}\n`);
    }
    process.exitCode = 1;
  });
