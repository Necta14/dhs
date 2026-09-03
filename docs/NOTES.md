# NOTES — DHS

Jurnal de sesiuni. Cel mai nou sus. Deciziile stau în `CLAUDE.md`; aici e *ce s-a întâmplat*.

## 03.09.2026 — prima sesiune de teste pe Codespaces: verde (Claude Fable 5.1)

**Rezultat.** Pe `3a39aef`: gofmt, vet, build linux/windows/arm64, `go test` (2 s), `go test
-race` (11 s), fluxul e2e (11 s) — **0 eșecuri**. Pe `6a55e1d`, `e2e-distro.sh`: backup pe Ubuntu,
restaurare în **Arch, Fedora, Debian, openSUSE, Alpine** — 5/5 identice bit cu bit, cu binarul
static de 4,3 MiB și nicio dependență în container.

**Ce au găsit rundele, în ordine.**
1. `NewWriter` scria manifestul înainte să existe emițătorul → nil pointer; bloca toate testele.
2. Patru curse de date: emițătorul serializa intrări în timp ce `Add` le adăuga părți (un bloc
   solid ține coada unui fișier terminat până se umple). Acum `addPart` sub lacăt, emițătorul
   serializează copii.
3. `octeti_stocati` dublat la închiderea volumului — contor, nu disc. Real: 12,9 → 8,6 MiB.
4. „Blocaj" de 10 minute sub `-race`: scrypt 2^18 apelat de sute de ori. `passphrase.WorkFactor`
   e variabilă; testele o coboară la 2^10, produsul rămâne la 2^18.
5. Încă două curse pe `progress`. Plus trei defecte în scripturile de test: `tee /dev/stderr`
   trunchia jurnalul, `pipefail` pica testul cu fraza greșită, `grep` fără rezultate omora scriptul.

**Infrastructură.** `gh` de pe laptop are două conturi: atelierul (**activ**, îl folosesc ceilalți
agenți — nu se schimbă) și Necta14 (scope `codespace`). Comenzile către Codespaces:
`GH_TOKEN=$(gh auth token -u Necta14) gh codespace ssh -c <nume> -- '<comandă>'`. Docker e în
imaginea implicită, `/dev/kvm` există, `sudo` fără parolă. Distrobox nu e necesar: containere
Docker simple fac exact ce trebuie.

**Următorul pas.** Partea de aplicații: `internal/appdb` (TOML + `go:embed`), detectarea
aplicațiilor instalate, `dhs plan`. De răspuns de user: numele din `NOTICE`.

## 02.09.2026 — definirea produsului și nucleul pentru fișiere (Claude Fable 5.1 / Opus 5)

**Dimineața.** S-a construit întâi unealta internă de memorie (RAG), apoi userul a definit
produsul. Deciziile D1–D7 s-au închis în aceeași zi: Go, fără AI în produs, fișiere + manifest +
listă mică de aplicații în v1, criptat implicit cu secrete opt-in, restaurare prin plan aprobat,
Apache-2.0 cu politică de marcă în loc de clauză etică. Research pe compresia „extremă" (FitGirl):
partea utilă e precomp, amânată (D8); Regula #3, nimic cu pierderi.

**După-amiaza.** Unealta RAG s-a mutat în `~/dhs-memory`. S-au scris `internal/system`,
`internal/scan`, `cmd/dhs scan` — rulat pe profilul real: 68 463 fișiere, 43,7 GiB, ~1 s. Apoi
`internal/pack` (formatul), `internal/passphrase`, `internal/restore`, comenzile `backup`,
`verify`, `list`, `restore` — **scrise, compilate, netestate**, conform regulii că testele
rulează doar pe Codespaces. Repo-ul a devenit public: github.com/Necta14/dhs.

**Bug găsit înainte de prima rulare.** Detectarea lua directorul acasă din `/etc/passwd`, nu din
`$HOME`; ar fi făcut imposibil testul pe profil sintetic.
