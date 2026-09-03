# DHS architecture — proposal for v1

> State as of 2026-09-03: `scan`, `backup`, `verify`, `list`, `restore` are implemented and
> **green on Codespaces** ([`TESTING.md`](TESTING.md)); `appdb`, app detection and `plan` are not
> written. Points marked ⚠️ still need confirmation.
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

Lives encrypted, in the first volume.

```json
{
  "system": { "os": "windows", "version": "11 Pro 23H2", "user": "you" },
  "apps": [
    {
      "id": "mozilla.firefox",
      "name": "Mozilla Firefox",
      "version": "142.0",
      "detected_via": "winget",
      "config": { "state": "captured", "portability": "identical" }
    },
    {
      "id": null,
      "name": "Internal Program Ltd v3",
      "detected_via": "registry",
      "unknown": true
    }
  ]
}
```

Unknown applications stay in the manifest with `id: null`. DHS **does not invent** an equivalent —
it lists them in the final report as "I don't know what this is, you deal with it".

## The app database

TOML files, one per application, embedded in the binary with `go:embed`. Contributable through pull
requests, with no service needed.

```toml
id = "mozilla.firefox"
name = "Mozilla Firefox"
category = "browser"

[windows]
detect   = { winget = "Mozilla.Firefox", registry = ["HKLM\\SOFTWARE\\Mozilla\\Mozilla Firefox"] }
install  = [{ manager = "winget", id = "Mozilla.Firefox" }, { manager = "choco", id = "firefox" }]
config   = ["%APPDATA%/Mozilla/Firefox/Profiles"]

[linux]
detect   = { pacman = "firefox", dpkg = "firefox", flatpak = "org.mozilla.firefox" }
install  = [{ manager = "pacman", id = "firefox" }, { manager = "apt", id = "firefox" },
            { manager = "flatpak", id = "org.mozilla.firefox" }]
config   = ["~/.mozilla/firefox"]

[portability]
config = "identical"      # identical | translatable | untranslatable
notes  = "The profile is portable across platforms; the profile directory is copied."

# For applications that do not exist on the other platform:
# equivalents = ["kde.kate", "vscodium"]
```

The 10–15 applications of v1 (D3), proposal: Firefox, Chrome/Chromium, VSCode/VSCodium, Git, SSH
(`~/.ssh/config`, no keys), Thunderbird, VLC, Obsidian, KeePassXC ⚠️, GIMP, LibreOffice, Discord,
Steam ⚠️ (configuration only, not the game library).

⚠️ To discuss: KeePassXC holds the password database — it belongs under "secrets", so opt-in. Steam
can have hundreds of GB; it must be excluded by default, with a separate opt-in.

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
  appdb/                  the app database (go:embed) + queries
  apps/                   detection of installed apps, install plan
```

OS-specific code lives **only** in files with the `_linux.go` and `_windows.go` suffix. Thanks to
build tags, the Linux binary contains not a single byte of the Windows code — hence the sizes:
4.2 MiB on Linux, 4.5 MiB on Windows.

The rule: the `_linux.go` / `_windows.go` files in `internal/system` are the only places with
OS-specific code. The rest is shared and testable on any machine.

## CLI surface

```bash
dhs scan                                    # what would be included, how much it takes, what apps were found
           --dest /run/media/you/SSD        # checks whether it fits, estimated compression included
           --level 2                        # 1 compatible | 2 balanced | 3 maximum
           --secrets | --all                # include secrets | exclude nothing
           --precise                        # sampling + dedup: measured estimate, not assumed (not yet implemented)
dhs backup --dest /run/media/you/SSD        # creates the package (asks for the passphrase)
           --name test                      # default: migration-<date>
           --level 2 --secrets --all
           --no-encrypt                     # unencrypted package, with a visible warning (D7)
           --passphrase-file <path> --yes --verify
dhs verify <package>                        # checks every checksum, extracts nothing
dhs list   <package> [--all]                # contents: summary or full listing
dhs restore <package>                       # default: shows the plan and asks for approval
            --dry-run                       # plan only, writes nothing
            --only documents,pictures       # only these roots
            --conflicts keep-both|skip|overwrite
dhs plan   <package>                        # (planned) what would be installed and restored; touches nothing
dhs report <package>                        # (planned) what stayed unknown or untranslated
dhs version
```

Every command accepts `--json` — the GUI and automation speak to the CLI through it. The roots are
`documents`, `pictures`, `videos`, `music`, `downloads`, `desktop`, `config` and `other`. Files
whose root does not exist on the target system, and everything under `other`, are restored under
`~/DHS-restored/`. A conflict keeps the existing file and writes the restored one next to it with
the suffix " (DHS)", unless `--conflicts` says otherwise. Environment: `DHS_LANG` (`en`/`ro`,
default from the locale) and `DHS_DEBUG` (diagnostic output for bug reports).

`dhs restore` with no extra arguments **executes nothing** until it sees the plan approved. `plan`
is the command anyone can run, at any time, with no risk.

## Privileges

- **Backup** runs as a regular user. Your own profile does not require administrator.
- **Restoring files** also runs as a regular user.
- **Installing applications** is the only step that requires `sudo` / administrator, and it is
  isolated exactly there, after the plan is approved.

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
