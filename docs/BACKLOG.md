# BACKLOG — DHS

Ce nu intră în v1. Ordinea în fiecare secțiune e o propunere, nu un angajament.

## v1 — ce mai lipsește din nucleu

- [x] `dhs scan` — inventar, clasificare, excluderi, estimare, verificarea destinației
- [ ] `--precis` — eșantionare pentru raport măsurat + hash pentru deduplicare
- [ ] `internal/pack` — formatul: volume de 3,5 GiB, blocuri solide pe clasă, jurnal de reluare, sume de control
- [ ] `internal/crypto` — `age` cu frază de acces; secțiune separată pentru secrete, cu parolă proprie
- [ ] `dhs backup` — scrierea pachetului
- [ ] `dhs verify` — verificarea integrală, fără extragere
- [ ] `internal/appdb` — baza de aplicații (TOML + `go:embed`) și interogarea ei
- [ ] Detectarea aplicațiilor instalate: `pacman`/`dpkg`/`rpm`/`flatpak`/`snap` pe Linux, registry + `winget` pe Windows
- [ ] `dhs plan` — manifest + appdb → plan de restaurare, fără să atingă nimic
- [ ] `dhs restore` — execuția planului aprobat
- [ ] `dhs raport` — ce a rămas necunoscut sau netradus
- [ ] Cele 10–15 aplicații cu configurații portabile (D3)
- [ ] Împărțirea pachetului pe mai multe medii

## Calitate și infrastructură

- [ ] CI: `go vet`, `go test`, build pentru linux/amd64, linux/arm64, windows/amd64, windows/arm64
- [ ] Buget de dimensiune verificat în CI (Regula #4: binar ≤ 15 MiB)
- [ ] Teste de integrare pe VM-uri: o migrare Windows → Linux dusă până la capăt
- [ ] `CONTRIBUTING.md` cu regula DCO și cum se adaugă o aplicație în bază
- [ ] Completarea numelui în `NOTICE` — blochează prima publicare

## După v1

- [ ] **GUI** — proces separat, vorbește JSON cu CLI-ul (D9). Linux: GTK4 + libadwaita prin `gotk4`. Windows: Win32 nativ sau WinUI
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
- Fișiere mai mari decât un volum: tăierea între volume (proiectat, neimplementat)
- KeePassXC: baza de parole intră la „secrete", deci opt-in
- Steam: configurația da, biblioteca de jocuri nu — exclusă implicit
