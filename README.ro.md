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
dhs restore /run/media/you/SSD/laptop.dhs                       # arată planul, întreabă, apoi scriedhs plan    /run/media/you/SSD/laptop.dhs                       # ce aplicații s-ar instala aici, și cum
dhs install /run/media/you/SSD/laptop.dhs                       # arată comenzile, întreabă o dată, le rulează
```

Restaurarea nu suprascrie niciodată: fișierul existent rămâne, iar cel restaurat apare alături, cu
sufixul ` (DHS)` — dacă nu spui `--conflicts skip` sau `--conflicts overwrite`. Fișierele a căror
rădăcină nu există pe sistemul nou ajung sub `~/DHS-restored/`. Fiecare comandă acceptă `--json`.

**Aplicații.** `scan` și `backup` se uită și la ce e instalat — prin managerele de pachete pe
Linux, prin *Aplicații și caracteristici*, winget, scoop și choco pe Windows — și potrivesc
rezultatul cu [baza de aplicații](appdb/README.md) proprie a DHS — 759 de aplicații la data
scrierii — un fișier JSON per aplicație,
care spune sub ce nume o cunosc managerele fiecărei platforme, unde își ține configurările și ce
îi ține locul acolo unde nu există. Configurările călătoresc cu pachetul, îndosariate după
aplicație, nu după cale, așa că un profil Firefox din `%APPDATA%` ajunge în `~/.mozilla/firefox`
pe Linux. Pe sistemul nou, `dhs plan` spune ce s-ar instala și prin ce manager, ce e deja acolo,
ce n-are versiune pentru platforma asta și ce echivalent l-ar înlocui, și ce nu cunoaște baza de
date — DHS nu ghicește niciodată o aplicație. `dhs install` rulează exact comenzile arătate de
`plan`, după o singură confirmare. Baza e întreținută de comunitate; o aplicație nouă înseamnă un
pull request cu un fișier.

## Instalare

**[Ultima versiune](https://github.com/Necta14/dhs/releases/latest) e un pre-release.** Nucleul
pentru fișiere e testat; manifestul aplicațiilor și detectarea lor nu sunt încă scrise. Nu migra
încă nimic care îți pasă.

Pe **Windows**, în PowerShell:

```powershell
irm https://dhs-suite.vercel.app/install.ps1 | iex
```

Alege singură varianta pentru procesorul tău, o verifică față de sumele SHA-256 publicate, o
dezarhivează în profilul tău și o pune în PATH. Fără drepturi de administrator, fără nimic lăsat în
execuție.

Pe **Linux**:

```bash
sudo apt install ./dhs_<versiune>_amd64.deb     # Debian, Ubuntu, Mint
sudo rpm -i dhs-<versiune>-1.x86_64.rpm         # Fedora, RHEL, openSUSE
makepkg -si                                     # Arch: dhs-cli și dhs-gui, din packaging/aur/
chmod +x DHS-<versiune>-x86_64.AppImage         # orice altceva, fără să instalezi nimic
```

Verifică ce ai descărcat cu sumele publicate lângă el:

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
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | [`docs/ro/ARHITECTURA.md`](docs/ro/ARHITECTURA.md) | formatul pachetului, structura codului, suprafața CLI |
| [`docs/COMPRESSION.md`](docs/COMPRESSION.md) | [`docs/ro/COMPRESIE.md`](docs/ro/COMPRESIE.md) | cele trei niveluri, estimarea, research-ul pe compresia extremă |
| [`docs/LICENSE-CHOICE.md`](docs/LICENSE-CHOICE.md) | [`docs/ro/LICENTA.md`](docs/ro/LICENTA.md) | de ce Apache-2.0 și nu o licență cu clauză etică |
| [`docs/TESTING.md`](docs/TESTING.md) | [`docs/ro/TESTARE.md`](docs/ro/TESTARE.md) | sesiunea dedicată de teste pe GitHub Codespaces |
| [`VALUES.md`](VALUES.md) | [`docs/ro/VALUES.md`](docs/ro/VALUES.md) | ce crede proiectul |

## Licență

[Apache License 2.0](LICENSE). Codul e liber. Numele nu — vezi [`TRADEMARK.md`](TRADEMARK.md).

**Marcă.** Sigla e [`assets/dhs.svg`](assets/dhs.svg), iar logotipul
[`assets/dhs-wordmark.svg`](assets/dhs-wordmark.svg). Sunt acoperite de politica de marcă, nu de
licența codului.
