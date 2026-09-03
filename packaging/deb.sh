#!/usr/bin/env bash
# Builds .deb packages from the binaries that packaging/build.sh left in dist/build/.
# Uses dpkg-deb when it is there, and falls back to assembling the archive with ar and tar,
# which needs no Debian tooling at all.
#
#   bash packaging/build.sh 0.1.0 && bash packaging/deb.sh 0.1.0
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-${DHS_VERSION:-$(git describe --tags --always 2>/dev/null || echo 0.0.0)}}"
VERSION="${VERSION#v}"
DIST="dist"
MAINTAINER="Necta <179446771+Necta14@users.noreply.github.com>"

one() {
  local arch="$1"
  local src="$DIST/build/linux_${arch}/dhs"
  [ -f "$src" ] || { echo "missing $src — run packaging/build.sh first"; exit 1; }

  local root; root="$(mktemp -d)"
  install -Dm755 "$src" "$root/usr/bin/dhs"
  install -Dm644 LICENSE "$root/usr/share/doc/dhs/LICENSE"
  install -Dm644 NOTICE  "$root/usr/share/doc/dhs/NOTICE"
  install -Dm644 README.md "$root/usr/share/doc/dhs/README.md"

  mkdir -p "$root/DEBIAN"
  cat > "$root/DEBIAN/control" <<EOF
Package: dhs
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${arch}
Maintainer: ${MAINTAINER}
Homepage: https://dhs-suite.vercel.app
Installed-Size: $(( $(stat -c%s "$src") / 1024 + 8 ))
Description: Migrate a working environment between operating systems
 DHS builds a portable migration package on an external drive: personal files,
 application configurations, a manifest of the installed applications and the
 metadata needed to put it all back on a freshly installed system.
 .
 The package is encrypted with a passphrase, split into volumes that fit any
 filesystem, and verified with SHA-256 on the way in and on the way out. It
 makes no network calls, needs no account and contains no AI: applications are
 matched across platforms through a curated database, not by guessing.
 .
 A single static binary. Nothing runs in the background.
EOF

  local out="$DIST/dhs_${VERSION}_${arch}.deb"
  if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --root-owner-group -Zxz -b "$root" "$out" >/dev/null
  else
    # Assemble the ar archive by hand: debian-binary, control.tar.xz, data.tar.xz, in that order.
    local staging; staging="$(mktemp -d)"
    echo "2.0" > "$staging/debian-binary"
    tar -C "$root/DEBIAN" -cJf "$staging/control.tar.xz" --owner=root --group=root .
    tar -C "$root" --exclude=./DEBIAN -cJf "$staging/data.tar.xz" --owner=root --group=root .
    ( cd "$staging" && ar rc "$OLDPWD/$out" debian-binary control.tar.xz data.tar.xz )
    rm -rf "$staging"
  fi
  rm -rf "$root"
  printf '   %-24s %s\n' "$(basename "$out")" "$(du -h "$out" | cut -f1)"
}

echo "── deb ${VERSION}"
one amd64
one arm64
( cd "$DIST" && sha256sum ./*.deb >> SHA256SUMS )
