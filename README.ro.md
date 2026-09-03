<p align="center"><img src="assets/dhs.svg" width="128" alt="DHS"></p>

# DHS — Direct Handoff Suite

🌐 **[dhs-suite.vercel.app/ro](https://dhs-suite.vercel.app/ro/)** · 🇬🇧 [English version](README.md) — versiunea de referință.

Îți mută mediul de lucru dintr-un sistem de operare în altul: **Windows ↔ Linux**, o distribuție
Linux în alta, o versiune de Windows în alta. Faci un **pachet de migrare portabil** pe un SSD sau
un stick, instalezi sistemul nou, și îți iei datele, configurările și aplicațiile înapoi.

Fără cloud. Fără cont. Fără rețea. Fără AI. Un binar de câțiva megaocteți.

> **Stare: în construcție.** `scan`, `backup`, `verify`, `list` și `restore` funcționează și sunt
> verzi pe Codespaces-urile de test. Manifestul de aplicații, detectarea aplicațiilor și `plan` nu
> sunt scrise încă. Nu folosi încă pentru o migrare reală.

## Ce face

```bash
dhs scan --dest /run/media/you/SSD     # ce s-ar salva, cât ocupă, dacă încape
```

```
System        Arch Linux (amd64)
Roots         /home/you/Documents
              /home/you/Pictures
              …

To include    44.0 GiB in 68 000 files   (scanned in ~1 s)
Excluded      6.2 GiB   --all brings them back
                target                 5.9 GiB · build artifacts
                node_modules           237 MiB · dependencies, restored by npm install
Secrets       1.2 MiB in 9 files   excluded by default; --secrets includes them

Composition
                binary           20.0 GiB     46%
                incompressible   11.8 GiB     27%   stored, not compressed
                text             2.4 GiB      6%

Level         2 · Balanced
Estimate      27.4 GiB – 34.4 GiB   ~3 min on 8 cores
Volumes       10 × 3.5 GiB

Destination   /run/media/you/SSD   64.0 GiB free of 119 GiB   exFAT

              ✓ Fits, with 29.6 GiB to spare.
```

Estimarea vine **înainte** să scriem ceva. Dacă nu încape, îți spune cât lipsește și ce poți face.
Cu `DHS_LANG=ro` (sau cu o localizare românească), mesajele apar în română.

```bash
dhs backup --dest /run/media/you/SSD --name laptop --level 2   # scrie migration-<data>.dhs, sau numele dat cu --name
dhs verify /run/media/you/SSD/laptop.dhs                        # fiecare sumă de control, fără să extragă nimic
dhs list   /run/media/you/SSD/laptop.dhs --all                  # ce e înăuntru
dhs restore /run/media/you/SSD/laptop.dhs --dry-run             # planul, fără să scrie nimic
dhs restore /run/media/you/SSD/laptop.dhs                       # arată planul, întreabă, apoi scrie
```

Restaurarea nu suprascrie niciodată: fișierul existent rămâne, iar cel restaurat apare alături, cu
sufixul ` (DHS)` — dacă nu spui `--conflicts skip` sau `--conflicts overwrite`. Fișierele a căror
rădăcină nu există pe sistemul nou ajung sub `~/DHS-restored/`. Fiecare comandă acceptă `--json`.

## Instalare

**[0.1.0 e publicată ca pre-release.](https://github.com/Necta14/dhs/releases/tag/v0.1.0)**
Nucleul pentru fișiere e testat; manifestul aplicațiilor și detectarea lor nu sunt încă scrise, iar
binarele de Windows sunt compilate încrucișat, dar nerulate pe Windows. Nu migra încă nimic care
îți pasă.

```bash
# Debian, Ubuntu, Mint
sudo apt install ./dhs_0.1.0_amd64.deb

# Fedora, RHEL, openSUSE
sudo rpm -i dhs-0.1.0-1.x86_64.rpm

# Arch: două pachete din aceeași rețetă, în packaging/aur/
makepkg -si          # dhs-cli, plus dhs-gui pentru interfața GTK4

# orice Linux, fără să instalezi nimic
chmod +x DHS-0.1.0-x86_64.AppImage && ./DHS-0.1.0-x86_64.AppImage scan --dest /run/media/tu/SSD
```

Toate fișierele sunt în [versiunea publicată](https://github.com/Necta14/dhs/releases/latest), cu `SHA256SUMS` lângă ele:

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

## Cum e construit

- **Un singur binar static**, în Go. Pe sistemul destinație, proaspăt instalat, nu există niciun
  runtime — deci nu depindem de niciunul. 4,2 MiB pe Linux, 4,5 MiB pe Windows.
- **Fără cod de rețea.** Baza de date de aplicații e compilată în binar.
- **Pachet pe volume de 3,5 GiB**, sub limita FAT32 de 4 GiB. Dacă nu încap pe un mediu, se împart
  pe mai multe.
- **Criptat cu parolă**, implicit. Secretele — chei SSH, parole de browser — sunt excluse dacă nu le
  ceri explicit, și primesc parolă separată când le ceri.
- **Nimic cu pierderi.** Un fișier restaurat e identic bit cu bit cu originalul, sau nu se scrie
  deloc.

## Construire

```bash
go build -ldflags="-s -w" -o dhs ./cmd/dhs        # Linux
GOOS=windows go build -ldflags="-s -w" ./cmd/dhs  # Windows, din Linux, fără toolchain în plus
gofmt -l . && go vet ./...                        # verificări statice; testele rulează pe Codespaces, vezi docs/TESTING.md
```

Codul specific unui sistem de operare stă doar în fișiere cu sufix `_linux.go` și `_windows.go`.
Binarul de Linux **nu conține** niciun octet din codul de Windows, și invers.

## Prototip de GUI pentru Linux

`gui/linux/dhs-gui.py` e o interfață GTK4 + libadwaita (PyGObject) care conduce CLI-ul prin
`--json` — nucleul stă într-un singur loc, GUI-ul e doar un înveliș peste el.

```bash
go build -o dhs ./cmd/dhs        # GUI-ul are nevoie de binar la rădăcina repo-ului, sau de DHS_BIN setat
python3 gui/linux/dhs-gui.py
```

## Documentație

Versiunea de referință a fiecărui document e cea în engleză; traducerile în română stau în
[`docs/ro/`](docs/ro/).

| engleză (referință) | română | |
|---|---|---|
| [`CLAUDE.md`](CLAUDE.md) | [`docs/ro/CLAUDE.md`](docs/ro/CLAUDE.md) | ce e produsul, regulile lui, jurnalul de decizii |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | [`docs/ro/ARHITECTURA.md`](docs/ro/ARHITECTURA.md) | formatul pachetului, structura codului, suprafața CLI |
| [`docs/COMPRESSION.md`](docs/COMPRESSION.md) | [`docs/ro/COMPRESIE.md`](docs/ro/COMPRESIE.md) | cele trei niveluri, estimarea, research-ul pe compresia extremă |
| [`docs/LICENSE-CHOICE.md`](docs/LICENSE-CHOICE.md) | [`docs/ro/LICENTA.md`](docs/ro/LICENTA.md) | de ce Apache-2.0 și nu o licență cu clauză etică |
| [`docs/TESTING.md`](docs/TESTING.md) | [`docs/ro/TESTARE.md`](docs/ro/TESTARE.md) | sesiunea dedicată de teste pe GitHub Codespaces |
| [`VALUES.md`](VALUES.md) | [`docs/ro/VALUES.md`](docs/ro/VALUES.md) | ce crede proiectul |
| [`AGENTS.md`](AGENTS.md) | [`docs/ro/AGENTS.md`](docs/ro/AGENTS.md) | reguli pentru agenții care lucrează pe repo |

## Licență

[Apache License 2.0](LICENSE). Codul e liber. Numele nu — vezi [`TRADEMARK.md`](TRADEMARK.md).

**Marcă.** Sigla e [`assets/dhs.svg`](assets/dhs.svg), iar logotipul
[`assets/dhs-wordmark.svg`](assets/dhs-wordmark.svg). Sunt acoperite de politica de marcă, nu de
licența codului.
