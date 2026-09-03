# CLAUDE.md — DHS · Direct Handoff Suite

> This file describes **the DHS product**. The working rules for agents are in
> [`AGENTS.md`](AGENTS.md), the journal in [`docs/NOTES.md`](docs/NOTES.md), the tasks in
> [`docs/BACKLOG.md`](docs/BACKLOG.md).
>
> **State as of 2026-09-03.** The file core is implemented and **green on Codespaces**: `scan`,
> `backup`, `verify`, `list`, `restore` — unit tests, race detector and a complete backup → restore
> flow with bit-for-bit comparison. Missing: the app database, app detection and `plan`. Tests run
> **only** on Codespaces (AGENTS.md, rule 7); how: [`docs/TESTING.md`](docs/TESTING.md).

## What DHS is

An **open source** tool that migrates a user's working environment between operating systems,
quickly and efficiently:

- Windows → Linux
- Linux → Windows
- one Linux distribution → another distribution
- one Windows version → another version

The central idea: a **portable migration package**, written to an external medium (SSD / HDD /
USB). **The cloud is never mandatory** — that is a property of the product, not a preference.

### The migration package

1. The user's personal files
2. Application configurations
3. A manifest of the installed applications
4. User configurations and preferences
5. The metadata needed for restoration

### The flow, on an example (Windows 11 Pro → Arch Linux)

**On the source (Windows):** install DHS → choose the external medium → choose the personal data
and the applications → DHS builds the package (documents, pictures/videos, application profiles,
configurations, the list of installed applications, Windows → Linux conversion rules).

**On the destination (after installing Arch):** install DHS → choose the package → DHS analyses the
manifest → identifies the Linux alternatives → installs the available applications → restores the
compatible files and configurations.

### The app database

Our own, not borrowed. It holds, per application:

- whether it is supported and under which identifiers it is known on each platform
- the equivalents across platforms (e.g. Notepad++ → Kate / VSCodium)
- the install methods: `winget` / `choco` / `scoop` on Windows; `pacman` / `apt` / `dnf` /
  `flatpak` on Linux
- the configuration locations on each operating system
- how portable the configuration is: identical / translatable / untranslatable

## The scope of version 1

**Only this goes into v1:**

- creating a backup
- restoring a backup
- the app manifest
- operating system detection
- Windows and Linux support
- a working CLI

**Explicitly postponed after v1:** automatic migration of complex configurations, sync between
devices, enterprise support, deep integration with distributions, GUI.

Any request outside the v1 list goes into `docs/BACKLOG.md`. It is not implemented ad hoc.

## Interfaces

- **CLI** — advanced users and automation. Comes first.
- **GUI** — ordinary users. Comes after the CLI core is stable.

A non-optional architectural consequence: **all the logic lives in a core library**, and the CLI and
GUI are thin wrappers over it. No business rule in the interface layer.

## ⛔ Rule #1 — the user's data is sacred

DHS handles someone's entire digital life: documents, photos, browser profiles, keys. A mistake does
not mean a bug, it means data lost for good. Proposals, **to be confirmed** (see D4):

- restoration **never overwrites** without explicit confirmation; by default, conflicts are kept
  side by side, not replaced
- every destructive operation has `--dry-run` and uses it as the presentation mode before asking
  for confirmation
- the package has **verifiable checksums**; restoration refuses a package that does not verify
- the process is **resumable** after an interruption (yanked cable, battery, full disk) — no
  half-written packages that look valid
- **secrets** (SSH/GPG keys, browser passwords, tokens) are treated separately from the rest of the
  data

## ⛔ Rule #2 — DHS contains no AI

**The shipped product has no AI, no RAG, no embeddings, and calls no model.** DHS is a deterministic
system tool: it reads, packs, verifies, restores. The user must be able to predict exactly what it
does.

Matching applications across platforms is done **through the human-curated database**, with exact
identifiers — not through semantic search, not through "clever guessing". If an application is not
in the database, DHS says it does not know and lists it in the report; it does not invent an
equivalent.

Direct consequence: no network dependency at runtime, no API key in the product, complete offline
operation.

## ⛔ Rule #4 — no bloat

DHS is not allowed to become bloatware. The budgets are thresholds, not suggestions:

| | Budget | Now |
|---|---|---|
| CLI binary, stripped (`-ldflags "-s -w"`) | ≤ 15 MiB | **4.2 MiB** Linux · **4.5 MiB** Windows (with age, zstd, xz) |
| Background processes, services, daemons | **zero** | zero |
| Telemetry, "anonymous statistics", auto-updater | **zero** | zero |
| Network calls at runtime | **zero** | zero |
| Runtime to install on the destination system | **none** | none |
| Cold start | < 100 ms | ~10 ms |

Every new dependency is justified. A Go package that opens a socket does not go into the product.

## The internal project-management system (RAG)

The memory tool used by the agents has moved, per D2, to **`~/dhs-memory`** — its own repo. It is
not part of the product, is not shipped, and does not influence its architecture.

```bash
cd ~/dhs-memory
node src/cli.ts recall "why we chose Go" -n dhs      # the product's decisions
node src/cli.ts handoff -n dhs                       # summary for the next session
node src/cli.ts recall "how the RAG is built" -n mem
```

## The stack

**Go** — a single static binary, no runtime on the destination system. The app database goes into
the binary through `go:embed`, so DHS works completely offline.

**Windows and Linux are developed separately, but from the same repo.** Build tags do the work:
platform-specific code lives only in `_linux.go` and `_windows.go`, and the Linux binary **contains
not a single byte** of the Windows code. You do not pay for the other platform. Cross-compiling is
one command, with no extra toolchain:

```bash
go build ./cmd/dhs                 # Linux
GOOS=windows go build ./cmd/dhs    # Windows, from Linux
```

**The GUI is a separate process that talks to the CLI through JSON** (`dhs <command> --json`). That
way each platform gets its own native interface, written in whatever suits it, without duplicating
the logic and without a Linux user downloading Windows code. See D9. A Linux prototype exists:
`gui/linux/dhs-gui.py` (GTK4 + libadwaita through PyGObject).

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — the package format, the module structure, the
  CLI surface, the app database schema
- [`docs/COMPRESSION.md`](docs/COMPRESSION.md) — the three compression levels, the size estimate
  before the backup, the research on "extreme" compression
- [`docs/LICENSE-CHOICE.md`](docs/LICENSE-CHOICE.md) — the licence options, the contribution model,
  what the Cyber Resilience Act says about open source
- [`docs/TESTING.md`](docs/TESTING.md) — the dedicated test session on Codespaces

## ⛔ Rule #3 — nothing lossy

A restored file must be **identical bit for bit** to the original. We are migrating someone's
wedding photos, not game assets. Any compression technique that cannot guarantee exact
reconstruction is outside the product, however much it would gain in size.

## Decisions taken

- **D1 — the stack · 2026-09-02: Go.** A single static binary, cross-compiled from Arch to Windows
  with one command, a stdlib that covers exactly the needs (archives, hashing, filesystem,
  processes), the Windows registry through `x/sys`. A small learning curve → contributors easier to
  find.
- **D2 — the relationship with the RAG tool · 2026-09-02.** There is no conflict: DHS is the
  product, the RAG is the project's internal management system. The product contains no AI or RAG
  ([Rule #2](#-rule-2--dhs-contains-no-ai)). The internal tool stays separate from the shipped
  code; when the product's implementation starts, it moves to its own repo, so the public repo
  contains only DHS.
- **D3 — configurations in v1 · 2026-09-02: files + manifest + a small list.** File copying, the
  app manifest, plus 10–15 applications with genuinely portable configurations. The rest is archived
  and reported, **not translated**. No promises we cannot keep.
- **D4 — security · 2026-09-02: encrypted by default, secrets opt-in.** The package is encrypted
  with a passphrase from the start. Secrets (SSH/GPG keys, browser passwords, cloud tokens) are
  **excluded by default** and included only with an explicit opt-in and a warning. A lost SSD does
  not become a total leak. **Clarified 2026-09-02:** the passphrase covers **all** the files in the
  package, and encryption is the user's choice — on by default, but it can be turned off. Secrets,
  when included, live in a section with **its own passphrase**, so the package can be handed to
  someone for the documents without also handing them the keys.
- **D7 — unencrypted mode · 2026-09-02, follows from D4.** Since encryption is a choice, the
  tension disappears: whoever wants a package that opens on any computer, without DHS, chooses
  level 1 (ZIP) and turns encryption off, with a visible warning. Whoever wants confidentiality
  leaves encryption on and accepts that DHS is needed to open it.
- **Rule #3 — nothing lossy · 2026-09-02.** See above. The safety of preprocessing does not come
  from avoiding the "important" files — DHS has no way of knowing which they are — but from
  **verification**: every preprocessed stream is recomposed and compared byte for byte on the spot,
  and at restore time the rebuilt file is checked against the original's SHA-256.
- **D6 — the licence · 2026-09-02: Apache-2.0.** Permissive, but with `NOTICE` (attribution that
  travels into every derivative), a trademark clause (§6 — nobody can call a fork "DHS") and a
  patent grant. Deliberately **without** an ethical usage clause: that would take DHS out of the
  open source definition and make it unpublishable in Debian, Fedora, Arch, openSUSE and nixpkgs —
  exactly the audience the tool exists for. The project's position is defended differently:
  [`TRADEMARK.md`](TRADEMARK.md) forbids using the **name** in a product that implements age
  verification, and [`VALUES.md`](VALUES.md) says so publicly. Trademark law can be enforced;
  "evil" cannot. Contributions through **DCO**; the app database gets **CC-BY-4.0**.
- **Code structure · 2026-09-02.** Windows and Linux are developed separately through build tags,
  not through separate repos. Package names and identifiers are in English (the language of
  comments and messages was settled later, by D10).
- **D5 — installation at restore time · 2026-09-02: approved plan, then execution.** DHS shows
  exactly what it installs, from which source and with which commands; the user approves once, then
  it runs automatically. Nobody blindly executes root commands generated from a file on a stick.
- **D10 — language · 2026-09-03: English is primary.** Code, comments, documentation and the CLI
  are in English. Romanian is maintained as a translation: [`README.ro.md`](README.ro.md),
  [`docs/ro/`](docs/ro/), and the CLI catalog `internal/i18n/ro.go` (selected with `DHS_LANG=ro` or
  from the locale). Discussion with the maintainer may happen in Romanian. Reason: the project is
  open source and aimed at people moving between operating systems anywhere; the reference
  documentation must be readable by any contributor.
- **NOTICE holder · 2026-09-03.** The copyright line in `NOTICE` names the maintainer's public
  handle, Necta (https://github.com/Necta14) — a person, not an entity. Unblocks the first release.

## Open decisions

| # | Decision | State |
|---|---|---|
| **D8** | How we implement preflate-style preprocessing, when we get there: a reimplementation in Go (clean binary, a few weeks) or `preflate-rs` through cgo (fast, but needs `mingw` for Windows). **Not decided now** — v1 ships level 3 as LZMA2 without preprocessing, and the format leaves room for it. | postponed until that stage |
| **D9** | Which GUI toolkit on each platform. **Settled for Windows on 2026-09-03: hand-drawn Win32.** A portable single `.exe` that looks current ruled out the alternatives: WinUI needs the Windows App SDK, web-based front ends drag in a runtime, and themed Win32 controls look like a 2009 utility. So the window is Win32 and the contents are drawn, with the libadwaita palette and metrics, so both interfaces read as one program: `gui/windows/`, about 2.9 MiB, no cgo. On Linux **GTK4 + libadwaita** stands (`gui/linux/dhs-gui.py`); `gotk4` remains the Go route if the prototype is ever rewritten. | Windows done, Linux prototype stands |

## Conventions

- Code, comments, documentation and the CLI are in **English**. Romanian is maintained as a
  translation: `README.ro.md`, `docs/ro/`, and the CLI catalog `internal/i18n/ro.go` (selected with
  `DHS_LANG=ro` or from the locale). Discussion with the maintainer may happen in Romanian.
- Every phase ends with a commit. Working branch: `main`.
- No files are deleted without confirmation. No working code is rewritten.
- Short answers: the conclusion in 1–3 lines, the details in the documentation, not in the chat.
