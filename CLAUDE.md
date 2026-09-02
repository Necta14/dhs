# CLAUDE.md — DHS · Direct Handoff Suite

> Fișierul de față descrie **produsul DHS**. Regulile de lucru pentru agenți sunt în
> [`AGENTS.md`](AGENTS.md), jurnalul în [`docs/NOTES.md`](docs/NOTES.md), sarcinile în
> [`docs/BACKLOG.md`](docs/BACKLOG.md).
>
> ⚠️ **Implementarea NU a început.** Proiectul e definit, dar userul a cerut explicit să stabilim
> întâi deciziile din secțiunea [Decizii deschise](#decizii-deschise). Nu scrie cod de produs până
> nu sunt închise D1–D3.

## Ce este DHS

Unealtă **open source** care migrează rapid și eficient mediul de lucru al unui utilizator între
sisteme de operare:

- Windows → Linux
- Linux → Windows
- o distribuție Linux → altă distribuție
- o versiune de Windows → altă versiune

Ideea centrală: un **pachet de migrare portabil**, scris pe un mediu extern (SSD / HDD / USB).
**Cloud-ul nu e obligatoriu niciodată** — asta e o proprietate a produsului, nu o preferință.

### Pachetul de migrare

1. Fișiere personale ale utilizatorului
2. Configurații ale aplicațiilor
3. Manifest cu aplicațiile instalate
4. Configurații și preferințe ale utilizatorului
5. Metadate necesare pentru restaurare

### Fluxul, pe un exemplu (Windows 11 Pro → Arch Linux)

**Pe sursă (Windows):** instalezi DHS → alegi mediul extern → alegi datele personale și aplicațiile
→ DHS construiește pachetul (documente, imagini/video, profile de aplicații, configurații, lista
aplicațiilor instalate, reguli de conversie Windows → Linux).

**Pe destinație (după instalarea Arch):** instalezi DHS → alegi pachetul → DHS analizează manifestul
→ identifică alternativele Linux → instalează aplicațiile disponibile → restaurează fișierele și
configurațiile compatibile.

### Baza de date de aplicații

Proprie, nu împrumutată. Conține, per aplicație:

- dacă e suportată și sub ce identificatori e cunoscută pe fiecare platformă
- echivalentele între platforme (ex. Notepad++ → Kate / VSCodium)
- metodele de instalare: `winget` / `choco` / `scoop` pe Windows; `pacman` / `apt` / `dnf` /
  `flatpak` pe Linux
- locațiile de configurare pe fiecare sistem de operare
- cât de portabilă e configurația: identică / traductibilă / netraductibilă

## Domeniul versiunii 1

**În v1 intră doar:**

- creare backup
- restaurare backup
- manifest de aplicații
- detectare sistem de operare
- suport Windows și Linux
- CLI funcțional

**Amânat explicit după v1:** migrare automată de configurații complexe, sincronizare între
dispozitive, suport enterprise, integrare profundă cu distribuțiile, GUI.

Orice cerere din afara listei de v1 se scrie în `docs/BACKLOG.md`. Nu se implementează ad-hoc.

## Interfețe

- **CLI** — utilizatori avansați și automatizare. Se face primul.
- **GUI** — utilizatori obișnuiți. Se face după ce nucleul CLI e stabil.

Consecință de arhitectură, nefacultativă: **toată logica stă într-o bibliotecă de nucleu**, iar CLI-ul
și GUI-ul sunt învelișuri subțiri peste ea. Nicio regulă de business în stratul de interfață.

## ⛔ Regula #1 — datele utilizatorului sunt sacre

DHS umblă la întreaga viață digitală a cuiva: documente, poze, profile de browser, chei. O greșeală
nu înseamnă un bug, înseamnă date pierdute definitiv. Propuneri, **de confirmat** (vezi D4):

- restaurarea **nu suprascrie niciodată** fără confirmare explicită; implicit, conflictele se
  păstrează alături, nu se înlocuiesc
- orice operațiune distructivă are `--dry-run` și îl folosește ca mod de prezentare înainte de a
  cere confirmarea
- pachetul are **sume de control verificabile**; restaurarea refuză un pachet care nu se verifică
- procesul e **reluabil** după întrerupere (cablu smuls, baterie, disc plin) — fără pachete pe
  jumătate scrise care par valide
- **secretele** (chei SSH/GPG, parole din browser, token-uri) sunt tratate separat de restul datelor

## ⛔ Regula #2 — DHS nu conține AI

**Produsul livrat nu are AI, nu are RAG, nu are embeddings, nu cheamă niciun model.** DHS e o
unealtă deterministă de sistem: citește, împachetează, verifică, restaurează. Utilizatorul trebuie
să poată prezice exact ce face.

Potrivirea aplicațiilor între platforme se face **prin baza de date curată de la om**, cu
identificatori exacți — nu prin căutare semantică, nu prin „ghicit inteligent". Dacă o aplicație nu
e în bază, DHS spune că nu știe și o trece în raport; nu inventează un echivalent.

Consecință directă: nicio dependență de rețea la runtime, nicio cheie de API în produs, funcționare
completă offline.

## Sistemul intern de management al proiectului (RAG)

Directorul conține și o **unealtă internă de memorie și cunoștințe** (comanda `dhs`, `src/`,
`test/`): RAG local peste SQLite cu embeddings Gemini, construit pe 02.09.2026. Rolul ei e să țină
minte deciziile proiectului între sesiuni și între agenți.

**E infrastructură de lucru, nu parte din produs.** Nu se livrează, nu se publică odată cu DHS, nu
influențează arhitectura produsului. Vezi [Regula #2](#-regula-2--dhs-nu-conține-ai).

Spații de nume separate:

```bash
node src/cli.ts recall "scopul proiectului DHS" -n dhs     # produsul
node src/cli.ts handoff -n dhs                             # rezumat pentru sesiunea următoare
node src/cli.ts recall "cum e construit RAG-ul" -n mem     # unealta internă
```

`README.md` și `AGENTS.md` din repo descriu deocamdată unealta internă. Se rescriu pentru produs
când începe implementarea.

## Decizii deschise

Ordinea contează: D1 și D3 blochează scrierea de cod.

| # | Decizie | Stare |
|---|---|---|
| **D1** | Limbajul și stiva nucleului. Constrângerea tare: pe sistemul destinație, proaspăt instalat, **nu există niciun runtime** — deci binar unic, static. | deschis |
| **D3** | Cât de departe merge migrarea configurațiilor în v1: doar fișiere + manifest, sau și o listă curată de aplicații cu reguli scrise de mână. | deschis |
| **D4** | Criptarea pachetului și tratarea secretelor (chei, parole, token-uri). | deschis |
| **D5** | La restaurare: instalare automată a pachetelor, sau generarea unui script pe care userul îl revizuiește și îl rulează. | deschis |
| **D6** | Licența (GPLv3 vs Apache-2.0) și modelul de contribuție pentru baza de date de aplicații. | deschis |

## Decizii luate

- **D2 — relația cu unealta RAG · 02.09.2026.** Nu există conflict: DHS e produsul, RAG-ul e
  sistemul intern de management al proiectului. Produsul nu conține AI sau RAG
  ([Regula #2](#-regula-2--dhs-nu-conține-ai)). Unealta internă rămâne separată de codul care se
  livrează; când începe implementarea produsului, ea se mută în propriul repo, ca repo-ul public să
  conțină doar DHS.

## Convenții

- Documentație, mesaje de interfață și comentarii **în română**; identificatorii în engleză.
- Fiecare fază se termină cu commit. Ramura de lucru: `main`.
- Nu se șterg fișiere fără confirmare. Nu se rescrie cod funcțional.
- Răspunsuri scurte: concluzia în 1–3 rânduri, detaliile în documentație, nu în chat.
