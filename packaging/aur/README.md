# Publishing to the AUR

One recipe, two packages. `pkgbase` is `dhs`, so the AUR repository is named `dhs`, and a single
push publishes both:

| Package | What it holds | Depends on |
|---|---|---|
| `dhs-cli` | `/usr/bin/dhs`, the tool itself | nothing |
| `dhs-gui` | `/usr/bin/dhs-gui`, the GTK4 front end, its desktop entry and icon | `dhs-cli`, `python-gobject`, `gtk4`, `libadwaita` |

`dhs-cli` also `provides=('dhs')`, so anything depending on `dhs` is satisfied. Installing
`dhs-gui` pulls in `dhs-cli`; the front end is a separate process that drives the CLI over
`--json`, which is decision D9.

The AUR holds only the recipe: `PKGBUILD` downloads the source tarball from the GitHub release and
builds it on the user's machine. `packaging/aur.sh <version>` keeps `pkgver`, the checksum and
`.SRCINFO` in step with a published release, so run that first.

## Before the first push, once

1. Create an account at <https://aur.archlinux.org/register>.
2. Paste a public SSH key into **My Account → SSH Public Key**. Any key works; a dedicated one
   keeps it separate from your GitHub key.
3. Point SSH at it, if the key is not your default:

```
Host aur.archlinux.org
    User aur
    IdentityFile ~/.ssh/aur
    IdentitiesOnly yes
```

`ssh aur@aur.archlinux.org help` answers with a command list once the key is registered. Until
then it says `Permission denied (publickey)`.

## Claiming the name

```bash
git clone ssh://aur@aur.archlinux.org/dhs.git ~/aur-dhs   # an empty repo on the first clone
cp packaging/aur/PKGBUILD packaging/aur/.SRCINFO ~/aur-dhs/
cd ~/aur-dhs && git add PKGBUILD .SRCINFO
git commit -m "Initial import: dhs 0.1.1 (dhs-cli, dhs-gui)"
git push
```

If the clone says the repository does not exist, the name is free and the first push creates it.

## For every later version

```bash
bash packaging/aur.sh 0.2.0                                # updates PKGBUILD and .SRCINFO
cp packaging/aur/PKGBUILD packaging/aur/.SRCINFO ~/aur-dhs/
cd ~/aur-dhs && git commit -am "dhs 0.2.0" && git push
```

## Checking it before pushing, on Arch

```bash
cd packaging/aur && makepkg -f            # builds both packages; check() runs the test suite
namcap PKGBUILD ./*.pkg.tar.zst           # the packaging linter, if installed
```

`makepkg` leaves `src/` and `pkg/` behind; both are ignored by git.

## Conventions this recipe follows

- `license=('Apache-2.0')`, the SPDX identifier, as current Arch guidelines ask.
- `prepare()` downloads the Go modules, so `build()` runs with `-mod=readonly` and no surprises.
- `check()` runs `go test ./...`, so a broken release fails on the builder's machine rather than
  installing quietly.
- `options=('!debug')`: the binary is linked with `-s -w`, so a split debug package would be empty.
- `arch=('any')` on `dhs-gui` alone, since it is a Python script.
- The desktop entry and the icon are named after the application id the GUI registers,
  `io.github.necta14.dhs`, so the shell matches the window to its icon.
- `NOTICE` is installed next to `LICENSE` in both packages: Apache-2.0 requires it to travel with
  the software.

## The namcap warnings that remain, and why

With both packages installed so namcap can resolve everything, three warnings are left. All three
are expected:

- **`dhs-cli W: ELF file lacks FULL RELRO`** — Go's internal linker does not emit `-z relro -z now`.
  Getting it would mean external linking through cgo, which would make the binary depend on the
  system glibc and destroy the property the whole project rests on: a binary that needs nothing
  installed on the destination system. `-buildmode=pie` is applied, which is the part that can be
  had without that cost.
- **`dhs-gui W: Referenced library 'python3' is an uninstalled dependency`** — the script's shebang
  names `python3`, while the Arch package that provides `/usr/bin/python3` is called `python`, which
  the package depends on. A naming artefact, not a missing dependency.
- **`dhs-gui W: Dependency included, but may not be needed ('dhs-cli')`** — the front end runs the
  `dhs` binary as a subprocess and parses its `--json` output. namcap reads linkage and imports, so
  it cannot see a runtime subprocess. The dependency is real and required.
