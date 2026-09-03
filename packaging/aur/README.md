# Publishing to the AUR

The AUR holds only the recipe, not the code: `PKGBUILD` downloads the source tarball from the
GitHub release and builds it on the user's machine. `packaging/aur.sh <version>` keeps `pkgver`,
the checksum and `.SRCINFO` in step with a published release, so run that first.

Publishing needs an SSH key registered on the maintainer's aur.archlinux.org account, which is why
it is not automated here.

## Once, to claim the package name

```bash
git clone ssh://aur@aur.archlinux.org/dhs.git ~/aur-dhs   # empty repo on the first clone
cp packaging/aur/PKGBUILD packaging/aur/.SRCINFO ~/aur-dhs/
cd ~/aur-dhs && git add PKGBUILD .SRCINFO
git commit -m "Initial import: dhs 0.1.0"
git push
```

If the clone says the repository does not exist, the name is free and the first push creates it.

## For every later version

```bash
bash packaging/aur.sh 0.2.0                                # updates PKGBUILD and .SRCINFO
cp packaging/aur/PKGBUILD packaging/aur/.SRCINFO ~/aur-dhs/
cd ~/aur-dhs && git commit -am "dhs 0.2.0" && git push
```

## Before pushing, if you are on Arch

```bash
cd packaging/aur && makepkg -f            # builds and runs the test suite through check()
namcap PKGBUILD dhs-*.pkg.tar.zst         # the packaging linter, if it is installed
```

`makepkg` builds in place and leaves `src/` and `pkg/` behind; both are ignored by git.

## Conventions this package follows

- `license=('Apache-2.0')`, the SPDX identifier, as current Arch guidelines ask.
- `prepare()` downloads the Go modules, so `build()` runs with `-mod=readonly` and no surprises.
- `check()` runs `go test ./...`, so a broken release fails on the builder's machine rather than
  installing quietly.
- `NOTICE` is installed next to `LICENSE`: Apache-2.0 requires it to travel with the software.
