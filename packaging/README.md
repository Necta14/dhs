# Packaging

Everything a release needs, as scripts rather than as steps someone remembers. Each one takes the
version as its only argument and writes into `dist/`, which is not tracked by git.

```bash
bash packaging/release.sh 0.1.0        # all of the below, in order
```

| Script | Produces | Needs |
|---|---|---|
| `build.sh` | binaries and archives for Linux and Windows on amd64 and arm64, the source tarball, `SHA256SUMS` | Go, `zip` |
| `deb.sh` | `dhs_<version>_{amd64,arm64}.deb` | `dpkg-deb`, or `ar` and `tar` as a fallback |
| `rpm.sh` | `dhs-<version>-1.{x86_64,aarch64}.rpm` | `rpmbuild`, or `docker` to borrow Fedora's |
| `appimage.sh` | `DHS-<version>-x86_64.AppImage` | network once, for `appimagetool` |
| `aur.sh` | updates `aur/PKGBUILD` and `aur/.SRCINFO` for a published release | `makepkg` if present |

The `.deb` and `.rpm` install the same three things: `/usr/bin/dhs`, and `LICENSE` plus `NOTICE`
under the distribution's documentation directory. Apache-2.0 requires `NOTICE` to travel with the
software, so it is not optional. Nothing is installed that runs on its own: no service, no timer,
no post-install hook.

The AppImage wraps the command-line tool, which is unusual but useful: it runs on any distribution
without installing anything.

```bash
./DHS-0.1.0-x86_64.AppImage scan --dest /run/media/you/SSD
```

## Where these run

On the maintainer's machine only `build.sh` is expected to work end to end; the Debian, RPM and
AppImage tooling lives on the test Codespaces (`docs/TESTING.md`, `AGENTS.md` rule 7). The scripts
skip cleanly when a tool is missing, and `release.sh` reports what it could not build.

## Verifying a download

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

`SHA256SUMS` covers the archives and the packages, and is attached to every release.
