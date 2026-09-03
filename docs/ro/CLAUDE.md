# CLAUDE.md — DHS · Direct Handoff Suite

> Traducere în română. Versiunea de referință e cea în engleză: [`../../CLAUDE.md`](../../CLAUDE.md)

> Fișierul de față descrie **produsul DHS**. Regulile de lucru pentru agenți sunt în
> [`AGENTS.md`](AGENTS.md), jurnalul în [`NOTES.md`](NOTES.md), sarcinile în
> [`BACKLOG.md`](BACKLOG.md).
>
> **Stare la 03.09.2026.** Nucleul pentru fișiere e implementat și **verde pe Codespaces**:
> `scan`, `backup`, `verify`, `list`, `restore` — teste unitare, detector de curse și un flux
> complet backup → restore cu comparare bit cu bit. Lipsesc baza de aplicații, detectarea
> aplicațiilor și `plan`. Testele se rulează **doar** pe Codespaces (AGENTS.md, regula 7);
> cum: [`TESTARE.md`](TESTARE.md).

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

Orice cerere din afara listei de v1 se scrie în `BACKLOG.md`. Nu se implementează ad-hoc.

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

## ⛔ Regula #4 — fără balast

DHS nu are voie să devină bloatware. Bugetele sunt praguri, nu sugestii:

| | Buget | Acum |
|---|---|---|
| Binar CLI, dezbrăcat (`-ldflags "-s -w"`) | ≤ 15 MiB | **4,2 MiB** Linux · **4,5 MiB** Windows (cu age, zstd, xz) |
| Procese în fundal, servicii, daemoni | **zero** | zero |
| Telemetrie, „statistici anonime", actualizator automat | **zero** | zero |
| Apeluri de rețea la rulare | **zero** | zero |
| Runtime de instalat pe sistemul destinație | **niciunul** | niciunul |
| Pornire la rece | < 100 ms | ~10 ms |

Fiecare dependență nouă se justifică. Un pachet Go care deschide un socket nu intră în produs.

## Sistemul intern de management al proiectului (RAG)

Unealta de memorie folosită de agenți s-a mutat, conform D2, în **`~/dhs-memory`** — repo propriu.
Nu face parte din produs, nu se livrează, nu influențează arhitectura lui.

```bash
cd ~/dhs-memory
node src/cli.ts recall "de ce am ales Go" -n dhs   # deciziile produsului
node src/cli.ts handoff -n dhs                     # rezumat pentru sesiunea următoare
node src/cli.ts recall "cum e construit RAG-ul" -n mem
```

## Stiva

**Go** — binar static unic, fără runtime pe sistemul destinație. Baza de date de aplicații intră în
binar prin `go:embed`, deci DHS funcționează complet offline.

**Windows și Linux se dezvoltă separat, dar din același repo.** Etichetele de build fac treaba:
codul specific stă doar în `_linux.go` și `_windows.go`, iar binarul de Linux **nu conține niciun
octet** din codul de Windows. Nu plătești pentru platforma cealaltă. Cross-compile-ul e o comandă,
fără toolchain suplimentar:

```bash
go build ./cmd/dhs                 # Linux
GOOS=windows go build ./cmd/dhs    # Windows, de pe Linux
```

**GUI-ul e proces separat, care vorbește cu CLI-ul prin JSON** (`dhs <comandă> --json`). Așa fiecare
platformă își primește interfața ei nativă, scrisă în ce i se potrivește, fără să dublăm logica și
fără ca un utilizator de Linux să descarce cod de Windows. Vezi D9. Există un prototip pentru Linux:
`gui/linux/dhs-gui.py` (GTK4 + libadwaita prin PyGObject).

- [`ARHITECTURA.md`](ARHITECTURA.md) — formatul pachetului, structura modulelor, suprafața CLI,
  schema bazei de aplicații
- [`COMPRESIE.md`](COMPRESIE.md) — cele trei niveluri de compresie, estimarea dimensiunii înainte
  de backup, research-ul pe compresia „extremă"
- [`LICENTA.md`](LICENTA.md) — opțiunile de licență, modelul de contribuție, ce spune Cyber
  Resilience Act despre open source
- [`TESTARE.md`](TESTARE.md) — sesiunea dedicată de teste pe Codespaces

## ⛔ Regula #3 — nimic cu pierderi

Un fișier restaurat trebuie să fie **identic bit cu bit** cu originalul. Migrăm pozele de la nunta
cuiva, nu asseturi de joc. Orice tehnică de compresie care nu poate garanta reconstrucția exactă e
în afara produsului, oricât ar câștiga la dimensiune.

## Decizii luate

- **D1 — stiva · 02.09.2026: Go.** Binar static unic, cross-compile din Arch către Windows cu o
  singură comandă, stdlib care acoperă exact nevoile (arhive, hashing, filesystem, procese),
  registry Windows prin `x/sys`. Curbă mică de învățare → contribuitori mai ușor de găsit.
- **D2 — relația cu unealta RAG · 02.09.2026.** Nu există conflict: DHS e produsul, RAG-ul e
  sistemul intern de management al proiectului. Produsul nu conține AI sau RAG
  ([Regula #2](#-regula-2--dhs-nu-conține-ai)). Unealta internă rămâne separată de codul care se
  livrează; când începe implementarea produsului, ea se mută în propriul repo, ca repo-ul public să
  conțină doar DHS.
- **D3 — configurații în v1 · 02.09.2026: fișiere + manifest + listă mică.** Copiere de fișiere,
  manifest de aplicații, plus 10–15 aplicații cu configurații cu adevărat portabile. Restul se
  arhivează și se raportează, **nu se traduce**. Fără promisiuni pe care nu le putem ține.
- **D4 — securitate · 02.09.2026: criptat implicit, secrete opt-in.** Pachetul e criptat cu parolă
  din start. Secretele (chei SSH/GPG, parole din browser, token-uri cloud) sunt **excluse implicit**
  și incluse doar cu opt-in explicit și avertisment. Un SSD pierdut nu devine o scurgere totală.
  **Precizat 02.09.2026:** parola acoperă **toate** fișierele din pachet, iar criptarea e alegerea
  userului — pornită implicit, dar se poate opri. Secretele, când sunt incluse, stau într-o secțiune
  cu **parolă proprie**, ca pachetul să poată fi dat cuiva pentru documente fără a-i da și cheile.
- **D7 — modul necriptat · 02.09.2026, decurge din D4.** Fiindcă criptarea e o alegere, tensiunea
  dispare: cine vrea un pachet deschizabil pe orice calculator, fără DHS, alege nivelul 1 (ZIP) și
  oprește criptarea, cu un avertisment vizibil. Cine vrea confidențialitate lasă criptarea pornită și
  acceptă că îi trebuie DHS ca să-l deschidă.
- **Regula #3 — nimic cu pierderi · 02.09.2026.** Vezi mai sus. Siguranța preprocesării nu vine din
  a evita fișierele „importante" — DHS n-are cum să știe care sunt — ci din **verificare**: fiecare
  flux preprocesat se recompune și se compară octet cu octet pe loc, iar la restaurare fișierul
  refăcut se verifică față de SHA-256-ul originalului.
- **D6 — licența · 02.09.2026: Apache-2.0.** Permisivă, dar cu `NOTICE` (atribuire care se duce în
  orice derivat), clauză de marcă (§6 — nimeni nu poate numi forkul „DHS") și cesiune de brevete.
  Deliberat **fără** clauză etică de utilizare: aceea ar scoate DHS din definiția open source și
  l-ar face nepublicabil în Debian, Fedora, Arch, openSUSE și nixpkgs — exact publicul pentru care
  există unealta. Poziția proiectului se apără altfel: [`TRADEMARK.md`](TRADEMARK.md) interzice
  folosirea **numelui** într-un produs care implementează verificarea vârstei, iar
  [`VALUES.md`](VALUES.md) o spune public. Dreptul mărcilor se poate aplica; „diabolic" nu.
  Contribuții prin **DCO**; baza de aplicații primește **CC-BY-4.0**.
- **Structura codului · 02.09.2026.** Windows și Linux se dezvoltă separat prin etichete de build,
  nu prin repo-uri separate. Numele de pachete și identificatorii sunt în engleză (limba
  comentariilor și a mesajelor a fost stabilită ulterior, prin D10).
- **D5 — instalare la restaurare · 02.09.2026: plan aprobat, apoi rulare.** DHS arată exact ce
  instalează, din ce sursă și cu ce comenzi; userul aprobă o dată, apoi rulează automat. Nimeni nu
  execută orbește comenzi de root generate dintr-un fișier de pe un stick.
- **D10 — limba · 03.09.2026: engleza e principală.** Codul, comentariile, documentația și CLI-ul
  sunt în engleză. Româna se menține ca traducere: [`README.ro.md`](../../README.ro.md), `docs/ro/`
  și catalogul CLI `internal/i18n/ro.go` (ales cu `DHS_LANG=ro` sau din localizare). Discuția cu
  menținătorul poate avea loc în română. Motiv: proiectul e open source și se adresează oamenilor
  care trec între sisteme de operare oriunde; documentația de referință trebuie să fie citibilă de
  orice contribuitor.
- **Deținătorul din NOTICE · 03.09.2026.** Linia de copyright din `NOTICE` poartă pseudonimul public
  al menținătorului, Necta (https://github.com/Necta14) — o persoană, nu o entitate. Deblochează
  prima publicare.

## Decizii deschise

| # | Decizie | Stare |
|---|---|---|
| **D8** | Cum implementăm preprocesarea de tip preflate, când ajungem la ea: reimplementare în Go (binar curat, câteva săptămâni) sau `preflate-rs` prin cgo (rapid, dar cere `mingw` pentru Windows). **Nu se decide acum** — v1 livrează nivelul 3 ca LZMA2 fără preprocesare, iar formatul lasă loc pentru ea. | amânat până la etapa respectivă |
| **D9** | Ce trusă grafică pe fiecare platformă. Candidați: pe Linux **GTK4 + libadwaita** (există un prototip PyGObject în `gui/linux/dhs-gui.py`; `gotk4` ar fi drumul în Go, nativ pe GNOME, legat de bibliotecile sistemului, deja instalate); pe Windows **Win32 nativ** sau WinUI. Fiindcă GUI-ul e proces separat care vorbește JSON cu CLI-ul, decizia **nu blochează nimic** și se ia când ajungem la ea. | amânat, GUI-ul vine după v1 |

## Convenții

- Codul, comentariile, documentația și CLI-ul sunt în **engleză**. Româna se menține ca traducere:
  `README.ro.md`, `docs/ro/` și catalogul CLI `internal/i18n/ro.go` (ales cu `DHS_LANG=ro` sau din
  localizare). Discuția cu menținătorul poate avea loc în română.
- Fiecare fază se termină cu commit. Ramura de lucru: `main`.
- Nu se șterg fișiere fără confirmare. Nu se rescrie cod funcțional.
- Răspunsuri scurte: concluzia în 1–3 rânduri, detaliile în documentație, nu în chat.
