# AGENTS.md — Direct Handoff Suite

Reguli pentru orice agent (orice model/unealtă) care lucrează pe acest repo. Contextul general al
userului (4 aplicații în paralel, răspunsuri scurte, secrete) e în `~/CLAUDE.md`; aici sunt doar
regulile specifice proiectului.

1. **Secrete.** `.env`, `.env.local`, orice `.env*`: nu le deschizi, nu le citești, nu le afișezi,
   nu le editezi, nu le comiți. Cheia intră în cod doar prin `process.env`. CLI-ul încarcă
   `.env.local`/`.env` singur, prin `process.loadEnvFile`. Indexatorul refuză aceste fișiere după
   nume (`isForbiddenFile`) înainte să le deschidă — nu slăbi regula aia.
2. **Fără dependențe la runtime.** Doar `node:sqlite`, `fetch`, `node:crypto`, `node:fs`. Dacă ai
   nevoie de ceva, scrie-l. Dev deps: `typescript`, `@types/node`.
3. **TypeScript rulează nativ** (Node ≥ 24, type stripping). Deci: importuri cu extensia `.ts`,
   sintaxă *erasable* — fără `enum`, `namespace`, parametri-proprietăți în constructori. `tsconfig`
   are `erasableSyntaxOnly` și te oprește dacă greșești.
4. **Strict.** `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`. Fără `any`.
5. **Testele nu ating rețeaua.** Folosește `HashEmbedder` (determinist) și `fetch` fals pentru
   Gemini. `npm run check` trebuie să fie verde înainte de commit.
6. **Verifică pe viu ce ține de API.** `node src/cli.ts doctor` face un embedding real. Dacă
   schimbi ceva în `src/embed/gemini.ts`, rulează-l.
7. **Baza implicită e globală** (`~/.local/share/dhs/dhs.sqlite`). În dezvoltare folosește
   `--db data/dhs.sqlite` sau `DHS_DB_PATH`, ca să nu murdărești memoria reală.
8. **Sincronizare.** La început de sesiune citește `docs/NOTES.md` și `docs/BACKLOG.md`. Task-urile
   mari se scriu în backlog, nu se execută ad-hoc. La final, notează în `docs/NOTES.md` ce ai
   schimbat și de ce.
9. **Limba.** Documentație, mesaje de CLI și comentarii în română; identificatorii în engleză.
10. **Ramura de lucru e `main`.** Commit după fiecare fază, cu mesaj care spune *ce* și *de ce*.
