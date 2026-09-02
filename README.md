# DHS — Direct Handoff Suite

Îți mută mediul de lucru dintr-un sistem de operare în altul: **Windows ↔ Linux**, o distribuție
Linux în alta, o versiune de Windows în alta. Faci un **pachet de migrare portabil** pe un SSD sau
un stick, instalezi sistemul nou, și îți iei datele, configurările și aplicațiile înapoi.

Fără cloud. Fără cont. Fără rețea. Fără AI. Un binar de câțiva megaocteți.

> **Stare: în construcție.** Comanda `scan` funcționează. `backup` și `restore` urmează.
> Nu folosi încă pentru o migrare reală.

## Ce face

```bash
dhs scan --dest /run/media/tu/SSD     # ce s-ar salva, cât ocupă, dacă încape
```

```
Sistem        CachyOS Linux (amd64)
Rădăcini      /home/tu/Documente
              /home/tu/Imagini
              …

De inclus     43,7 GiB în 68 463 fișiere   (scanat în ~1 sec)
Excluse       6,2 GiB   --tot le include înapoi
                target                 5,9 GiB · artefacte de build
                node_modules           237 MiB · dependențe, se refac cu npm install
Secrete       1,2 MiB în 9 fișiere   excluse implicit; --secrete le include

Compoziție
                binar            20,0 GiB     46%
                incompresibil    11,8 GiB     27%   se stochează, nu se comprimă
                text             2,4 GiB      6%

Nivel         2 · Echilibrat
Estimare      27,4 GiB – 34,4 GiB   ~3 min pe 8 nuclee
Volume        10 × 3,5 GiB

Destinație    /run/media/tu/SSD   64,0 GiB liberi din 119 GiB   exFAT

              ✓ Încape, cu 29,6 GiB de rezervă.
```

Estimarea vine **înainte** să scriem ceva. Dacă nu încape, îți spune cât lipsește și ce poți face.

## Cum e construit

- **Un singur binar static**, în Go. Pe sistemul destinație, proaspăt instalat, nu există niciun
  runtime — deci nu depindem de niciunul. 2,1 MiB pe Linux, 2,4 MiB pe Windows.
- **Fără cod de rețea.** Baza de date de aplicații e compilată în binar.
- **Pachet pe volume de 3,5 GiB**, sub limita FAT32 de 4 GiB. Dacă nu încap pe un mediu, se împart
  pe mai multe.
- **Criptat cu parolă**, implicit. Secretele — chei SSH, parole de browser — sunt excluse dacă nu le
  ceri explicit, și primesc parolă separată când le ceri.
- **Nimic cu pierderi.** Un fișier restaurat e identic bit cu bit cu originalul, sau nu se scrie
  deloc.

## Construire

```bash
go build -ldflags="-s -w" -o dhs ./cmd/dhs      # Linux
GOOS=windows go build -ldflags="-s -w" ./cmd/dhs  # Windows, din Linux, fără toolchain în plus
go test ./...
```

Codul specific unui sistem de operare stă doar în fișiere cu sufix `_linux.go` și `_windows.go`.
Binarul de Linux **nu conține** niciun octet din codul de Windows, și invers.

## Documentație

| | |
|---|---|
| [`CLAUDE.md`](CLAUDE.md) | ce e produsul, regulile lui, jurnalul de decizii |
| [`docs/ARHITECTURA.md`](docs/ARHITECTURA.md) | formatul pachetului, structura codului, suprafața CLI |
| [`docs/COMPRESIE.md`](docs/COMPRESIE.md) | cele trei niveluri, estimarea, research-ul pe compresia extremă |
| [`docs/LICENTA.md`](docs/LICENTA.md) | de ce Apache-2.0 și nu o licență cu clauză etică |
| [`VALUES.md`](VALUES.md) | ce crede proiectul |
| [`AGENTS.md`](AGENTS.md) | reguli pentru agenții care lucrează pe repo |

## Licență

[Apache License 2.0](LICENSE). Codul e liber. Numele nu — vezi [`TRADEMARK.md`](TRADEMARK.md).
