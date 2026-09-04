# Changelog

All notable changes to DHS. The format follows [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/). Every release carries the same set of
artifacts: archives for Linux and Windows (amd64 and arm64), `.deb`, `.rpm`, an AppImage, the source
archive and `SHA256SUMS`.

## [0.2.0] — 2026-09-04

The release that gives the product its name: applications travel too.

### Added
- **The application database**, `appdb/`: 759 entries, one JSON file per application, embedded in
  the binary. For each: the identifiers under which winget, choco, scoop, pacman, the AUR, apt,
  dnf, zypper, apk, Flatpak and Snap know it, the display name in *Apps & features*, the
  configuration locations keyed the same way on every platform, how portable that configuration
  is, and the equivalents where the application does not exist. CC-BY-4.0, contributable by pull
  request; `appdb/README.md` is the guide, `appdb/tools/validate.py` the check.
- **Detection of installed applications**. Linux: the pacman, dpkg, apk, Flatpak and Snap
  databases are read directly, rpm is queried. Windows: the *Uninstall* registry keys, `winget
  export`, scoop and choco. Configuration is found where it lives, including inside Flatpak and
  Snap sandboxes.
- **Configuration carried by application**, under package roots `apps/<id>/<key>`, so a Firefox
  profile from `%APPDATA%` lands in `~/.mozilla/firefox`. The manifest `dhs/apps.json` is an
  encrypted entry in the first volume.
- **`dhs plan`**: what would be installed here and through which manager, what is already here,
  what has no way to be installed, what has no version for this platform and which equivalent
  replaces it, what the database does not know, and where every configuration goes. Touches
  nothing.
- **`dhs install`**: runs exactly the commands `plan` showed, after one confirmation; a batch that
  fails is retried one package at a time so the report names what did not install.
- `dhs restore` places application configuration through the same plan and keeps the rest under
  `~/DHS-restored/apps/` with the reason; `--only apps` selects it.
- `--no-apps` on `scan` and `backup`; an *Applications* page in both graphical interfaces.
- Browser credential stores (`logins.json`, `key4.db`, `Login Data`) are treated as secrets.

### Changed
- Unknown applications are reported only for packages that ship a desktop entry, and Windows
  redistributables and runtimes are left out; a Linux system's thousand libraries no longer bury
  the report.
- Binary size: 5.4 MiB on Linux, 5.6 MiB on Windows, database included.

### Notes
- This is the first release not marked pre-release. The file core has been verified bit for bit
  on five Linux distributions and on Windows 11; the application side is covered by unit tests and
  the end-to-end script on Linux. Windows detection has been exercised only through its unit
  tests so far.
- Identifiers in the database were checked against Repology, winget-pkgs, Flathub and the Snap
  store where doubtful. A wrong name is found by someone who has the application installed;
  please report them.

## [0.1.3] — 2026-09-03
### Added
- A native Windows interface, `dhs-gui.exe`: a portable single executable, Win32 drawn by hand
  with libadwaita's palette and metrics, no runtime, no elevation.
### Fixed
- A mistyped flag exited with status 0; it now exits 2.

## [0.1.2] — 2026-09-03
### Fixed
- A deduplicated compressible file lost its block references and came back empty at restore
  time. Found under Wine; the test now asserts on the index, not only on the bytes.

## [0.1.1] — 2026-09-03
### Added
- Desktop file and icon; the AUR recipe with two packages, `dhs-cli` and `dhs-gui`.

## [0.1.0] — 2026-09-03
### Added
- The file core: `scan`, `backup`, `verify`, `list`, `restore`. Encrypted packages on external
  media, 3.5 GiB volumes, solid compression per file class, file-level deduplication, resumable
  journal, bit-for-bit verification at restore time. Linux and Windows, amd64 and arm64.

[0.2.0]: https://github.com/Necta14/dhs/releases/tag/v0.2.0
[0.1.3]: https://github.com/Necta14/dhs/releases/tag/v0.1.3
[0.1.2]: https://github.com/Necta14/dhs/releases/tag/v0.1.2
[0.1.1]: https://github.com/Necta14/dhs/releases/tag/v0.1.1
[0.1.0]: https://github.com/Necta14/dhs/releases/tag/v0.1.0
