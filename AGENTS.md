# AGENTS.md — DHS

Rules for any agent working on this repo. What the product is and what has been decided:
[`CLAUDE.md`](CLAUDE.md). Do not start without reading it.

1. **The user's data is sacred.** The tool handles someone's documents, photos and keys. A mistake
   is not a bug, it is lost data. Nothing is overwritten without confirmation, every destructive
   step has `--dry-run`, and whatever cannot be verified is not written.
2. **No network in the product.** Zero HTTP calls, zero dependencies that make calls, zero
   telemetry. If a Go package you want to add opens a socket, it does not go in.
3. **No AI in the product.** No embeddings, no models, no "smart matching". The app database is
   written by people, with exact identifiers.
4. **No bloat.** Every new dependency is justified. The budgets in Rule #4 (CLAUDE.md) are
   thresholds, not suggestions: the CLI binary stays under 15 MiB.
5. **OS-specific code lives only** in `_linux.go` / `_windows.go` files. The rest is shared and must
   compile on any machine. Always check both platforms, statically:
   ```bash
   gofmt -l . && go vet ./... && go build ./... && GOOS=windows go build ./...
   ```
   `go test` — only on Codespaces, see rule 7.
6. **Nothing lossy.** Every data transformation must be reversible bit for bit, and that must be
   **proven** by verification, not assumed.
7. **Tests are NOT run on the maintainer's machine.** Neither `go test` nor the `dhs` binary on
   their data. Tests are written here, but they run **only in the dedicated session, on the two
   GitHub Codespaces** — `scripts/codespace-tests.sh`, following
   [`docs/TESTING.md`](docs/TESTING.md). Locally only the static checks are allowed: `gofmt`,
   `go vet`, `go build` for both platforms. The tests, when they run, do not touch the network and
   do not write outside `t.TempDir()`.
8. **Language.** Code, comments, documentation and the CLI are in **English**. Romanian is
   maintained as a translation: [`README.ro.md`](README.ro.md), [`docs/ro/`](docs/ro/), and the CLI
   catalog `internal/i18n/ro.go` (selected with `DHS_LANG=ro` or from the locale). Discussion with
   the maintainer may happen in Romanian. The project is open source; the code must be readable by
   anyone.
9. **`gofmt` and `go vet` clean** before every commit. No exceptions.
10. **What is not in v1 is not implemented.** The scope is in CLAUDE.md; everything else goes in
    [`docs/BACKLOG.md`](docs/BACKLOG.md).
11. **Sync.** At the start of a session read `CLAUDE.md`, `docs/BACKLOG.md` and the decision log.
    At the end, write down what you changed and why.
12. **The working branch is `main`.** Commit after every phase, with a message that says *what*
    and *why*.
