# BACKLOG — DHS

> Traducere în română. Versiunea de referință e cea în engleză: [`../BACKLOG.md`](../BACKLOG.md)

Ce nu intră în v1. Ordinea în fiecare secțiune e o propunere, nu un angajament.

> **04.09.2026:** baza de aplicații, detectarea, `dhs plan` și `dhs install` sunt scrise; vezi
> secțiunea „v1” din versiunea engleză pentru starea la zi. Rămân: umplerea și verificarea bazei,
> detectarea pe Windows real.

## v1 — ce mai lipsește din nucleu

- [x] `dhs scan` — inventar, clasificare, excluderi, estimare, verificarea destinației
- [ ] `--precise` — eșantionare pentru raport măsurat + hash pentru deduplicare
- [x] `internal/pack` — formatul: volume de 3,5 GiB, blocuri solide pe clasă, dedup pe fișier, jurnal, sume, index criptat, index redundant per volum — **verde pe Codespaces** (vezi `TESTARE.md`)
- [x] `internal/passphrase` — `age` cu frază de acces — **verde pe Codespaces**
- [ ] Secțiune separată pentru secrete, cu parolă proprie (D4) — formatul o permite, neimplementat
- [ ] Reluarea unui backup întrerupt din jurnal + `index.dhsi` parțial — formatul o permite, comanda nu există
- [x] `dhs backup` — scrierea pachetului — **verde pe Codespaces**
- [x] `dhs verify` — verificarea integrală, fără extragere — **verde pe Codespaces**
- [x] `dhs list` — conținutul pachetului, rezumat sau complet — **verde pe Codespaces**
- [x] `dhs restore` — plan (destinații, conflicte, nume adaptate, coliziuni de majuscule) → confirmare → scriere prin temporar + verificare hash + rename — **verde pe Codespaces**
- [ ] `internal/appdb` — baza de aplicații (TOML + `go:embed`) și interogarea ei
- [ ] Detectarea aplicațiilor instalate: `pacman`/`dpkg`/`rpm`/`flatpak`/`snap` pe Linux, registry + `winget` pe Windows
- [ ] `dhs plan` — manifest + appdb → plan de restaurare, fără să atingă nimic
- [ ] `dhs report` — ce a rămas necunoscut sau netradus
- [ ] Cele 10–15 aplicații cu configurații portabile (D3)
- [ ] Împărțirea pachetului pe mai multe medii

## Calitate și infrastructură

- [ ] CI: `go vet`, `go test`, build pentru linux/amd64, linux/arm64, windows/amd64, windows/arm64
- [ ] Buget de dimensiune verificat în CI (Regula #4: binar ≤ 15 MiB)
- [ ] Teste de integrare pe VM-uri: o migrare Windows → Linux dusă până la capăt
- [ ] `CONTRIBUTING.md` cu regula DCO și cum se adaugă o aplicație în bază
- [x] Numele din `NOTICE` — stabilit 03.09.2026: Necta (https://github.com/Necta14)

## După v1

- [ ] **GUI** — proces separat, vorbește JSON cu CLI-ul (D9). Linux: GTK4 + libadwaita; există un prototip PyGObject în `gui/linux/dhs-gui.py`, versiunea de producție poate fi rescrisă cu `gotk4`. Windows: Win32 nativ sau WinUI
- [ ] **Preprocesare tip preflate** pentru nivelul 3 (D8): docx/xlsx/pptx, PDF, instalatoare. Doar cu verificare bit-identică per flux
- [ ] Recompresie JPEG fără pierderi (~20% pe o colecție de poze) — evaluat și amânat, vezi `COMPRESIE.md`
- [ ] Deduplicare pe blocuri, în stil `srep`
- [ ] Backup incremental peste un pachet existent
- [ ] `dhs watch` — pachet ținut la zi
- [ ] Traducerea configurațiilor complexe, registry Windows → fișiere Linux
- [ ] Sincronizare între dispozitive
- [ ] Suport enterprise, implementare în masă
- [ ] Integrare cu instalatoarele de distribuții („ai un pachet DHS? îl restaurez acum")

## Puncte deschise

- Cum comunicăm că parola pierdută înseamnă pachet pierdut, fără să speriem un utilizator obișnuit
- Fișiere mai mari decât un volum: tăierea între volume (proiectată, implementată în `pack`; neexersată încă pe un pachet real cu mai multe volume)
- KeePassXC: baza de parole intră la „secrete", deci opt-in
- Steam: configurația da, biblioteca de jocuri nu — exclusă implicit
