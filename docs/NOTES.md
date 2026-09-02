# NOTES — Direct Handoff Suite

Jurnal de sesiuni. Cel mai nou sus.

## 2026-09-02 — faza 1: nucleul RAG + memorie (Claude Fable 5.1)

**Ce există.** CLI `dhs` și bibliotecă `Suite`: indexare incrementală de fișiere, fragmentare
Markdown, embeddings Gemini cu cache/loturi/limitator, căutare hibridă (vector în memorie + FTS5,
RRF, MMR), memorie cu tipuri/importanță/etichete/înlocuire/expirare, `handoff` Markdown.
`npm run check` verde: tipuri + 40 de aserțiuni în 6 fișiere de test, fără rețea.

**Decizii și motivele lor.**
- *SQLite prin `node:sqlite`, nu Postgres/pgvector.* Zero infrastructură, o singură bază pentru
  toate proiectele, FTS5 inclus în Node 26. Postgres-ul de pe VPS rămâne al aplicațiilor.
- *Index vectorial exact, în memorie, fără extensii native.* La scara actuală (mii–zeci de mii
  de fragmente) o căutare e sub 10 ms; sqlite-vec/cuantizare sunt în backlog pentru >200k.
- *`gemini-embedding-2` la 768 dimensiuni.* Verificat pe cheia userului: există și e GA (8192
  tokeni intrare, întoarce vectori deja normalizați și `usageMetadata`). `gemini-embedding-001` la
  768 nu e normalizat — normalizăm oricum, mereu.
- *Cheia cache-ului = model + dimensiune + titlu + text.* Schimbarea modelului nu invalidează
  cache-ul vechi, doar îl ocolește.
- *Interogările FTS se construiesc, nu se transmit.* Tokeni citați (operatorii userului devin
  literali), prefix pentru tokeni ≥ 5 caractere (flexiuni RO), stopwords RO/EN, AND cu fallback OR.
- *Baza implicită globală (XDG).* Memoria e a userului, nu a repo-ului.
- *Regula #1 se aplică și la indexare.* `.env*`, chei, credențiale sunt refuzate după nume, înainte
  de `open`.

**Verificat pe viu.** `dhs doctor` cu cheia reală: embedding 768 dimensiuni în ~1,1 s. `dhs models`
listează `gemini-embedding-001`, `gemini-embedding-2-preview`, `gemini-embedding-2`.

**De făcut de user.** `.env.local` cu `GEMINI_API_KEY=…` (agentul nu scrie fișiere `.env*`).

**Capcane întâlnite.**
- `"parola"*` nu prinde „parolele" — prefixul e pe litere, nu pe rădăcină lexicală. Testul a fost
  corectat, nu codul; un stemmer RO ar fi următorul pas dacă se dovedește necesar.
- Un heredoc cu un caracter NUL a fost respins de unealta de shell; fișierele sursă se scriu cu
  unealta de scriere, nu prin `cat <<EOF`.
