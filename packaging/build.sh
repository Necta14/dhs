#!/usr/bin/env bash
# Builds every release artifact into dist/: the binaries, their archives, the source tarball and
# SHA256SUMS. Reproducible from a clean checkout; needs nothing but the Go toolchain.
#
#   bash packaging/build.sh 0.1.0
#
# The version reaches the binary through -ldflags "-X main.version=…", so `dhs version` reports it.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-${DHS_VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}}"
VERSION="${VERSION#v}"
DIST="dist"
LDFLAGS="-s -w -X main.version=${VERSION}"
DOCS=(LICENSE NOTICE README.md README.ro.md)

rm -rf "$DIST"
mkdir -p "$DIST/build"

echo "── dhs ${VERSION} · $(go version | awk '{print $3}')"

# one <os> <arch>: a static binary plus the documents that must travel with it
one() {
  # Separate statements on purpose: `local a=$1 b=${a}` expands every word before the first
  # assignment happens, so ${a} would still be unset there — and fatal under `set -u`.
  local os="$1"
  local arch="$2"
  local ext=""
  local dir="$DIST/build/${os}_${arch}"
  [ "$os" = windows ] && ext=".exe"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$dir/dhs${ext}" ./cmd/dhs
  # Windows also gets the graphical front end, which is a separate process over the same binary.
  if [ "$os" = windows ]; then
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "$LDFLAGS -H windowsgui" -o "$dir/dhs-gui.exe" ./gui/windows
  fi
  cp "${DOCS[@]}" "$dir/"

  local base="dhs_${VERSION}_${os}_${arch}"
  if [ "$os" = windows ]; then
    if command -v zip >/dev/null 2>&1; then
      ( cd "$dir" && zip -q -9 "../../${base}.zip" "dhs${ext}" dhs-gui.exe "${DOCS[@]}" )
    else
      # No zip on the host: Python's zipfile writes the same archive.
      ( cd "$dir" && python3 -c "
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for name in sys.argv[2:]:
        z.write(name)
" "../../${base}.zip" "dhs${ext}" dhs-gui.exe "${DOCS[@]}" )
    fi
  else
    tar -czf "$DIST/${base}.tar.gz" -C "$dir" "dhs${ext}" "${DOCS[@]}"
  fi
  if [ "$os" = windows ]; then
    printf '   %-22s %s + gui %s\n' "${os}/${arch}" "$(du -h "$dir/dhs${ext}" | cut -f1)" "$(du -h "$dir/dhs-gui.exe" | cut -f1)"
  else
    printf '   %-22s %s\n' "${os}/${arch}" "$(du -h "$dir/dhs${ext}" | cut -f1)"
  fi
}

one linux   amd64
one linux   arm64
one windows amd64
one windows arm64

# The source tarball: what AUR and anyone rebuilding from scratch consumes.
git archive --format=tar.gz --prefix="dhs-${VERSION}/" \
  -o "$DIST/dhs-${VERSION}-source.tar.gz" HEAD
printf '   %-22s %s\n' "source" "$(du -h "$DIST/dhs-${VERSION}-source.tar.gz" | cut -f1)"

# Checksums over every artifact, the binaries inside their archives included.
( cd "$DIST" && sha256sum ./*.tar.gz ./*.zip > SHA256SUMS )

echo "── artifacts in $DIST/"
ls -1 "$DIST" | sed 's/^/   /'
