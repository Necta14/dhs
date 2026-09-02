# Arhitectura DHS — propunere pentru v1

> Stare: **propunere**, scrisă după închiderea deciziilor D1–D5 (02.09.2026). Nu s-a scris cod de
> produs. Punctele marcate ⚠️ mai au nevoie de confirmare.
>
> Context și decizii: [`../CLAUDE.md`](../CLAUDE.md).

## Principiul de bază

Un singur binar Go, static, care rulează identic pe Windows și Linux, **fără rețea și fără AI**.
Toată logica stă în bibliotecă; CLI-ul e un înveliș subțire, iar GUI-ul de mai târziu va fi un al
doilea înveliș peste exact aceeași bibliotecă.

## Formatul pachetului de migrare

Pachetul e un **director**, nu un fișier unic. Motivul e practic: mediile externe sunt adesea
formatate FAT32 sau exFAT, iar FAT32 nu acceptă fișiere peste 4 GB și nu păstrează permisiuni,
proprietar sau legături simbolice.

```
migrare-2026-09-02-necta.dhs/
  dhs.json              # manifest rădăcină, NECRIPTAT — vezi mai jos ce conține și ce nu
  volume/
    0001.dhsv           # volum de date: blocuri solide pe clasă → criptare. 3,5 GiB fiecare
    0002.dhsv
    ...
  SUME.txt              # SHA-256 pentru fiecare fișier din pachet
  jurnal.jsonl          # jurnal append-only de progres, pentru reluare după întrerupere
```

### Ce e necriptat și de ce

`dhs.json` trebuie citit **înainte** ca userul să introducă parola, ca DHS să poată spune ce e
pachetul ăsta. Deci conține strict:

```json
{
  "format": 1,
  "id": "01J...",
  "creat": "2026-09-02T10:14:00Z",
  "sursa": { "os": "windows", "versiune": "11 Pro 23H2", "arh": "x86_64" },
  "cifru": "age-scrypt",
  "volume": 7,
  "octeti": 14332891136,
  "aplicatii": 63,
  "secrete_incluse": false
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

⚠️ **Parola pierdută = pachet pierdut, definitiv.** Trebuie decis cum comunicăm asta în interfață
(propunere: confirmare dublă la creare, plus un avertisment care nu se poate sări).

### Integritate și reluare

- `SUME.txt` — SHA-256 pentru fiecare fișier. `dhs verify` îl verifică integral.
- `jurnal.jsonl` — o linie per volum încheiat. La reluare, DHS citește jurnalul și continuă de unde
  a rămas.
- Un volum se scrie sub nume temporar și se redenumește doar după ce e complet. Deci un pachet
  întrerupt **nu conține niciodată** un volum pe jumătate scris care pare valid.

## Manifestul aplicațiilor

Stă criptat, în primul volum.

```json
{
  "sistem": { "os": "windows", "versiune": "11 Pro 23H2", "user": "necta" },
  "aplicatii": [
    {
      "id": "mozilla.firefox",
      "nume": "Mozilla Firefox",
      "versiune": "142.0",
      "detectat_prin": "winget",
      "config": { "stare": "capturata", "portabilitate": "identica" }
    },
    {
      "id": null,
      "nume": "Program intern SRL v3",
      "detectat_prin": "registry",
      "necunoscuta": true
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
nume = "Mozilla Firefox"
categorie = "browser"

[windows]
detectare  = { winget = "Mozilla.Firefox", registry = ["HKLM\\SOFTWARE\\Mozilla\\Mozilla Firefox"] }
instalare  = [{ manager = "winget", id = "Mozilla.Firefox" }, { manager = "choco", id = "firefox" }]
config     = ["%APPDATA%/Mozilla/Firefox/Profiles"]

[linux]
detectare  = { pacman = "firefox", dpkg = "firefox", flatpak = "org.mozilla.firefox" }
instalare  = [{ manager = "pacman", id = "firefox" }, { manager = "apt", id = "firefox" },
              { manager = "flatpak", id = "org.mozilla.firefox" }]
config     = ["~/.mozilla/firefox"]

[portabilitate]
config = "identica"      # identica | traductibila | netraductibila
note   = "Profilul e portabil între platforme; se copiază directorul de profil."

# Pentru aplicații care nu există pe cealaltă platformă:
# echivalente = ["kde.kate", "vscodium"]
```

Cele 10–15 aplicații din v1 (D3), propunere: Firefox, Chrome/Chromium, VSCode/VSCodium, Git, SSH
(`~/.ssh/config`, fără chei), Thunderbird, VLC, Obsidian, KeePassXC ⚠️, GIMP, LibreOffice, Discord,
Steam ⚠️ (doar configurația, nu biblioteca de jocuri).

⚠️ De discutat: KeePassXC ține baza de parole — intră la „secrete", deci opt-in. Steam poate avea
sute de GB; trebuie exclus implicit, cu opt-in separat.

## Structura codului

```
cmd/dhs/                  CLI — doar parsare de argumente și afișare
internal/
  sistem/                 detectare OS, căi standard, privilegii
    windows/              registry, Known Folders, winget/choco/scoop
    linux/                XDG, pacman/dpkg/rpm/flatpak/snap
  scanare/                inventar de fișiere și aplicații, estimare de dimensiune
  pachet/                 formatul: scriere, citire, volume, jurnal, sume de control
  cripto/                 age cu frază de acces
  appdb/                  baza de aplicații (go:embed) + interogare
  plan/                   manifest + appdb → plan de restaurare
  restaurare/             execuția planului aprobat
  raport/                 ce a mers, ce nu, ce a rămas necunoscut
```

Regula: `internal/sistem/windows` și `internal/sistem/linux` sunt singurele locuri cu cod specific
unui sistem de operare. Restul e comun și testabil pe orice mașină.

## Suprafața CLI

```bash
dhs scan                                    # ce s-ar include, cât ocupă, ce aplicații s-au găsit
           --dest /run/media/x/SSD          # verifică dacă încape, cu tot cu compresie estimată
           --nivel 2                        # 1 compatibil | 2 echilibrat | 3 maxim
           --precis                         # eșantionare + dedup: estimare măsurată, nu presupusă
dhs backup --dest /run/media/x/SSD          # creează pachetul (întreabă parola)
           --include documente,poze,config
           --nivel 2
           --secrete                        # opt-in explicit pentru chei și parole
dhs verify <pachet>                         # verifică sumele de control, fără să extragă nimic
dhs plan   <pachet>                         # ce s-ar instala și restaura. Nu atinge nimic
dhs restore <pachet>                        # implicit: arată planul și cere aprobare
            --doar-fisiere | --doar-aplicatii
dhs raport <pachet>                         # ce a rămas necunoscut sau netradus
```

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
3. **D7** — modul ZIP necriptat. Nivelul 1 de compresie are sens doar dacă pachetul se poate
   deschide fără DHS, ceea ce intră în conflict cu criptarea implicită din D4.
   Vezi [`COMPRESIE.md` §4.3](COMPRESIE.md).
4. D6 — licența.
