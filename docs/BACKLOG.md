# BACKLOG — Direct Handoff Suite

Ordinea e o propunere; primele trei dau cel mai mult pentru scopul „handoff între agenți".

## Faza 2 — integrare cu agenții
- [ ] **Server MCP** (`dhs mcp`) care expune `recall`, `remember`, `search`, `handoff` ca unelte,
      ca Claude Code / Codex să le cheme direct. Transport stdio; un singur fișier, fără SDK dacă
      protocolul minim se poate scrie de mână, altfel `@modelcontextprotocol/sdk` ca dep opțională.
- [ ] **Import KB existent**: `dhs index ~/atm/docs/kb -n atm` + extragerea deciziilor din
      `NOTES.md`/`PROBLEMS.md` ca memorii (`decision`/`problem`), cu script dedicat.
- [ ] **Handoff „la închidere"**: comandă care primește transcriptul/rezumatul sesiunii și
      salvează o memorie de tip `handoff` + propune decizii/probleme de reținut.

## Calitate a regăsirii
- [ ] **Reranker LLM opțional** (`gemini-flash-lite`, free tier) peste top-30 din hibrid; interfață
      `Reranker`, oprit implicit.
- [ ] **Deduplicare la `remember`**: dacă există o memorie activă cu cosinus > 0,95, propune
      `--supersedes` în loc să dubleze.
- [ ] **Contextual retrieval**: un rând de context generat de LLM per fragment (scump la indexare;
      doar pentru spații mici și valoroase).
- [ ] Evaluare: set mic de întrebări/răspunsuri pe KB-ul ATM, măsurat MRR pentru hibrid vs vector
      vs lexical, ca reglajele (k RRF, ponderi, prefix ≥ 5) să fie justificate cu cifre.

## Scalare
- [ ] **sqlite-vec** sau cuantizare int8 a matricei când un spațiu trece de ~200k fragmente
      (acum: produs scalar exact, Float32, ~30 MB / 10k fragmente la 768 dimensiuni).
- [ ] Contor **RPD** (cereri pe zi) pe lângă RPM/TPM — free tier are și plafon zilnic.
- [ ] `dhs watch <dosar>` cu `fs.watch` pentru indexare continuă.
- [ ] `dhs migrate-model` — re-vectorizare controlată la schimbarea modelului, cu raport de cost.

## Igienă
- [ ] Export/backup (`dhs export -n atm > atm.jsonl`) și import.
- [ ] `dhs gc` — șterge fizic documentele inactive mai vechi de N zile și intrările de cache orfane.
- [ ] Pagină de ajutor per comandă (`dhs help search`).
