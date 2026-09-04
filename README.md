<p align="center"><img src="assets/dhs.svg" width="128" alt="DHS"></p>

# DHS — Direct Handoff Suite

🌐 **[dhs-suite.vercel.app](https://dhs-suite.vercel.app)** · 🇷🇴 [Versiunea în română](README.ro.md)
Moves your working environment from one operating system to another: **Windows ↔ Linux**, one
Linux distribution to another, one Windows version to another. You build a **portable migration
package** on an SSD or a USB stick, install the new system, and take your data, settings and
applications back.

No cloud. No account. No network. No AI. A binary of a few megabytes.

> **Status: under construction.** `scan`, `backup`, `verify`, `list` and `restore` work and are
> green on the test Codespaces. The app manifest, app detection and `plan` are not written yet.
> Do not use it for a real migration yet.

## What it does

```bash
dhs scan --dest /run/media/you/SSD     # what would be saved, how much it takes, whether it fits
```

```
System        Arch Linux (amd64)
Roots         /home/you/Documents
              /home/you/Pictures
              …

To include    44.0 GiB in 68 000 files   (scanned in ~1 s)
Excluded      6.2 GiB   --all brings them back
                target                 5.9 GiB · build artifacts
                node_modules           237 MiB · dependencies, restored by npm install
Secrets       1.2 MiB in 9 files   excluded by default; --secrets includes them

Composition
                binary           20.0 GiB     46%
                incompressible   11.8 GiB     27%   stored, not compressed
                text             2.4 GiB      6%

Level         2 · Balanced
Estimate      27.4 GiB – 34.4 GiB   ~3 min on 8 cores
Volumes       10 × 3.5 GiB

Destination   /run/media/you/SSD   64.0 GiB free of 119 GiB   exFAT

              ✓ Fits, with 29.6 GiB to spare.
```

The estimate comes **before** anything is written. If it does not fit, it tells you how much is
missing and what you can do about it.

```bash
dhs backup --dest /run/media/you/SSD --name laptop --level 2   # writes migration-<date>.dhs, or the --name you give
dhs verify /run/media/you/SSD/laptop.dhs                        # every checksum, extracts nothing
dhs list   /run/media/you/SSD/laptop.dhs --all                  # what is inside
dhs restore /run/media/you/SSD/laptop.dhs --dry-run             # the plan, writes nothing
dhs restore /run/media/you/SSD/laptop.dhs                       # shows the plan, asks, then writes
dhs plan    /run/media/you/SSD/laptop.dhs                       # which applications would be installed here, and how
dhs install /run/media/you/SSD/laptop.dhs                       # shows the commands, asks once, runs them
```

Restoring never overwrites: an existing file stays, and the restored one lands beside it with the
suffix ` (DHS)` — unless you say `--conflicts skip` or `--conflicts overwrite`. Files whose root does
not exist on the new system go under `~/DHS-restored/`. Every command takes `--json`;
`DHS_LANG=ro` switches the messages to Romanian.

**Applications.** `scan` and `backup` also look at what is installed — through the package
managers on Linux, through *Apps & features*, winget, scoop and choco on Windows — and match it
against DHS's own [application database](appdb/README.md) — 759 applications at the time of
writing — one JSON file per application, saying
how each platform's managers know it, where it keeps its configuration, and what stands in for it
where it does not exist. Configuration travels with the package, filed by application rather than
by path, so a Firefox profile from `%APPDATA%` lands in `~/.mozilla/firefox` on Linux. On the new
system, `dhs plan` says what would be installed and through which manager, what is already there,
what has no version for this platform and which equivalent would replace it, and what the database
does not know — DHS never guesses an application. `dhs install` runs exactly the commands `plan`
showed, after one confirmation. The database is community-maintained; adding an application is a
pull request with one file.

## Install

**[The latest release](https://github.com/Necta14/dhs/releases/latest) is a pre-release.** The file
core is tested; the app manifest and app detection are not written yet. Do not migrate anything you
care about with it yet.

On **Windows**, in PowerShell:

```powershell
irm https://dhs-suite.vercel.app/install.ps1 | iex
```

It picks the build for your processor, checks it against the published SHA-256 sums, unpacks it
under your own profile and puts it on your PATH. No administrator rights, nothing left running.

On **Linux**:

```bash
sudo apt install ./dhs_<version>_amd64.deb     # Debian, Ubuntu, Mint
sudo rpm -i dhs-<version>-1.x86_64.rpm         # Fedora, RHEL, openSUSE
makepkg -si                                    # Arch: dhs-cli and dhs-gui, from packaging/aur/
chmod +x DHS-<version>-x86_64.AppImage         # anything else, installing nothing
```

Check what you downloaded against the sums published beside it:

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

## How it is built

- **A single static binary**, in Go. On the freshly installed destination system there is no
  runtime — so we depend on none. 4.2 MiB on Linux, 4.5 MiB on Windows.
- **No network code.** The app database is compiled into the binary.
- **A package in 3.5 GiB volumes**, under the FAT32 limit of 4 GiB. If they do not fit on one
  medium, they are split across several.
- **Encrypted with a passphrase**, by default. Secrets — SSH keys, browser passwords — are excluded
  unless you ask for them explicitly, and get a separate passphrase when you do.
- **Nothing lossy.** A restored file is identical bit for bit to the original, or it is not written
  at all.

## Building

```bash
go build -ldflags="-s -w" -o dhs ./cmd/dhs        # Linux
GOOS=windows go build -ldflags="-s -w" ./cmd/dhs  # Windows, from Linux, no extra toolchain
gofmt -l . && go vet ./...                        # static checks; the tests run on Codespaces, see docs/TESTING.md
```

OS-specific code lives only in files with the `_linux.go` and `_windows.go` suffix. The Linux binary
**contains not a single byte** of the Windows code, and vice versa.

## Linux GUI prototype

`gui/linux/dhs-gui.py` is a GTK4 + libadwaita front end (PyGObject) that drives the CLI over
`--json` — the core stays in one place, the GUI is just a shell over it.

```bash
go build -o dhs ./cmd/dhs        # the GUI needs the binary at the repo root, or DHS_BIN set
python3 gui/linux/dhs-gui.py
```

## Documentation

| | |
|---|---|
| [`CLAUDE.md`](CLAUDE.md) | what the product is, its rules, the decision log |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | the package format, the code structure, the CLI surface |
| [`docs/COMPRESSION.md`](docs/COMPRESSION.md) | the three levels, the estimate, the research on extreme compression |
| [`docs/LICENSE-CHOICE.md`](docs/LICENSE-CHOICE.md) | why Apache-2.0 and not a licence with an ethical clause |
| [`docs/TESTING.md`](docs/TESTING.md) | the dedicated test session on GitHub Codespaces |
| [`VALUES.md`](VALUES.md) | what the project believes |
| [`AGENTS.md`](AGENTS.md) | rules for the agents working on the repo |

Romanian translations of all documents live in [`docs/ro/`](docs/ro/).

## License

[Apache License 2.0](LICENSE). The code is free. The name is not — see
[`TRADEMARK.md`](TRADEMARK.md).

**Brand.** The logo is [`assets/dhs.svg`](assets/dhs.svg) and the wordmark
[`assets/dhs-wordmark.svg`](assets/dhs-wordmark.svg). They are covered by the trademark policy, not
by the code licence.
