# NOTES — DHS

Session journal. Newest at the top. Decisions live in `CLAUDE.md`; this is *what happened*.

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
