# BACKLOG — DHS

What does not go into v1. The order within each section is a proposal, not a commitment.

## v1 — what the core still lacks

- [x] `dhs scan` — inventory, classification, exclusions, estimate, destination check
- [ ] `--precise` — sampling for a measured report + hashing for deduplication
- [x] `internal/pack` — the format: 3.5 GiB volumes, solid blocks per class, file-level dedup, journal, checksums, encrypted index, redundant index per volume — **green on Codespaces** (see `TESTING.md`)
- [x] `internal/passphrase` — `age` with a passphrase — **green on Codespaces**
- [ ] Separate section for secrets, with its own passphrase (D4) — the format allows it, not implemented
- [ ] Resuming an interrupted backup from the journal + partial `index.dhsi` — the format allows it, the command does not exist
- [x] `dhs backup` — writing the package — **green on Codespaces**
- [x] `dhs verify` — full verification, without extraction — **green on Codespaces**
- [x] `dhs list` — package contents, summary or full — **green on Codespaces**
- [x] `dhs restore` — plan (destinations, conflicts, adapted names, case collisions) → confirmation → write through a temporary file + hash check + rename — **green on Codespaces**
- [ ] `internal/appdb` — the app database (TOML + `go:embed`) and its queries
- [ ] Detection of installed apps: `pacman`/`dpkg`/`rpm`/`flatpak`/`snap` on Linux, registry + `winget` on Windows
- [ ] `dhs plan` — manifest + appdb → restore plan, without touching anything
- [ ] `dhs report` — what stayed unknown or untranslated
- [ ] The 10–15 apps with portable configurations (D3)
- [ ] Splitting the package across several media

## Quality and infrastructure

- [ ] CI: `go vet`, `go test`, builds for linux/amd64, linux/arm64, windows/amd64, windows/arm64
- [ ] Size budget checked in CI (Rule #4: binary ≤ 15 MiB)
- [ ] Integration tests on VMs: a Windows → Linux migration carried through to the end
- [ ] `CONTRIBUTING.md` with the DCO rule and how to add an app to the database
- [x] The name in `NOTICE` — settled 2026-09-03: Necta (https://github.com/Necta14)

## After v1

- [ ] **GUI** — separate process, speaks JSON with the CLI (D9). Linux: GTK4 + libadwaita; a PyGObject prototype exists in `gui/linux/dhs-gui.py`, the production version may be rewritten with `gotk4`. Windows: native Win32 or WinUI
- [ ] **preflate-style preprocessing** for level 3 (D8): docx/xlsx/pptx, PDF, installers. Only with bit-identical verification per stream
- [ ] Lossless JPEG recompression (~20% on a photo collection) — evaluated and postponed, see `COMPRESSION.md`
- [ ] Block-level deduplication, `srep` style
- [ ] Incremental backup on top of an existing package
- [ ] `dhs watch` — a package kept up to date
- [ ] Translation of complex configurations, Windows registry → Linux files
- [ ] Sync between devices
- [ ] Enterprise support, mass deployment
- [ ] Integration with distribution installers ("have a DHS package? I'll restore it now")

## Open points

- How we communicate that a lost passphrase means a lost package, without scaring an ordinary user
- Files larger than a volume: cutting across volumes (designed, implemented in `pack`; not yet exercised on a real multi-volume package)
- KeePassXC: the password database falls under "secrets", so opt-in
- Steam: the configuration yes, the game library no — excluded by default
