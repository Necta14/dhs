#!/usr/bin/env bash
# Builds an x86_64 AppImage from the binary that packaging/build.sh left in dist/build/.
#
#   bash packaging/build.sh 0.1.0 && bash packaging/appimage.sh 0.1.0
#
# DHS is a command-line tool, so the AppImage is simply a self-contained way to run it on any
# distribution without installing anything: ./DHS-x86_64.AppImage scan --dest /run/media/you/SSD
# appimagetool is fetched from the AppImage project's own releases and cached under dist/.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-${DHS_VERSION:-$(git describe --tags --always 2>/dev/null || echo 0.0.0)}}"
VERSION="${VERSION#v}"
DIST="dist"
SRC="$DIST/build/linux_amd64/dhs"
[ -f "$SRC" ] || { echo "missing $SRC — run packaging/build.sh first"; exit 1; }

TOOL="$DIST/appimagetool-x86_64.AppImage"
if [ ! -x "$TOOL" ]; then
  echo "── fetching appimagetool"
  for url in \
    "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage" \
    "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage"
  do
    if curl -fsSL --retry 2 -o "$TOOL" "$url"; then break; fi
  done
  [ -s "$TOOL" ] || { echo "could not fetch appimagetool"; exit 1; }
  chmod +x "$TOOL"
fi

APPDIR="$DIST/AppDir"
rm -rf "$APPDIR"
install -Dm755 "$SRC" "$APPDIR/usr/bin/dhs"
install -Dm644 LICENSE "$APPDIR/usr/share/doc/dhs/LICENSE"
install -Dm644 NOTICE  "$APPDIR/usr/share/doc/dhs/NOTICE"
install -Dm644 assets/dhs.svg "$APPDIR/usr/share/icons/hicolor/scalable/apps/dhs.svg"
cp assets/dhs.svg "$APPDIR/dhs.svg"

cat > "$APPDIR/dhs.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=DHS
GenericName=Direct Handoff Suite
Comment=Migrate a working environment between operating systems
Exec=dhs
Icon=dhs
Terminal=true
Categories=Utility;Archiving;
Keywords=migration;backup;restore;
EOF
install -Dm644 "$APPDIR/dhs.desktop" "$APPDIR/usr/share/applications/dhs.desktop"

cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/dhs" "$@"
EOF
chmod +x "$APPDIR/AppRun"

OUT="$DIST/DHS-${VERSION}-x86_64.AppImage"
echo "── appimage ${VERSION}"
# --appimage-extract-and-run: no FUSE inside containers and CI runners.
ARCH=x86_64 VERSION="$VERSION" "$TOOL" --appimage-extract-and-run \
  --no-appstream "$APPDIR" "$OUT" >"$DIST/appimage.log" 2>&1 \
  || { echo "appimagetool failed:"; tail -20 "$DIST/appimage.log"; exit 1; }

# Prove the artifact actually runs. A desktop has FUSE; containers and CI runners do not, hence
# --appimage-extract-and-run, which unpacks to a temporary directory instead of mounting.
if ! ( cd "$DIST" && ./"$(basename "$OUT")" --appimage-extract-and-run version >/dev/null 2>&1 ); then
  echo "   ✗ the AppImage was written but does not run"; exit 1
fi

rm -rf "$APPDIR" "$TOOL" "$DIST/appimage.log" "$DIST/squashfs-root" 2>/dev/null || true
printf '   %-24s %s  (runs)\n' "$(basename "$OUT")" "$(du -h "$OUT" | cut -f1)"
( cd "$DIST" && sha256sum ./*.AppImage >> SHA256SUMS )
