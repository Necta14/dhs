# Testarea — sesiunea dedicată pe GitHub Codespaces

> Regulă (AGENTS.md, 7): testele **nu** se rulează pe laptopul userului. Nici `go test`, nici
> binarul `dhs` pe datele lui. Se rulează pe două Codespaces, într-o sesiune separată. Documentul
> ăsta e ce trebuie făcut acolo, în ordine, ca sesiunea să fie scurtă și completă.

## De ce două Codespaces

1. **Sursa** — joacă rolul sistemului de pe care pleci. Aici se face `dhs backup`.
2. **Destinația** — joacă rolul sistemului proaspăt instalat. Aici se face `dhs restore`.

Pachetul trece de la una la alta ca fișiere (un `tar` al directorului `.dhs`, sau un mount
comun). Așa testăm exact fluxul real: două mașini, niciun fișier în comun în afară de pachet.

Codespaces sunt Linux. Pentru Windows real e nevoie de o VM sau de un runner Windows în CI —
până atunci, pe Windows verificăm doar că **compilează** (`GOOS=windows go build`).

## Pasul 0 — pregătire, pe amândouă

```bash
git clone https://github.com/Necta14/dhs.git && cd dhs
go version                       # ≥ 1.26
gofmt -l . && go vet ./...       # trebuie tăcute
go build -ldflags="-s -w" -o dhs ./cmd/dhs
ls -l dhs                        # sub 15 MiB, Regula #4
```

## Pasul 1 — testele unitare (pe oricare)

```bash
go test ./... -count=1 -v 2>&1 | tee /tmp/go-test.log
go test ./... -race -count=1     # detectorul de curse — conducta de compresie e concurentă
```

Ce trebuie să iasă verde și ce dovedește fiecare:

| Test | Dovedește |
|---|---|
| `TestRoundTripEncrypted` (×3 niveluri) | scrie → verifică → extrage, identic bit cu bit, pe toate nivelurile |
| `TestRoundTripUnencrypted` | modul fără criptare merge la fel |
| `TestManifestLeaksNothing` | `dhs.json`, `SUME.txt`, `jurnal.jsonl` nu conțin nume de fișiere sau utilizator; `index.dhsi` e criptat |
| `TestPassphraseErrors` | fără frază → `ErrNeedPassphrase`; frază greșită → `ErrBadPassphrase`; frază scurtă refuzată la scriere |
| `TestDedup` | fișierul identic e marcat `Dup`, trimite la aceleași blocuri, se extrage întreg |
| `TestLargeFileSpansVolumes` | un fișier mai mare decât volumul se taie și se reface corect |
| `TestFilterSkipsUnneededVolumes` | extragerea selectivă scrie doar ce s-a cerut |
| `TestCorruptionIsDetected` | un octet stricat e prins de `Verify`; la extragere pică **doar** fișierele din blocul stricat, iar ele nu ajung la `Commit` |
| `TestAbortLeavesIncompletePackage` | după întrerupere: manifest incomplet, volum în curs rămas `.tmp`, nimic care să pară valid |
| `TestRefusesNonEmptyDir` | nu suprascriem un director cu conținut |
| `TestSizeMismatchIsRecorded` | un fișier crescut între scan și backup se reține la dimensiunea reală |
| `restore.TestPlanAndExecute` | planul rezolvă destinații, conflictul păstrează existentul și scrie alături, rădăcinile inexistente merg sub `DHS-restaurat`, mod și mtime se refac, niciun `.dhs-tmp` nu rămâne |
| `restore.TestPlanConflictPolicies` | `sari` nu atinge nimic; `suprascrie` înlocuiește |
| `restore.TestPlanCaseCollisionOnWindows` | `Raport.txt` și `raport.txt` ajung în două fișiere pe Windows |
| `restore.TestSanitizeWindows` | caractere interzise, nume rezervate, puncte finale — adaptate |
| `internal/scan`, `internal/system` | clasificare, excluderi, secrete, inventar, estimare, detectare OS |

Dacă ceva pică, **nu se trece la pasul 2**. Se notează în `docs/PROBLEMS.md` și se repară întâi.

## Pasul 2 — flux real, sursă → destinație

### Pe sursă

```bash
# un profil de test, cu ceva din fiecare clasă
mkdir -p ~/Documents ~/Pictures ~/Downloads
head -c 5M /dev/urandom > ~/Pictures/poza.jpg          # incompresibil
cp ~/Pictures/poza.jpg ~/Downloads/copie.jpg           # duplicat
seq 1 2000000 > ~/Documents/numere.txt                 # text, comprimabil
head -c 300M /dev/urandom > ~/Downloads/mare.bin       # mai mare decât un volum? nu la 3,5 GiB —
                                                       # pentru asta, testul unitar e cel care contează

./dhs scan --dest /tmp                                 # estimarea, fără să scrie
echo 'fraza-de-test-pentru-codespaces' > /tmp/parola
./dhs backup --dest /tmp --nume test --parola-fisier /tmp/parola --da --verifica
./dhs verify /tmp/test.dhs --parola-fisier /tmp/parola
cat /tmp/test.dhs/dhs.json                             # fără nume de fișiere, fără user
cat /tmp/test.dhs/SUME.txt
tar -cf /tmp/test.dhs.tar -C /tmp test.dhs
```

Verificări cu ochiul: manifestul spune `"complet": true`; `volume/` conține doar `.dhsv`, niciun
`.tmp`; `SUME.txt` listează fiecare fișier din pachet.

### Întrerupere (tot pe sursă)

```bash
head -c 2G /dev/urandom > ~/Downloads/enorm.bin
./dhs backup --dest /tmp --nume intrerupt --parola-fisier /tmp/parola --da &
sleep 3; kill -INT %1; wait
cat /tmp/intrerupt.dhs/dhs.json | grep complet         # false
ls /tmp/intrerupt.dhs/volume/                          # volumele încheiate .dhsv, cel în curs .tmp
./dhs verify /tmp/intrerupt.dhs --parola-fisier /tmp/parola   # raportează „incomplet", nu cade
```

### Pe destinație

```bash
tar -xf /tmp/test.dhs.tar -C /tmp
./dhs verify /tmp/test.dhs --parola-fisier /tmp/parola
./dhs list /tmp/test.dhs --parola-fisier /tmp/parola --tot
./dhs restore /tmp/test.dhs --parola-fisier /tmp/parola --dry-run
./dhs restore /tmp/test.dhs --parola-fisier /tmp/parola
sha256sum ~/Pictures/poza.jpg ~/Downloads/copie.jpg ~/Documents/numere.txt

# conflict: rulează din nou — existentul rămâne, restauratul apare cu „ (DHS)”
./dhs restore /tmp/test.dhs --parola-fisier /tmp/parola --da
ls ~/Documents/                                        # numere.txt și „numere (DHS).txt”
./dhs restore /tmp/test.dhs --parola-fisier /tmp/parola --conflicte sari --dry-run
```

Sumele trebuie să fie identice cu cele de pe sursă. **Asta e testul care contează.**

## Pasul 3 — ce se notează la final

În `docs/NOTES.md`: versiunea testată (commit), ce a trecut, ce a picat, timpi (cât a durat
backup-ul pe câți GiB, la ce nivel), dimensiunea binarului. Orice problemă în `docs/PROBLEMS.md`.
