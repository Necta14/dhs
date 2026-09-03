#!/usr/bin/env bash
# Builds every artifact for a release, in order, into dist/.
#
#   bash packaging/release.sh 0.1.0
#
# Needs: the Go toolchain always; zip for the Windows archives; dpkg-deb or ar for the .deb;
# rpmbuild or docker for the .rpm; network access once, for appimagetool. Steps that cannot run
# are reported and skipped rather than failing the whole release — the summary says what is there.
set -uo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-${DHS_VERSION:-}}"
[ -n "$VERSION" ] || { echo "usage: bash packaging/release.sh <version>"; exit 1; }
VERSION="${VERSION#v}"
SKIPPED=()

step() { # <name> <script...>
  local name="$1"; shift
  if bash "$@" "$VERSION"; then
    return 0
  fi
  echo "   ✗ ${name} skipped"
  SKIPPED+=("$name")
}

bash packaging/build.sh "$VERSION" || { echo "the binaries failed to build; stopping"; exit 1; }
step deb      packaging/deb.sh
step rpm      packaging/rpm.sh
step appimage packaging/appimage.sh

echo
echo "══ dist/ ══"
( cd dist && ls -1sh -- *.tar.gz *.zip *.deb *.rpm *.AppImage 2>/dev/null | sed 's/^/   /' )
echo
echo "── SHA256SUMS"
sed 's/^/   /' dist/SHA256SUMS
if [ "${#SKIPPED[@]}" -gt 0 ]; then
  echo
  echo "── not built here: ${SKIPPED[*]}"
fi
