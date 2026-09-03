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

## Reaching users

The order matters more than the list. A tool that says "do not use this for a real migration"
spends its one-time launch attention badly, so nothing below happens before v1 works end to end
on Windows as well as Linux. The website is live at https://dhs-suite.vercel.app and the
repository points at it.

**Before any announcement**
- [ ] A v1 release with attached binaries for Linux and Windows, plus `SHA256SUMS` next to them.
      Nothing spreads that people cannot download and run.
- [ ] A recording in the README: `scan`, `backup`, `restore` in one terminal, and the GUI beside
      it. A migration tool is judged on whether the output looks trustworthy.
- [ ] `dhs backup` and `dhs restore` exercised on real Windows, not only cross-compiled.

**Where the audience already is** (people mid-migration, searching for exactly this)
- [ ] Reddit: r/linux, r/linux4noobs, r/DistroHopping, r/selfhosted. Post as an author, not as an
      advert: what it does, what it does not do yet, and ask what breaks.
- [ ] Hacker News, as "Show HN". One shot, weekday morning US time.
- [ ] Lobsters, Fosstodon and the other Mastodon instances where Linux tooling circulates.
- [ ] Existing threads asking "how do I move from Windows to Linux without losing my files" —
      answer the question, mention the tool only if it genuinely fits.

**Packaging is distribution**
- [ ] AUR first: Arch users are the distribution-hopping audience, and a `PKGBUILD` is a day's work.
- [ ] Then `winget` and `scoop` for Windows, `.deb` and `.rpm`, and Flathub once the GUI is real.
- [ ] Every packaging line is also a backlink and an entry in someone's package search.

**Directories and lists**
- [ ] AlternativeTo, as an alternative to the vendor migration assistants.
- [ ] awesome-go, awesome-linux, awesome-selfhosted, awesome-privacy.
- [ ] LibHunt and the Go package index pick these up on their own once the topics are set.

**What actually compounds**
- [ ] Answer every issue within a day, even with "not yet, here is why".
- [ ] A short post per milestone rather than one big launch: the app database, the GUI, Windows
      support. Each one is a fresh reason for someone to link to the project.
- [ ] Keep the "under construction" banner honest. For a tool that handles someone's whole home
      directory, credibility is the only real advantage over the alternatives.

## Open points

- How we communicate that a lost passphrase means a lost package, without scaring an ordinary user
- Files larger than a volume: cutting across volumes (designed, implemented in `pack`; not yet exercised on a real multi-volume package)
- KeePassXC: the password database falls under "secrets", so opt-in
- Steam: the configuration yes, the game library no — excluded by default
