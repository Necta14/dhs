# Testarea — sesiunea dedicată pe GitHub Codespaces

> Traducere în română. Versiunea de referință e cea în engleză: [`../TESTING.md`](../TESTING.md)

> Regulă: testele **nu** se rulează pe mașina menținătorului. Nici `go test`, nici
> binarul `dhs` pe datele lui. Se rulează pe două Codespaces, într-o sesiune separată. Documentul
> ăsta e ce trebuie făcut acolo, în ordine, ca sesiunea să fie scurtă și completă.
>
> **Prima rundă verde: 03.09.2026, commit `3a39aef`**, pe Codespace-ul cu 4 nuclee — tot ce e în
> `scripts/codespace-tests.sh`, inclusiv `-race` și fluxul e2e. Rundele anterioare au găsit și
> reparat: un nil pointer la creare, patru curse de date la serializarea indexului, un contor
> dublat și scrypt de producție care făcea suita să dureze 10 minute.

## Migrare între distribuții — `scripts/e2e-distro.sh`

Backup pe gazda Ubuntu, restaurare în containere Docker de **Arch, Fedora, Debian, openSUSE și
Alpine (musl)**, comparare sha256 pe căi relative. Fiecare container are doar binarul static și
pachetul de pe „SSD" — un sistem proaspăt instalat. **Verde 03.09.2026, commit `6a55e1d`**: cinci
din cinci, identic bit cu bit. Pentru o singură distribuție: `bash scripts/e2e-distro.sh alpine:latest`.

Ce nu acoperă: Windows (Codespaces n-are VM-uri Windows) și XDG localizat (`user-dirs.dirs`) —
containerele folosesc numele implicite.

## Cel mai scurt drum: de pe mașina ta, prin `gh`

```bash
gh auth token -u <user> | GH_TOKEN=$(gh auth token -u <user>) gh codespace ssh -c <codespace> -- \
  'cd /workspaces/dhs && git pull -q && bash scripts/codespace-tests.sh'
```

Primul `gh auth token` e trimis prin pipe pentru partea din Codespace: o sesiune SSH nu încarcă
mediul Codespace-ului, deci git-ul de acolo n-are credențiale, iar scriptul citește un token de pe
stdin ca să publice rezultatele. Fără el rularea se termină oricum; rezultatele rămân doar în
`test-results/` pe Codespace, iar rezumatul e afișat la final.

`gh` are nevoie de scope-ul `codespace` (`gh auth refresh -s codespace`). Dacă `gh` ține mai multe
conturi, alege-l pe cel care deține Codespaces-urile per comandă, prin
`GH_TOKEN=$(gh auth token -u <user>)`, fără să schimbi contul activ. Testele rulează în Codespace;
mașina ta doar trimite comanda.

Scriptul publică rezultatele pe ramura `tests/<nume-codespace>`, în dosarul `test-results/`
(`SUMMARY.md` plus un jurnal per pas), ca să poată fi citite de oriunde cu un simplu `git fetch` —
fără `gh`, fără SSH.

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
| `TestManifestLeaksNothing` | `dhs.json`, `SHA256SUMS`, `journal.jsonl` nu conțin nume de fișiere sau utilizator; `index.dhsi` e criptat |
| `TestPassphraseErrors` | fără frază → `ErrNeedPassphrase`; frază greșită → `ErrBadPassphrase`; frază scurtă refuzată la scriere |
| `TestDedup` | fișierul identic e marcat `Dup`, trimite la aceleași blocuri, se extrage întreg |
| `TestLargeFileSpansVolumes` | un fișier mai mare decât volumul se taie și se reface corect |
| `TestFilterSkipsUnneededVolumes` | extragerea selectivă scrie doar ce s-a cerut |
| `TestCorruptionIsDetected` | un octet stricat e prins de `Verify`; la extragere pică **doar** fișierele din blocul stricat, iar ele nu ajung la `Commit` |
| `TestAbortLeavesIncompletePackage` | după întrerupere: manifest incomplet, volum în curs rămas `.tmp`, nimic care să pară valid |
| `TestRefusesNonEmptyDir` | nu suprascriem un director cu conținut |
| `TestSizeMismatchIsRecorded` | un fișier crescut între scan și backup se reține la dimensiunea reală |
| `restore.TestPlanAndExecute` | planul rezolvă destinații, conflictul păstrează existentul și scrie alături, rădăcinile inexistente merg sub `DHS-restored`, mod și mtime se refac, niciun `.dhs-tmp` nu rămâne |
| `restore.TestPlanConflictPolicies` | `skip` nu atinge nimic; `overwrite` înlocuiește |
| `restore.TestPlanCaseCollisionOnWindows` | `Report.txt` și `report.txt` ajung în două fișiere pe Windows |
| `restore.TestSanitizeWindows` | caractere interzise, nume rezervate, puncte finale — adaptate |
| `internal/scan`, `internal/system` | clasificare, excluderi, secrete, inventar, estimare, detectare OS |

Dacă ceva pică, **nu se trece la pasul 2**. Se notează în `docs/NOTES.md` și se repară întâi.

## Pasul 2 — flux real, sursă → destinație

### Pe sursă

```bash
# un profil de test, cu ceva din fiecare clasă
mkdir -p ~/Documents ~/Pictures ~/Downloads
head -c 5M /dev/urandom > ~/Pictures/photo.jpg           # incompresibil
cp ~/Pictures/photo.jpg ~/Downloads/copy.jpg             # duplicat
seq 1 2000000 > ~/Documents/numbers.txt                  # text, comprimabil
head -c 300M /dev/urandom > ~/Downloads/big.bin          # mai mare decât un volum? nu la 3,5 GiB —
                                                         # pentru asta, testul unitar e cel care contează

./dhs scan --dest /tmp                                   # estimarea, fără să scrie
echo 'test-passphrase-for-codespaces' > /tmp/passphrase
./dhs backup --dest /tmp --name test --passphrase-file /tmp/passphrase --yes --verify
./dhs verify /tmp/test.dhs --passphrase-file /tmp/passphrase
cat /tmp/test.dhs/dhs.json                               # fără nume de fișiere, fără user
cat /tmp/test.dhs/SHA256SUMS
tar -cf /tmp/test.dhs.tar -C /tmp test.dhs
```

Verificări cu ochiul: manifestul spune `"complete": true`; `volumes/` conține doar `.dhsv`, niciun
`.tmp`; `SHA256SUMS` listează fiecare fișier din pachet.

### Întrerupere (tot pe sursă)

```bash
head -c 2G /dev/urandom > ~/Downloads/huge.bin
./dhs backup --dest /tmp --name interrupted --passphrase-file /tmp/passphrase --yes &
sleep 3; kill -INT %1; wait
cat /tmp/interrupted.dhs/dhs.json | grep complete        # false
ls /tmp/interrupted.dhs/volumes/                         # volumele încheiate .dhsv, cel în curs .tmp
./dhs verify /tmp/interrupted.dhs --passphrase-file /tmp/passphrase   # raportează „incomplet", nu cade
```

### Pe destinație

```bash
tar -xf /tmp/test.dhs.tar -C /tmp
./dhs verify /tmp/test.dhs --passphrase-file /tmp/passphrase
./dhs list /tmp/test.dhs --passphrase-file /tmp/passphrase --all
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase --dry-run
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase
sha256sum ~/Pictures/photo.jpg ~/Downloads/copy.jpg ~/Documents/numbers.txt

# conflict: rulează din nou — existentul rămâne, restauratul apare cu „ (DHS)"
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase --yes
ls ~/Documents/                                          # numbers.txt și „numbers (DHS).txt"
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase --conflicts skip --dry-run
```

Sumele trebuie să fie identice cu cele de pe sursă. **Asta e testul care contează.**

## Pasul 3 — ce se notează la final

În `docs/NOTES.md`: versiunea testată (commit), ce a trecut, ce a picat, timpi (cât a durat
backup-ul pe câți GiB, la ce nivel), dimensiunea binarului. Orice problemă în `docs/NOTES.md`.
