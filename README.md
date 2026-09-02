# Direct Handoff Suite (`dhs`)

Memorie și cunoștințe locale pentru agenți: un RAG hibrid (vectori + BM25) peste **SQLite**, cu
embeddings **Gemini** (Google AI Studio, free tier) și un strat de **memorie** cu importanță,
prospețime, etichete, înlocuire și expirare. Scopul: orice agent (Claude Code, Codex, un script)
poate să *își amintească* ce au decis ceilalți și să *predea* contextul următorului, fără să
recitească zeci de fișiere.

- **Zero infrastructură.** Un singur fișier SQLite, `node:sqlite` încorporat în Node ≥ 24, fără
  dependențe la runtime. TypeScript rulează nativ (fără pas de build).
- **Eficient cu API-ul.** Cache de vectori pe hash de conținut, loturi de până la 100 de texte,
  limitator RPM/TPM pentru free tier, reîncercare care respectă `retryDelay`. Re-indexarea unui
  fișier neschimbat costă **zero** apeluri.
- **Căutare hibridă.** Index vectorial în memorie (produs scalar exact peste o matrice `Float32`
  contiguă) + FTS5/BM25 cu diacritice pliate, fuzionate prin Reciprocal Rank Fusion, opțional MMR.
- **Memorie, nu doar documente.** `remember` / `recall` / `forget` / `history` / `handoff`, cu scor
  = relevanță × (1 + importanță) × prospețime.

## Cerințe

- Node **≥ 24** (testat pe 26.7). `npm install` aduce doar TypeScript și `@types/node`, pentru
  verificarea tipurilor.
- O cheie Gemini API (Google AI Studio) în `GEMINI_API_KEY` sau `GOOGLE_API_KEY`. Fără cheie,
  totul merge în modul **lexical** (BM25), iar vectorizarea se poate face mai târziu cu `dhs embed`.

## Instalare

```bash
npm install
cp .env.example .env.local && $EDITOR .env.local   # pune cheia; CLI-ul citește .env.local și .env din cwd
node src/cli.ts doctor                             # sau: npm run dhs -- doctor
npm link                                           # opțional: comanda globală `dhs`
```

`.env.local` e ignorat de git (`.gitignore`); `.env.example` e șablonul care se comite.

Baza implicită e **globală**: `~/.local/share/dhs/dhs.sqlite` (respectă `XDG_DATA_HOME`), ca aceeași
memorie să fie văzută din orice proiect. Se schimbă cu `DHS_DB_PATH` sau `--db`.

## CLI

```bash
dhs index ~/proiect/docs -n atm            # indexare incrementală + vectorizare
dhs search "cum tratăm secretele" -n atm   # hibrid; --mode vector|lexical; --mmr 0.7; --json
dhs remember "Am migrat pe Postgres 17 pe VPS." -n atm -t decision --tags db,infra --importance 0.9
dhs remember "..." --supersedes 12         # înlocuiește memoria #12 (rămâne în istoric)
dhs recall "unde e baza de date" -n atm    # relevanță × importanță × prospețime; -t decision; --tags infra
dhs history 12                             # lanțul de înlocuiri
dhs forget 12                              # dezactivare logică
dhs handoff -n atm                         # rezumat Markdown pentru agentul următor (-q "temă" îl focalizează)
dhs embed                                  # vectorizează ce a rămas în așteptare
dhs stats · dhs models · dhs doctor
```

Spații de nume (`-n`) separă proiectele. Tipuri de memorie: `fact`, `decision`, `preference`,
`episode`, `task`, `problem`, `handoff`.

## Ca bibliotecă

```ts
import { Suite, GeminiEmbedder, readConfig, loadEnvFiles } from './src/index.ts';

loadEnvFiles();
const cfg = readConfig();
const suite = Suite.open({
  dbPath: cfg.dbPath,
  embedder: cfg.apiKey ? new GeminiEmbedder({ apiKey: cfg.apiKey, model: cfg.embedModel, dimensions: cfg.embedDims }) : null,
});

await suite.index(['./docs'], { namespace: 'atm' });
const hits = await suite.search('publicare pe VPS', { namespace: 'atm', limit: 5, mmrLambda: 0.7 });
await suite.remember({ text: 'Deploy-ul se face cu infra/deploy.sh pe server.', type: 'decision', namespace: 'atm', importance: 0.8 });
const memories = await suite.recall('cum publicăm', { namespace: 'atm' });
const { markdown } = await suite.handoff({ namespace: 'atm' });
suite.close();
```

Pentru teste sau mod offline există `HashEmbedder` (determinist, fără rețea).

## Arhitectură

```
 index                                     search
 ─────                                     ──────
 fișiere ─► filtru (ext, .env*, binare)    interogare ─► vector (Gemini, cache LRU) ─► index în memorie ─┐
   │                                                  └► FTS5 MATCH (tokeni citați, prefixe) ─► BM25 ────┤
   ▼                                                                                                     ▼
 hash conținut ─► neschimbat? ─► stop                                              RRF (k=60) ─► MMR? ─► hidratare
   │
   ▼
 chunker Markdown (titluri, blocuri de cod, overlap) ─► hash/fragment ─► SQLite (chunks + FTS5 prin triggere)
   │
   ▼
 embedPending: cache(hash) ─► lipsuri ─► Gemini batchEmbedContents (≤100/lot, RPM/TPM, retry) ─► BLOB Float32
```

- **`src/db/store.ts`** — tot SQL-ul: documente, fragmente, cache, memorii, filtre, statistici.
  WAL, `synchronous=NORMAL`, statement-uri pregătite, tranzacții pe lot. `version` crește la orice
  scriere relevantă; indexul vectorial se reconstruiește leneș când o vede schimbată.
- **`src/text/chunker.ts`** — fragmente ≤ 1500 caractere, tăiate de preferință la granițe de
  secțiune; blocurile de cod sunt atomice; suprapunere doar în interiorul unei secțiuni; fiecare
  fragment poartă calea de titluri (`Titlu > H2 > H3`), care intră și în textul vectorizat, și în FTS.
- **`src/embed/gemini.ts`** — `gemini-embedding-2` la 768 dimensiuni (MRL), `RETRIEVAL_DOCUMENT`
  cu `title` / `RETRIEVAL_QUERY`, vectori normalizați. Limitele implicite (90 RPM / 28 000 TPM) sunt
  sub free tier; se ajustează din `DHS_EMBED_RPM` / `DHS_EMBED_TPM`.
- **`src/search/vector.ts`** — matrice contiguă + heap Top-K: ~100k fragmente × 768 dimensiuni în
  zeci de ms, fără extensii native. Filtrele de document se aplică în timpul scanării.
- **`src/search/hybrid.ts`** — RRF ponderat și MMR.
- **`src/memory/memory.ts`** — `remember` (tranzacție unică, `supersedes` dezactivează memoria
  veche), scor final, grupare pentru `handoff`.

Detalii despre decizii și ce urmează: [`docs/NOTES.md`](docs/NOTES.md), [`docs/BACKLOG.md`](docs/BACKLOG.md).
Regulile pentru agenții care lucrează pe acest repo: [`AGENTS.md`](AGENTS.md).

## Variabile de mediu

| Nume | Implicit | Rol |
|---|---|---|
| `GEMINI_API_KEY` / `GOOGLE_API_KEY` | — | cheia Google AI Studio |
| `DHS_DB_PATH` | `~/.local/share/dhs/dhs.sqlite` | baza SQLite |
| `DHS_EMBED_MODEL` | `gemini-embedding-2` | modelul de embedding |
| `DHS_EMBED_DIMS` | `768` | dimensiunea MRL (768 / 1536 / 3072) |
| `DHS_EMBED_RPM` / `DHS_EMBED_TPM` | `90` / `28000` | plafoanele limitatorului |
| `DHS_DEBUG` | — | stack trace la erori |

Schimbarea modelului sau a dimensiunii face toate fragmentele „în așteptare" pentru noul model;
`dhs embed` le refac (cache-ul e pe model, deci se cheamă API-ul o singură dată per text).

## Dezvoltare

```bash
npm run check        # tsc --noEmit + node --test (fără rețea; embedder determinist)
```
