# DHS architecture — proposal for v1

> State as of 2026-09-04: `scan`, `backup`, `verify`, `list`, `restore` are implemented and
> **green on Codespaces** ([`TESTING.md`](TESTING.md)). The application side — the database
> (`appdb/`), detection, `plan` and `install` — is written and covered by unit tests and the e2e
> script; see the notes for the state of its verification. Points marked ⚠️ still need confirmation.
>
> Context and decisions: [`../CLAUDE.md`](../CLAUDE.md).

## The basic principle

A single static Go binary that runs identically on Windows and Linux, **with no network and no
AI**. All the logic lives in the library; the CLI is a thin wrapper, and the later GUI will be a
second wrapper over exactly the same library.

## The migration package format

The package is a **directory**, not a single file. The reason is practical: external media are
often formatted FAT32 or exFAT, and FAT32 does not accept files over 4 GB and does not preserve
permissions, ownership or symbolic links.

```
migration-2026-09-02-1014.dhs/
  dhs.json              # root manifest, UNENCRYPTED — see below for what it contains and what it doesn't
  index.dhsi            # the complete index (files, blocks, placement), ENCRYPTED; small, read first
  volumes/
    0001.dhsv           # blocks + own index + trailer, ENCRYPTED, ≤ 3.5 GiB
    0002.dhsv
    ...
  SHA256SUMS            # SHA-256 for every file in the package
  journal.jsonl         # one line per finished volume, for resuming after an interruption
```

Implemented in `internal/pack`: `format.go` (binary headers), `index.go` (the JSON structures),
`writer.go` + `emitter.go` (parallel, ordered writing with volume rotation), `reader.go` (opening,
verification, sequential extraction), `journal.go` (journal, checksums, atomic writes).

The index lives **in a separate, encrypted file** (`index.dhsi`), not at the start of the first
volume: `age` is a stream, it does not allow reading from the tail, and the index is not known
until the end. Every volume nevertheless also carries its own index, at its tail — if `index.dhsi`
is lost, it can be rebuilt.

### What is unencrypted, and why

`dhs.json` must be readable **before** the user types the passphrase, so that DHS can say what this
package is. So it contains strictly:

```json
{
  "format": 1,
  "id": "01J...",
  "created": "2026-09-02T10:14:00Z",
  "source": { "os": "windows", "version": "11 Pro 23H2", "arch": "x86_64" },
  "cipher": "age-scrypt",
  "volumes": 7,
  "bytes": 14332891136,
  "apps": 63,
  "secrets_included": false
}
```

It **does not contain** file names, paths, application names or the user name. The file list and
the app manifest live **inside the encrypted volumes** — otherwise the package would tell anyone
who finds it exactly which software and which documents you have.

### Volumes

Each volume is a stream written straight to disk, without holding anything large in memory: we go
just as fast on a laptop with 8 GB as on one with 64.

**3.5 GiB by default**, because FAT32 refuses files of 4 GiB or larger. Uniform on every file
system — the package stays movable to any medium, at any time.

The volume is also the unit of resumption: if the cable gets yanked, at most the current volume is
redone, not the whole backup. A file larger than a volume (an 8 GiB ISO) is cut across volumes, and
the manifest records that the pieces belong to the same file.

If the package does not fit on a single medium, the volumes are **split across several** —
`dhs.json` knows how many there are in total and which ones are missing at restore time.

### What is inside a volume

Not a single compressed stream, but **solid blocks grouped by file class**:

- **incompressible** (photos, video, music, archives) → stored, no compression
- **compressible** (text, code, documents, configurations) → solid compressed block, so that the
  redundancy between small files gets exploited
- **unknown** → entropy test on the first 256 KB, then into one of the classes above

Plus **file-level deduplication**: the SHA-256 sums are computed anyway for integrity, so a file that
appears three times is stored once.

Details, measurements and the three compression levels the user chooses from:
[`COMPRESSION.md`](COMPRESSION.md).

### Encryption

`age` with a passphrase (`filippo.io/age`, scrypt recipient). The reason we do not write our own
layer: `age` is designed and audited for exactly this, and hand-written cryptography is the classic
place where a project like this one makes a hole that nobody notices for two years.

**The passphrase covers the whole package** — every file, not a selection. Encryption is the user's
choice: on by default, it can be turned off (`--no-encrypt`), in which case level 1 yields a ZIP
that opens on any computer, without DHS.

**Secrets have their own passphrase.** When the user includes them explicitly (`--secrets`), they
live in a separate section, with a second passphrase. That way you can hand the package to someone
to recover your documents, without also handing them your SSH keys.

⚠️ **Lost passphrase = lost package, for good.** How we communicate this in the interface still has
to be decided (proposal: double confirmation at creation, plus a warning that cannot be skipped).

### Integrity and resume

- `SHA256SUMS` — SHA-256 for every file. `dhs verify` checks it in full.
- `journal.jsonl` — one line per finished volume. On resume, DHS reads the journal and continues
  from where it left off.
- A volume is written under a temporary name and renamed only once it is complete. So an
  interrupted package **never contains** a half-written volume that looks valid.

## The app manifest

`dhs/apps.json`, an ordinary entry of the package under the reserved root `dhs`: encrypted like
everything else, checksummed like everything else, never written to disk at restore time. It goes
in first, so it sits in the first volume and `dhs plan` can read it without touching the others.

```json
{
  "format": 1,
  "created": "2026-09-04T10:12:00Z",
  "database_entries": 512,
  "os": "windows",
  "managers": ["winget"],
  "apps": [
    {
      "id": "firefox", "name": "Mozilla Firefox", "category": "browser", "version": "142.0.1",
      "sources": [
        {"via": "registry", "package": "Mozilla Firefox (x64 en-US)", "name": "Mozilla Firefox (x64 en-US)", "version": "142.0.1"},
        {"via": "winget", "package": "Mozilla.Firefox", "version": "142.0.1"}
      ],
      "config": [{"key": "profiles", "path": "C:\\Users\\you\\AppData\\Roaming\\Mozilla\\Firefox"}]
    },
    {
      "id": "git", "name": "Git", "category": "vcs", "config_only": true,
      "config": [{"key": "gitconfig", "path": "C:\\Users\\you\\.gitconfig", "file": true}]
    }
  ],
  "unknown": [
    {"via": "registry", "package": "Internal Program Ltd v3", "name": "Internal Program Ltd v3", "version": "3.0"}
  ]
}
```

An application appears with every manager that reported it. `config_only` marks one that no
manager reported but whose configuration is on disk — a portable build, an AppImage, a leftover:
its files travel, nothing is proposed for installation on its account. Unknown applications stay
in `unknown`, with what the manager said about them. DHS **does not invent** an equivalent — it
lists them in the plan as "not in the database, nothing is installed for them".

### Configuration roots

The files of a configuration location are stored under a root of their own, `apps/<id>/<key>`,
with `@flatpak` or `@snap` appended when they came out of a sandboxed build's home. The key is
the one from the database, identical on every platform, so `apps/firefox/profiles` means "the
Firefox profiles, wherever this system keeps them". On the source, the location's absolute path is
registered as a root, so a directory such as `~/.config/Code/User` is filed under
`apps/vscode/user` rather than under `config`. On the target, `dhs plan` decides where each root
goes (below), and `dhs restore` registers those decisions before building its own file plan;
whatever is not placed lands under `~/DHS-restored/apps/<id>/<key>/`, with the reason.

A location that is a single file (`~/.gitconfig`) is stored with the path `.` under its root,
exactly like a file given directly to `dhs backup`.

## The app database

JSON, one file per application, in [`appdb/apps/`](../appdb/apps/), embedded in the binary with
`go:embed` and loaded once per process. No parser dependency: the standard library reads it, and a
test refuses to build a database that does not validate. The data is CC-BY-4.0, apart from the
code; the format, the rules and how to contribute are in [`appdb/README.md`](../appdb/README.md).

```json
{
  "id": "firefox",
  "name": "Mozilla Firefox",
  "category": "browser",
  "windows": {
    "winget": ["Mozilla.Firefox"], "choco": ["firefox"], "scoop": ["extras/firefox"],
    "registry": ["Mozilla Firefox"],
    "paths": { "profiles": "%APPDATA%/Mozilla/Firefox" }
  },
  "linux": {
    "pacman": ["firefox"], "apt": ["firefox", "firefox-esr"], "dnf": ["firefox"],
    "zypper": ["MozillaFirefox"], "apk": ["firefox"],
    "flatpak": ["org.mozilla.firefox"], "snap": ["firefox"],
    "paths": { "profiles": "~/.mozilla/firefox" }
  },
  "config": { "portability": "identical", "exclude": ["cache2", "startupCache", "*.lock"] },
  "equivalents": []
}
```

Each manager's list is both detection and installation: the names are what the manager reports
when the application is installed, and the first one is what DHS installs. `registry` is the
display name in Windows' *Apps & features* and only detects. `paths` are keyed, and the keys tie
the platforms together. `portability` says how far the configuration travels **between operating
systems**: `identical` is restored in place, `translatable` and `untranslatable` are kept aside
(v1 translates nothing, D3), `none` has no paths. On the same operating system everything is
restored in place. `equivalents` are the ids that stand in for the application where it does not
exist — curated, never guessed.

### Detection

- **Linux**: `/var/lib/pacman/local/*/desc`, `/var/lib/dpkg/status`, `/lib/apk/db/installed` and
  the Flatpak and Snap deployment directories are read directly; the rpm database is binary, so
  `rpm -qa` is run. Which manager is *native* comes from `ID` / `ID_LIKE` in `/etc/os-release`,
  not from which binaries exist.
- **Windows**: the three `Uninstall` registry keys (both architectures, machine and user), minus
  system components and updates — what the Settings app shows; then `winget export`, which is
  JSON and complete where `winget list` truncates; then the `scoop` and `choco` directories.
- Every package found is matched against the database, but only the ones that ship a **desktop
  entry** (`/usr/share/applications/*.desktop`, read from the package's file list) are reported as
  unknown when they do not match. A Linux system holds a thousand libraries and command-line
  packages; listing them as "unknown applications" would bury the report. Flatpaks, snaps and
  everything Windows shows in *Apps & features* are applications by definition and are always
  reported.
- Then, for every application the database knows on this platform, whether its configuration
  locations exist — natively, and inside the sandbox homes of the Flatpak or Snap builds it was
  seen in (`~/.var/app/<id>/config/…`, `~/snap/<name>/current/.config/…`).

### The plan

`dhs plan` is a pure function of the manifest, the database and the target system (its managers,
what is installed). For every application in the manifest: already installed here → nothing;
has a version here → the first manager in the target's order of preference that names it; no
version here → the first equivalent that can be installed, once, however many applications point
to it; otherwise reported as missing, with the reason. Managers not present on the target
(`flatpak` not installed) make an application "no way to install here", with what would do it.
For every configuration location: placed if the application is installed or being installed here
and, across operating systems, if its portability is `identical`; otherwise kept aside. When the
package holds both the native and the Flatpak copy of a profile, the one matching how the
application is present here is placed and the other kept aside.

The commands are part of the plan and are shown verbatim before anything runs (D5): `pacman -Syu
--needed` (never `-Sy`), `apt-get update` then `apt-get install -y`, `dnf install -y`,
`zypper --non-interactive install`, `apk add`, `flatpak install -y --noninteractive flathub`,
`snap install` per snap (some need `--classic`), `winget install --id … --exact --silent` per
package, `choco install -y`, `scoop install`. A batch that fails is retried one package at a time,
so the report names exactly what did not install. Root comes through `sudo`, `doas` or `run0`,
whichever exists; on Windows the installers raise their own prompts.

## Code structure

Code, comments, documentation and the CLI are in English; Romanian is maintained as a translation
(`README.ro.md`, `docs/ro/`, and the CLI catalog `internal/i18n/ro.go`, selected by `DHS_LANG` or
the locale). ✅ = already exists.

```
cmd/dhs/                  CLI — argument parsing and display only                   ✅
internal/
  system/                 OS detection, standard paths, free space, privileges      ✅
    system_linux.go       /etc/os-release, XDG user-dirs, statfs                    ✅
    system_windows.go     registry, Known Folders, GetDiskFreeSpaceEx               ✅
  scan/                   inventory, classification, exclusions, estimate           ✅
  report/                 formatting numbers for human eyes                         ✅
  pack/                   the format: writing, reading, volumes, journal, checksums ✅
  passphrase/             age with a passphrase                                     ✅
  restore/                restore plan + execution through a temporary file         ✅
  i18n/                   CLI messages: English default + Romanian catalog (DHS_LANG)   ✅
  apps/                   detection, the manifest, the install plan, running the commands ✅
appdb/                    the app database: JSON entries (CC-BY-4.0), loader, queries     ✅
```

OS-specific code lives **only** in files with the `_linux.go` and `_windows.go` suffix. Thanks to
build tags, the Linux binary contains not a single byte of the Windows code — hence the sizes:
4.9 MiB on Linux, 5.2 MiB on Windows, database included.

The rule: the `_linux.go` / `_windows.go` files in `internal/system` and `internal/apps` are the
only places with OS-specific code. The rest is shared and testable on any machine; the parsers of
the Linux package databases deliberately carry no build tag, so their tests run everywhere.

## CLI surface

```bash
dhs scan                                    # what would be included, how much it takes, what apps were found
           --dest /run/media/you/SSD        # checks whether it fits, estimated compression included
           --level 2                        # 1 compatible | 2 balanced | 3 maximum
           --secrets | --all                # include secrets | exclude nothing
           --no-apps                        # skip application detection
           --precise                        # sampling + dedup: measured estimate, not assumed (not yet implemented)
dhs backup --dest /run/media/you/SSD        # creates the package (asks for the passphrase)
           --name test                      # default: migration-<date>
           --level 2 --secrets --all --no-apps
           --no-encrypt                     # unencrypted package, with a visible warning (D7)
           --passphrase-file <path> --yes --verify
dhs verify <package>                        # checks every checksum, extracts nothing
dhs list   <package> [--all]                # contents: summary or full listing
dhs restore <package>                       # default: shows the plan and asks for approval
            --dry-run                       # plan only, writes nothing
            --only documents,pictures,apps  # only these roots; "apps" is every application's configuration
            --conflicts keep-both|skip|overwrite
dhs plan    <package>                       # what would be installed here and where each configuration goes; touches nothing
dhs install <package>                       # shows the plan with its commands, asks once, runs them
            --skip vlc,spotify              # leave applications out
            --dry-run --yes
dhs version
```

Every command accepts `--json` — the GUI and automation speak to the CLI through it. The roots are
`documents`, `pictures`, `videos`, `music`, `downloads`, `desktop`, `config` and `other`. Files
whose root does not exist on the target system, and everything under `other`, are restored under
`~/DHS-restored/`. A conflict keeps the existing file and writes the restored one next to it with
the suffix " (DHS)", unless `--conflicts` says otherwise. Environment: `DHS_LANG` (`en`/`ro`,
default from the locale) and `DHS_DEBUG` (diagnostic output for bug reports).

`dhs restore` with no extra arguments **executes nothing** until it sees the plan approved. `plan`
is the command anyone can run, at any time, with no risk: it reads, detects, decides, prints.
`install` runs exactly the commands `plan` printed, after one confirmation. What `plan` reports as
unknown or kept aside is the "report" the earlier design had as a separate command.

## Privileges

- **Backup** runs as a regular user. Your own profile does not require administrator.
- **Restoring files** also runs as a regular user.
- **Installing applications** is the only step that requires `sudo` / administrator, and it is
  isolated exactly there: `dhs install`, after the plan is approved. Restoring application
  configuration is part of `dhs restore` and needs nothing.

## What v1 does NOT do

So that it stays in black and white: no sync between devices, no cloud, no translation of complex
configurations, no Windows registry → dconf, no GUI, no `srep`-style block deduplication, no
`precomp` preprocessing, no incremental backup. All of them in `BACKLOG.md`.

**File-level** deduplication does go into v1, though: it is practically free, because the hashes are
computed anyway.

## Open points

1. ⚠️ How we communicate the risk of a lost passphrase, without scaring the ordinary user.
2. ⚠️ Default exclusions for files: `node_modules`, caches, Steam libraries, virtual machines.
   Proposal: a default list that is visible and editable before the backup.
3. D7 — decided (see CLAUDE.md): level 1 together with `--no-encrypt`, with a visible warning.
   See [`COMPRESSION.md` §4.3](COMPRESSION.md).
4. D6 — decided: Apache-2.0, see [`LICENSE-CHOICE.md`](LICENSE-CHOICE.md).
