import { promises as fs } from 'node:fs';
import { basename, extname, join, resolve, sep } from 'node:path';
import type { Store } from '../db/store.ts';
import { chunkText, isMarkdownExtension, toChunkInputs, type ChunkerOptions } from '../text/chunker.ts';
import { sha256 } from '../text/normalize.ts';

export type FileStatus = 'added' | 'updated' | 'unchanged' | 'skipped';

export interface FileEvent {
  path: string;
  status: FileStatus;
  chunks: number;
  reason?: string | undefined;
}

export interface IndexOptions {
  namespace?: string | undefined;
  /** Extensii acceptate (cu sau fără punct). Implicit: documente text/Markdown. */
  extensions?: readonly string[] | undefined;
  /** Nume de directoare ocolite oriunde în arbore. */
  excludeDirs?: readonly string[] | undefined;
  maxFileBytes?: number | undefined;
  /** Dezactivează documentele din același rădăcină+spațiu care nu mai există pe disc. Implicit true. */
  prune?: boolean | undefined;
  chunker?: ChunkerOptions | undefined;
  onFile?: ((event: FileEvent) => void) | undefined;
  now?: number | undefined;
}

export interface IndexReport {
  scanned: number;
  added: number;
  updated: number;
  unchanged: number;
  skipped: number;
  pruned: number;
  chunks: number;
  reusedEmbeddings: number;
}

export const DEFAULT_EXTENSIONS: readonly string[] = ['.md', '.mdx', '.markdown', '.txt', '.rst', '.adoc', '.org'];
export const DEFAULT_EXCLUDED_DIRS: readonly string[] = [
  'node_modules', '.git', '.next', 'dist', 'build', 'coverage', '.turbo', '.cache', '.venv', '__pycache__', 'data',
];

/** Fișiere care nu se indexează niciodată, oricare ar fi extensia cerută (Regula #1: secrete). */
const FORBIDDEN_FILE =
  /^(\.env(\..*)?|.*\.(pem|key|p12|pfx|jks)|id_(rsa|dsa|ecdsa|ed25519)(\.pub)?|.*credentials.*\.json|.*secrets?.*\.(json|ya?ml|toml|env))$/i;

export function isForbiddenFile(name: string): boolean {
  return FORBIDDEN_FILE.test(name);
}

/**
 * Indexare incrementală: fiecare fișier are un hash de conținut; dacă nu s-a schimbat, nu se
 * re-fragmentează. Fragmentele noi identice cu cele vechi își păstrează vectorul. Fișierele
 * dispărute din rădăcinile date sunt dezactivate (prune), nu șterse.
 */
export async function indexPaths(store: Store, paths: readonly string[], opts: IndexOptions = {}): Promise<IndexReport> {
  const namespace = opts.namespace ?? 'default';
  const extensions = new Set((opts.extensions ?? DEFAULT_EXTENSIONS).map((e) => (e.startsWith('.') ? e : `.${e}`).toLowerCase()));
  const excluded = new Set(opts.excludeDirs ?? DEFAULT_EXCLUDED_DIRS);
  const maxBytes = opts.maxFileBytes ?? 2 * 1024 * 1024;
  const now = opts.now ?? Date.now();
  const report: IndexReport = { scanned: 0, added: 0, updated: 0, unchanged: 0, skipped: 0, pruned: 0, chunks: 0, reusedEmbeddings: 0 };

  for (const p of paths) {
    const root = resolve(p);
    const st = await fs.stat(root);
    if (st.isDirectory()) {
      const files = await collectFiles(root, extensions, excluded);
      const seen = new Set(files);
      for (const file of files) await indexFile(store, file, namespace, maxBytes, opts, now, report);
      if (opts.prune ?? true) {
        const prefix = root.endsWith(sep) ? root : root + sep;
        for (const doc of store.listDocuments({ namespace, kinds: ['file'], uriPrefix: prefix })) {
          if (!seen.has(doc.uri)) {
            store.setDocumentActive(doc.id, false, now);
            report.pruned++;
          }
        }
      }
    } else {
      await indexFile(store, root, namespace, maxBytes, opts, now, report);
    }
  }
  return report;
}

async function collectFiles(root: string, extensions: ReadonlySet<string>, excluded: ReadonlySet<string>): Promise<string[]> {
  const out: string[] = [];
  for await (const entry of fs.glob('**/*', {
    cwd: root,
    withFileTypes: true,
    exclude: (dirent) => excluded.has(dirent.name),
  })) {
    if (!entry.isFile()) continue;
    const abs = join(entry.parentPath, entry.name);
    const rel = abs.slice(root.length + 1);
    if (rel.split(/[\\/]/).some((seg) => excluded.has(seg))) continue;
    if (!extensions.has(extname(entry.name).toLowerCase())) continue;
    if (isForbiddenFile(entry.name)) continue;
    out.push(abs);
  }
  out.sort();
  return out;
}

async function indexFile(
  store: Store,
  file: string,
  namespace: string,
  maxBytes: number,
  opts: IndexOptions,
  now: number,
  report: IndexReport,
): Promise<void> {
  report.scanned++;
  const name = basename(file);
  const emit = (status: FileStatus, chunks: number, reason?: string): void => {
    report[status]++;
    opts.onFile?.({ path: file, status, chunks, reason });
  };
  if (isForbiddenFile(name)) return emit('skipped', 0, 'fișier de secrete');

  const st = await fs.stat(file);
  if (st.size > maxBytes) return emit('skipped', 0, `prea mare (${st.size} B)`);
  const buf = await fs.readFile(file);
  if (buf.subarray(0, 8192).includes(0)) return emit('skipped', 0, 'binar');
  const content = buf.toString('utf8');
  if (content.trim() === '') return emit('skipped', 0, 'gol');

  const ext = extname(file);
  const title = extractTitle(content, name);
  const up = store.upsertDocument(
    {
      uri: file,
      kind: 'file',
      namespace,
      title,
      contentHash: sha256(content),
      metadata: { path: file, ext, size: st.size, mtime: Math.floor(st.mtimeMs) },
    },
    now,
  );
  if (!up.changed) return emit('unchanged', 0);

  const chunks = toChunkInputs(chunkText(content, { ...(opts.chunker ?? {}), title, markdown: isMarkdownExtension(ext) }));
  const result = store.replaceChunks(up.id, chunks);
  report.chunks += result.inserted;
  report.reusedEmbeddings += result.reusedEmbeddings;
  emit(up.created ? 'added' : 'updated', chunks.length);
}

function extractTitle(content: string, fileName: string): string {
  const m = /^#\s+(.+?)\s*#*\s*$/m.exec(content.slice(0, 4000));
  if (m?.[1]) return m[1].trim();
  return fileName.replace(/\.[^.]+$/, '');
}
