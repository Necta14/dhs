# NOTES — DHS

Session journal. Newest at the top. Decisions live in `CLAUDE.md`; this is *what happened*.

## 2026-09-03 - 0.1.3, and a native interface on Windows

The Windows interface exists: `gui/windows`, one portable executable under 3 MiB, Win32 drawn by
hand, no cgo, nothing installed and no elevation asked. It drives the same binary over `--json`
like the GTK4 front end, and takes libadwaita's palette and metrics so the two read as one program.

The route there is worth recording. Themed common controls were not the problem: the manifest was
correct, verified by walking the PE resource directory. A grey dialog with static labels and a tab
strip simply looks like 2009 whatever the theme does, so the contents are now drawn: header,
sidebar, cards, toggle switches, buttons. Real `EDIT` controls stay for text entry, with the sunken
border replaced by a drawn frame, because nobody should reimplement selection and IME behaviour.

The constraint that shaped `render.go`: **GDI+ cannot be called from Go for anything taking a
float.** The Microsoft x64 convention passes floats in XMM registers and Go's `syscall` puts every
argument in an integer register, so `GdipCreateFont` would receive rubbish. Rather than carry an
assembly trampoline, shapes are drawn into a surface twice the size with integer GDI and shrunk
with a halftone stretch; text is drawn afterwards at native size so ClearType stays crisp. Two
passes over one layout.

Verified by clicking the Scan button through the VM's own input channel, then reading the screen
back through the QEMU monitor: the binary ran and the panel filled with the real system line, home
directory and scanned places.

Two defects closed on the way:

- **A mistyped flag exited 0.** Every command swallowed parse errors with `return nil`. Found
  because a wrong flag in the Windows harness looked like three clean runs in a row.
- **Nothing was compiling the Windows interface.** It sits behind a build tag, so `go vet ./...` on
  Linux never saw it. The Codespace suite now vets the Windows tree and builds both interfaces for
  both architectures: thirteen steps, all green.

0.1.3 is published with complete packages: the Windows archives carry both executables, and D9 is
settled for Windows. It stays a pre-release until the application database exists, because matching
programs across systems is the reason the tool exists and that part is still unwritten.

## 2026-09-03 - the first run on real Windows

Windows 11 IoT Enterprise LTSC 24H2, PowerShell 5.1, in a local VM driven over SSH. The 0.1.2
release binary, installed by our own `install.ps1`, ran the whole cycle: scan, backup with
`--verify`, verify, list, a refused wrong passphrase, a dry run, a restore into an emptied profile
and a second restore over existing files.

**All eight test files came back identical bit for bit**, including a name with Romanian diacritics
written through the real Win32 API, a compressible duplicate pair and an incompressible one. The
second restore placed all eight beside the originals with the ` (DHS)` suffix rather than
overwriting. The system line reads `Windows 11 IoT Enterprise LTSC 2024 24H2 (amd64)`.

`install.ps1` works on stock PowerShell 5.1 with Defender's real-time protection on: it resolves
the pre-release through the API, verifies the SHA-256 against the published sums, installs under
LOCALAPPDATA and updates the user PATH. **It is not flagged.**

Two real defects came out of the session:

- **A mistyped flag exited 0.** Every command swallowed flag parse errors with `return nil`, so
  `dhs scan --only foo` printed "flag provided but not defined" and then reported success. A
  script, a CI job or the GUI would carry on as if the command had run. Fixed: parse errors exit 2,
  `--help` still exits 0.
- **The output carried non-ASCII decoration.** The ellipsis in `install.ps1` rendered as a bare
  full stop on a real console, making lines look truncated. Now ASCII only.

One thing that looked like a defect and was not: Defender flagged
`powershell -NoProfile -ExecutionPolicy Bypass -Command irm <url> | iex` as
`Trojan:Win32/Commando.A!ml` and killed it mid-run. That is the child-process form used by the test
harness, not the documented interactive one-liner, which is clean. Worth knowing before anyone
automates the install.

Three defects were mine, in the harness, and are worth recording because they cost three rounds:
`$Args` is an automatic PowerShell variable and cannot be a parameter name; `@(Invoke-RestMethod)`
nests the already-deserialised array one level deeper; and wrapping a pipeline that ends in
`Select-Object -First 1` in parentheses yields `$null` on 5.1. The last one is why the harness
failed where `install.ps1`, which does not wrap it, works.

## 2026-09-03 — 0.1.2: a real defect, found by testing Windows under Wine

The Windows binary had never been run. Wine is not Windows, but it is close enough to be worth an
hour: `wine64` on Ubuntu ran `dhs.exe` against a Wine-built user profile, and the whole cycle,
scan, backup, verify, list, restore, works. It reports `Windows 10 Pro`, finds the profile roots,
reads free space on NTFS and refuses a wrong passphrase.

It also found a **data-integrity defect, and not a Windows one**. A duplicate of a *compressible*
file was written with no block references: the dedup branch copied the original's parts at the
moment it was added, while the original's bytes were still in an open solid block, and every part
that arrived afterwards went to the original alone. The package verified as intact and restoring
that one file failed with `corrupt data`, writing nothing. Safe behaviour, lost file.

Why three rounds of green tests missed it: the only duplicate fixture is `pictures/copy.jpg`,
Incompressible, and stored bytes get their part immediately. Duplicates never went through a solid
block in any test. `TestDuplicateInSolidBlockKeepsItsParts` now covers a Text and a Binary
duplicate, and asserts the index entry has parts at all rather than only comparing bytes. Verified
to fail against the old writer and pass against the new one.

0.1.2 carries the fix; 0.1.0 and 0.1.1 are marked superseded in their release notes, with the way
to tell whether a package is affected (`dhs list --all` marks duplicates).

Also in 0.1.2: `site/install.ps1`, so Windows installs with
`irm https://dhs-suite.vercel.app/install.ps1 | iex`. It resolves the release through the API
rather than `/releases/latest`, which skips pre-releases and would have found nothing, verifies the
SHA-256 before unpacking, installs under LOCALAPPDATA and adds that to the user PATH. Exercised in
Microsoft's PowerShell container: default run, pinned version, and a version that does not exist.

The packages were checked on the distributions they target: `.deb` on Debian and Ubuntu, `.rpm` on
Fedora, Rocky and openSUSE, the tarball on Alpine against musl, the AppImage without FUSE.

One false alarm worth recording: a filename with diacritics appeared mangled and unreadable in the
first Wine run. It was the container's C locale, through which Wine translates Unix filenames. With
`LANG=C.UTF-8` the file backs up and restores byte for byte.

## 2026-09-03 — 0.1.1: desktop integration and two AUR packages

0.1.1 exists because the GTK4 front end could not be installed: it had no desktop entry and no
icon. Both are now in `gui/linux/`, named after the application id the GUI registers
(`io.github.necta14.dhs`) so the shell matches the window to its icon.

The AUR recipe became a split package: `pkgbase=dhs` with `pkgname=('dhs-cli' 'dhs-gui')`, so one
push publishes both. `dhs-cli` is the binary and also `provides=('dhs')`; `dhs-gui` is `arch=any`
because it is a Python script, and depends on `dhs-cli` plus the GTK stack. The distribution
package builds with `-buildmode=pie`, which Go links internally with CGO off, so the binary stays
static; the release artifacts stay plain static binaries.

Validated in a clean Arch container: `makepkg` builds both, `check()` runs the test suite, both
install, `desktop-file-validate` is clean, GTK4 and libadwaita import, and `dhs version` answers.
Three namcap warnings survive and are explained in `packaging/aur/README.md`; none is a defect.

Not submitted yet: the AUR needs an account with a registered SSH key, which only the maintainer
can create.

## 2026-09-03 — 0.1.0 published, with packages

The first pre-release: https://github.com/Necta14/dhs/releases/tag/v0.1.0, eleven files. Binaries
and archives for Linux and Windows on amd64 and arm64, `.deb` for both Debian architectures, `.rpm`
for both RPM ones, an x86_64 AppImage, the source tarball and `SHA256SUMS` over all of it.

Built by `packaging/release.sh` on the test Codespace, not by hand. Three defects surfaced on that
first real run and are fixed in the scripts: `local a=$1 b=${a}` expands before it assigns and died
under `set -u`; `dpkg-deb` refused the control directory's permissions; the containerised
`rpmbuild` left root-owned output the host could not copy. The AppImage step now runs the artifact
it just wrote.

Verified in clean containers: the `.deb` installs on Debian, the `.rpm` on Fedora, the AUR recipe
builds on Arch through `makepkg` with `check()` running the test suite, and the binary answers
`dhs version` in all three.

The AUR recipe is ready in `packaging/aur/` but not submitted: that needs the maintainer's own SSH
key on aur.archlinux.org. `packaging/aur/README.md` has the commands.

Also: the site gained SEO metadata, structured data and a sitemap, and is verified in Google Search
Console. The repository now carries a description, the site URL and discovery topics.

## 2026-09-03 — English becomes the primary language (Claude Fable 5.1)

**What changed.** Code, comments, documentation and the CLI are now in English; Romanian stays as
a translation (`README.ro.md`, `docs/ro/`, and the CLI catalog `internal/i18n/ro.go`, selected with
`DHS_LANG=ro` or from the locale). The English docs are the reference: `docs/ARCHITECTURE.md`,
`docs/COMPRESSION.md`, `docs/LICENSE-CHOICE.md`, `docs/TESTING.md`; the old Romanian paths are
stubs that point both ways. Package layout and CLI flags were renamed to English at the same time
(`SHA256SUMS`, `journal.jsonl`, `volumes/`, `--passphrase-file`, `--yes`, `--conflicts keep-both|skip|overwrite`,
`~/DHS-restored/`), and the test script became `scripts/codespace-tests.sh`, publishing to
`tests/<codespace>` in `test-results/SUMMARY.md`. Personal data (machine names, paths, accounts)
was scrubbed from the docs. The `NOTICE` holder is settled: Necta (https://github.com/Necta14).

**Also.** A Linux GUI prototype, `gui/linux/dhs-gui.py` (GTK4 + libadwaita through PyGObject),
drives the CLI over `--json` — the D9 architecture, tried by hand. Brand assets in `assets/`.

## 2026-09-03 — first test session on Codespaces: green (Claude Fable 5.1)

**Result.** On `3a39aef`: gofmt, vet, linux/windows/arm64 builds, `go test` (2 s), `go test
-race` (11 s), the e2e flow (11 s) — **0 failures**. On `6a55e1d`, `e2e-distro.sh`: backup on
Ubuntu, restore in **Arch, Fedora, Debian, openSUSE, Alpine** — 5/5 identical bit for bit, with the
4.3 MiB static binary and no dependency in the container.

**What the rounds found, in order.**
1. `NewWriter` wrote the manifest before the emitter existed → nil pointer; blocked every test.
2. Four data races: the emitter serialised entries while `Add` was still appending parts to them (a
   solid block holds the tail of a finished file until it fills up). Now `addPart` runs under the
   lock and the emitter serialises copies.
3. `stored_bytes` doubled when a volume closed — a counter, not the disk. Real: 12.9 → 8.6 MiB.
4. A 10-minute "hang" under `-race`: scrypt 2^18 called hundreds of times. `passphrase.WorkFactor`
   is a variable; the tests lower it to 2^10, the product stays at 2^18.
5. Two more races on `progress`. Plus three defects in the test scripts: `tee /dev/stderr`
   truncated the log, `pipefail` failed the wrong-passphrase test, `grep` with no matches killed
   the script.

**Infrastructure.** `gh` on the maintainer's machine holds two accounts; the one that owns the
Codespaces has the `codespace` scope and is used per command, without switching the active
account: `GH_TOKEN=$(gh auth token -u <user>) gh codespace ssh -c <codespace> -- '<command>'`.
Docker is in the default image, `/dev/kvm` exists, `sudo` needs no password. Distrobox is not
needed: plain Docker containers do exactly what is required.

**Next step.** The apps side: `internal/appdb` (TOML + `go:embed`), detection of installed apps,
`dhs plan`.

## 2026-09-02 — product definition and the file core (Claude Fable 5.1 / Opus 5)

**Morning.** The internal memory tool (RAG) was built first, then the maintainer defined the
product. Decisions D1–D7 were closed the same day: Go, no AI in the product, files + manifest + a
small list of apps in v1, encrypted by default with opt-in secrets, restore through an approved
plan, Apache-2.0 with a trademark policy instead of an ethical clause. Research on "extreme"
compression (FitGirl): the useful part is precomp, postponed (D8); Rule #3, nothing lossy.

**Afternoon.** The RAG tool moved to `~/dhs-memory`. `internal/system`, `internal/scan`,
`cmd/dhs scan` were written — run on a real profile: about 68 000 files, 44 GiB, ~1 s. Then
`internal/pack` (the format), `internal/passphrase`, `internal/restore`, the commands `backup`,
`verify`, `list`, `restore` — **written, compiled, untested**, following the rule that tests run
only on Codespaces. The repo went public: github.com/Necta14/dhs.

**Bug found before the first run.** Detection took the home directory from `/etc/passwd`, not from
`$HOME`; it would have made the test on a synthetic profile impossible.
