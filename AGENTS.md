# AGENTS.md — DHS

Reguli pentru orice agent care lucrează pe acest repo. Ce e produsul și ce s-a decis:
[`CLAUDE.md`](CLAUDE.md). Nu începe fără să-l citești.

1. **Datele utilizatorului sunt sacre.** Unealta umblă la documentele, pozele și cheile cuiva. O
   greșeală nu e un bug, sunt date pierdute. Nimic nu se suprascrie fără confirmare, orice pas
   distructiv are `--dry-run`, iar ce nu poate fi verificat nu se scrie.
2. **Fără rețea în produs.** Zero apeluri HTTP, zero dependențe care fac apeluri, zero telemetrie.
   Dacă un pachet Go pe care vrei să-l adaugi deschide un socket, nu intră.
3. **Fără AI în produs.** Fără embeddings, fără modele, fără „potrivire inteligentă". Baza de
   aplicații e scrisă de om, cu identificatori exacți.
4. **Fără balast.** Fiecare dependență nouă se justifică. Bugetele din Regula #4 (CLAUDE.md) sunt
   praguri, nu sugestii: binarul CLI rămâne sub 15 MiB.
5. **Codul specific unui OS stă doar** în fișiere `_linux.go` / `_windows.go`. Restul e comun și
   trebuie să se compileze și să se testeze pe orice mașină. Verifică mereu amândouă:
   ```bash
   go build ./... && GOOS=windows go build ./... && go vet ./... && go test ./...
   ```
6. **Nimic cu pierderi.** Orice transformare de date trebuie să fie reversibilă bit cu bit, și
   trebuie **dovedit** prin verificare, nu presupus.
7. **Testele nu ating rețeaua și nu scriu în afara `t.TempDir()`.**
8. **Limba.** Documentație, mesaje de interfață și comentarii în **română**; identificatorii și
   numele de pachete în **engleză**. Proiectul e open source, codul trebuie să fie citibil de
   oricine.
9. **`gofmt` și `go vet` curate** înainte de commit. Fără excepții.
10. **Ce nu e în v1 nu se implementează.** Domeniul e în CLAUDE.md; restul se scrie în
    [`docs/BACKLOG.md`](docs/BACKLOG.md).
11. **Sincronizare.** La început de sesiune citește `CLAUDE.md`, `docs/BACKLOG.md` și jurnalul de
    decizii. La final, notează ce ai schimbat și de ce.
12. **Ramura de lucru e `main`.** Commit după fiecare fază, cu mesaj care spune *ce* și *de ce*.
