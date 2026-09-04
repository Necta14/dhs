# Arhitectura DHS — propunere pentru v1

> Traducere în română. Versiunea de referință e cea în engleză: [`../ARCHITECTURE.md`](../ARCHITECTURE.md)

> Stare la 04.09.2026: `scan`, `backup`, `verify`, `list`, `restore` sunt implementate și **verzi
> pe Codespaces** ([`TESTARE.md`](TESTARE.md)). Partea de aplicații — baza de date (`appdb/`),
> detectarea, `plan` și `install` — e scrisă și acoperită de teste unitare și de scriptul e2e.
> **Secțiunile despre aplicații de mai jos sunt depășite**: forma actuală (JSON în loc de TOML,
> rădăcinile `apps/<id>/<cheie>`, comanda `install`) e descrisă în versiunea engleză, care rămâne
> referința. Punctele ⚠️ mai au nevoie de confirmare.
>
> Context: [`../../README.ro.md`](../../README.ro.md); deciziile sunt rezumate acolo unde se aplică.

## Principiul de bază

Un singur binar Go, static, care rulează identic pe Windows și Linux, **fără rețea și fără AI**.
Toată logica stă în bibliotecă; CLI-ul e un înveliș subțire, iar GUI-ul de mai târziu va fi un al
doilea înveliș peste exact aceeași bibliotecă.

## Formatul pachetului de migrare

Pachetul e un **director**, nu un fișier unic. Motivul e practic: mediile externe sunt adesea
formatate FAT32 sau exFAT, iar FAT32 nu acceptă fișiere peste 4 GB și nu păstrează permisiuni,
proprietar sau legături simbolice.

```
migration-2026-09-02-1014.dhs/
  dhs.json              # manifest rădăcină, NECRIPTAT — vezi mai jos ce conține și ce nu
  index.dhsi            # indexul complet (fișiere, blocuri, plasare), CRIPTAT; mic, se citește primul
  volumes/
    0001.dhsv           # blocuri + index propriu + trailer, CRIPTAT, ≤ 3,5 GiB
    0002.dhsv
    ...
  SHA256SUMS            # SHA-256 pentru fiecare fișier din pachet
  journal.jsonl         # o linie per volum încheiat, pentru reluare după întrerupere
```

Implementat în `internal/pack`: `format.go` (antete binare), `index.go` (structurile JSON),
`writer.go` + `emitter.go` (scriere paralelă, ordonată, cu rotirea volumelor), `reader.go`
(deschidere, verificare, extragere secvențială), `journal.go` (jurnal, sume, scriere atomică).

Indexul stă **într-un fișier separat, criptat** (`index.dhsi`), nu la începutul primului volum:
`age` e un flux, nu permite citirea de la coadă, iar indexul nu e cunoscut până la final. Fiecare
volum poartă totuși și un index propriu, la coadă — dacă `index.dhsi` se pierde, se poate reconstrui.

### Ce e necriptat și de ce

`dhs.json` trebuie citit **înainte** ca userul să introducă parola, ca DHS să poată spune ce e
pachetul ăsta. Deci conține strict:

```json
{
  "format": 1,
  "id": "01J...",
  "created": "2026-09-02T10:14:00Z",
  "source": { "os": "windows", "version": "11 Pro 23H2", "arch": "x86_64" },
  "cipher": "age-scrypt",
  "volumes": 7,
  "bytes": 14332891136,
  "apps": 63,
  "secrets_included": false
}
```

**Nu conține** nume de fișiere, căi, nume de aplicații sau numele utilizatorului. Lista de fișiere și
manifestul aplicațiilor stau **în interiorul volumelor criptate** — altfel pachetul ar spune oricui
îl găsește exact ce software și ce documente ai.

### Volume

Fiecare volum e un flux scris direct pe disc, fără să ținem nimic mare în memorie: mergem la fel de
repede pe un laptop cu 8 GB ca pe unul cu 64.

**3,5 GiB implicit**, fiindcă FAT32 refuză fișierele de 4 GiB sau mai mari. Uniform pe orice sistem
de fișiere — pachetul rămâne mutabil pe orice mediu, oricând.

Volumul e și unitatea de reluare: dacă se smulge cablul, se reface cel mult volumul curent, nu tot
backupul. Un fișier mai mare decât un volum (un ISO de 8 GiB) se taie între volume, iar manifestul
reține că bucățile fac parte din același fișier.

Dacă pachetul nu încape pe un singur mediu, volumele se **împart pe mai multe** — `dhs.json` știe
câte sunt în total și care lipsesc la restaurare.

### Ce e într-un volum

Nu un singur flux comprimat, ci **blocuri solide grupate pe clasă de fișier**:

- **incompresibil** (poze, video, muzică, arhive) → stocat, fără compresie
- **comprimabil** (text, cod, documente, configurații) → bloc solid comprimat, ca redundanța dintre
  fișiere mici să se exploateze
- **necunoscut** → test de entropie pe primii 256 KB, apoi într-una din clasele de sus

Plus **deduplicare la nivel de fișier**: sumele SHA-256 se calculează oricum pentru integritate,
deci un fișier care apare de trei ori se stochează o dată.

Detalii, măsurători și cele trei niveluri de compresie alese de user: [`COMPRESIE.md`](COMPRESIE.md).

### Criptare

`age` cu frază de acces (`filippo.io/age`, recipient scrypt). Motivul pentru care nu ne scriem
propriul strat: `age` e proiectat și auditat exact pentru asta, iar criptografia scrisă de mână e
locul clasic unde un proiect ca ăsta face o gaură pe care nimeni n-o observă doi ani.

**Parola acoperă tot pachetul** — toate fișierele, nu o selecție. Criptarea e alegerea userului:
pornită implicit, se poate opri (`--no-encrypt`), caz în care nivelul 1 dă un ZIP deschizabil pe
orice calculator, fără DHS.

**Secretele au parolă proprie.** Când userul le include explicit (`--secrets`), ele stau într-o
secțiune separată, cu a doua frază de acces. Așa poți da pachetul cuiva ca să-ți recupereze
documentele, fără să-i dai și cheile SSH.

⚠️ **Parola pierdută = pachet pierdut, definitiv.** Trebuie decis cum comunicăm asta în interfață
(propunere: confirmare dublă la creare, plus un avertisment care nu se poate sări).

### Integritate și reluare

- `SHA256SUMS` — SHA-256 pentru fiecare fișier. `dhs verify` îl verifică integral.
- `journal.jsonl` — o linie per volum încheiat. La reluare, DHS citește jurnalul și continuă de unde
  a rămas.
- Un volum se scrie sub nume temporar și se redenumește doar după ce e complet. Deci un pachet
  întrerupt **nu conține niciodată** un volum pe jumătate scris care pare valid.

## Manifestul aplicațiilor

Stă criptat, în primul volum.

```json
{
  "system": { "os": "windows", "version": "11 Pro 23H2", "user": "you" },
  "apps": [
    {
      "id": "mozilla.firefox",
      "name": "Mozilla Firefox",
      "version": "142.0",
      "detected_via": "winget",
      "config": { "state": "captured", "portability": "identical" }
    },
    {
      "id": null,
      "name": "Internal Program Ltd v3",
      "detected_via": "registry",
      "unknown": true
    }
  ]
}
```

Aplicațiile necunoscute rămân în manifest cu `id: null`. DHS **nu inventează** un echivalent — le
trece în raportul final ca „nu știu ce e asta, ocupă-te tu".

## Baza de date de aplicații

Fișiere TOML, unul per aplicație, incluse în binar cu `go:embed`. Contribuibile prin pull request,
fără să fie nevoie de vreun serviciu.

```toml
id = "mozilla.firefox"
name = "Mozilla Firefox"
category = "browser"

[windows]
detect   = { winget = "Mozilla.Firefox", registry = ["HKLM\\SOFTWARE\\Mozilla\\Mozilla Firefox"] }
install  = [{ manager = "winget", id = "Mozilla.Firefox" }, { manager = "choco", id = "firefox" }]
config   = ["%APPDATA%/Mozilla/Firefox/Profiles"]

[linux]
detect   = { pacman = "firefox", dpkg = "firefox", flatpak = "org.mozilla.firefox" }
install  = [{ manager = "pacman", id = "firefox" }, { manager = "apt", id = "firefox" },
            { manager = "flatpak", id = "org.mozilla.firefox" }]
config   = ["~/.mozilla/firefox"]

[portability]
config = "identical"      # identical | translatable | untranslatable
notes  = "The profile is portable across platforms; the profile directory is copied."

# Pentru aplicații care nu există pe cealaltă platformă:
# equivalents = ["kde.kate", "vscodium"]
```

Cele 10–15 aplicații din v1 (D3), propunere: Firefox, Chrome/Chromium, VSCode/VSCodium, Git, SSH
(`~/.ssh/config`, fără chei), Thunderbird, VLC, Obsidian, KeePassXC ⚠️, GIMP, LibreOffice, Discord,
Steam ⚠️ (doar configurația, nu biblioteca de jocuri).

⚠️ De discutat: KeePassXC ține baza de parole — intră la „secrete", deci opt-in. Steam poate avea
sute de GB; trebuie exclus implicit, cu opt-in separat.

## Structura codului

Codul, comentariile, documentația și CLI-ul sunt în engleză; româna se menține ca traducere
(`README.ro.md`, `docs/ro/` și catalogul CLI `internal/i18n/ro.go`, ales prin `DHS_LANG` sau din
localizare). ✅ = există deja.

```
cmd/dhs/                  CLI — doar parsare de argumente și afișare                    ✅
internal/
  system/                 detectare OS, căi standard, spațiu liber, privilegii          ✅
    system_linux.go       /etc/os-release, XDG user-dirs, statfs                        ✅
    system_windows.go     registry, Known Folders, GetDiskFreeSpaceEx                   ✅
  scan/                   inventar, clasificare, excluderi, estimare                    ✅
  report/                 formatarea cifrelor pentru ochi de om                         ✅
  pack/                   formatul: scriere, citire, volume, jurnal, sume               ✅
  passphrase/             age cu frază de acces                                         ✅
  restore/                plan de restaurare + execuție prin fișier temporar            ✅
  i18n/                   mesajele CLI: engleză implicit + catalog românesc (DHS_LANG)  ✅
  appdb/                  baza de aplicații (go:embed) + interogare
  apps/                   detectarea aplicațiilor instalate, plan de instalare
```

Codul specific unui sistem de operare stă **doar** în fișiere cu sufix `_linux.go` și `_windows.go`.
Datorită etichetelor de build, binarul de Linux nu conține niciun octet din codul de Windows — de
aici și dimensiunile: 4,2 MiB pe Linux, 4,5 MiB pe Windows.

Regula: fișierele `_linux.go` / `_windows.go` din `internal/system` sunt singurele locuri cu cod
specific unui sistem de operare. Restul e comun și testabil pe orice mașină.

## Suprafața CLI

```bash
dhs scan                                    # ce s-ar include, cât ocupă, ce aplicații s-au găsit
           --dest /run/media/you/SSD        # verifică dacă încape, cu tot cu compresie estimată
           --level 2                        # 1 compatibil | 2 echilibrat | 3 maxim
           --secrets | --all                # include secretele | nu exclude nimic
           --precise                        # eșantionare + dedup: estimare măsurată, nu presupusă (neimplementat)
dhs backup --dest /run/media/you/SSD        # creează pachetul (întreabă parola)
           --name test                      # implicit: migration-<data>
           --level 2 --secrets --all
           --no-encrypt                     # pachet necriptat, cu avertisment vizibil (D7)
           --passphrase-file <cale> --yes --verify
dhs verify <pachet>                         # verifică fiecare sumă de control, fără să extragă nimic
dhs list   <pachet> [--all]                 # conținutul: rezumat sau listă completă
dhs restore <pachet>                        # implicit: arată planul și cere aprobare
            --dry-run                       # doar planul, nu scrie nimic
            --only documents,pictures       # doar aceste rădăcini
            --conflicts keep-both|skip|overwrite
dhs plan   <pachet>                         # (planificat) ce s-ar instala și restaura; nu atinge nimic
dhs report <pachet>                         # (planificat) ce a rămas necunoscut sau netradus
dhs version
```

Fiecare comandă acceptă `--json` — GUI-ul și automatizările vorbesc cu CLI-ul prin el. Rădăcinile
sunt `documents`, `pictures`, `videos`, `music`, `downloads`, `desktop`, `config` și `other`.
Fișierele a căror rădăcină nu există pe sistemul destinație, și tot ce e sub `other`, se restaurează
sub `~/DHS-restored/`. Un conflict păstrează fișierul existent și îl scrie pe cel restaurat alături,
cu sufixul „ (DHS)", dacă `--conflicts` nu spune altfel. Mediu: `DHS_LANG` (`en`/`ro`, implicit din
localizare) și `DHS_DEBUG` (ieșire de diagnostic pentru rapoarte de bug).

`dhs restore` fără argumente în plus **nu execută nimic** până nu vede planul aprobat. `plan` e
comanda pe care o poate rula oricine, oricând, fără risc.

## Privilegii

- **Backup** rulează ca utilizator obișnuit. Profilul propriu nu cere administrator.
- **Restaurarea fișierelor** rulează tot ca utilizator.
- **Instalarea aplicațiilor** e singurul pas care cere `sudo` / administrator, și e izolat exact
  acolo, după aprobarea planului.

## Ce NU face v1

Ca să rămână scris negru pe alb: fără sincronizare între dispozitive, fără cloud, fără traducere de
configurații complexe, fără registry Windows → dconf, fără GUI, fără deduplicare pe blocuri în stil
`srep`, fără preprocesare `precomp`, fără backup incremental. Toate în `BACKLOG.md`.

Deduplicarea **la nivel de fișier** intră totuși în v1: e practic gratuită, fiindcă hash-urile se
calculează oricum.

## Puncte deschise

1. ⚠️ Cum comunicăm riscul parolei pierdute, fără să speriem userul obișnuit.
2. ⚠️ Excluderi implicite pentru fișiere: `node_modules`, cache-uri, biblioteci Steam, mașini
   virtuale. Propunere: listă implicită vizibilă și editabilă înainte de backup.
3. D7 — decis: nivelul 1 împreună cu `--no-encrypt`, cu avertisment vizibil.
   Vezi [`COMPRESIE.md` §4.3](COMPRESIE.md).
4. D6 — decis: Apache-2.0, vezi [`LICENTA.md`](LICENTA.md).
