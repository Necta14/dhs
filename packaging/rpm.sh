#!/usr/bin/env bash
# Builds .rpm packages from the binaries that packaging/build.sh left in dist/build/.
# Prefers a local rpmbuild; if there is none, runs one inside a Fedora container.
#
#   bash packaging/build.sh 0.1.0 && bash packaging/rpm.sh 0.1.0
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-${DHS_VERSION:-$(git describe --tags --always 2>/dev/null || echo 0.0.0)}}"
VERSION="${VERSION#v}"
DIST="dist"

# Go's names on the left, the RPM world's on the right.
rpm_arch() { case "$1" in amd64) echo x86_64 ;; arm64) echo aarch64 ;; *) echo "$1" ;; esac; }

spec() { # <version>
  cat <<EOF
%global debug_package %{nil}
%define _build_id_links none

Name:           dhs
Version:        ${1}
Release:        1%{?dist}
Summary:        Migrate a working environment between operating systems

License:        Apache-2.0
URL:            https://dhs-suite.vercel.app
Source0:        dhs
Source1:        LICENSE
Source2:        NOTICE
Source3:        README.md

%description
DHS builds a portable migration package on an external drive: personal files,
application configurations, a manifest of the installed applications and the
metadata needed to put it all back on a freshly installed system.

The package is encrypted with a passphrase, split into volumes that fit any
filesystem, and verified with SHA-256 on the way in and on the way out. It makes
no network calls, needs no account and contains no AI: applications are matched
across platforms through a curated database, not by guessing.

A single static binary. Nothing runs in the background.

%prep
# nothing to unpack: the binary is already built

%build
# nothing to compile: see packaging/build.sh

%install
install -Dm755 %{SOURCE0} %{buildroot}%{_bindir}/dhs
install -Dm644 %{SOURCE1} %{buildroot}%{_docdir}/dhs/LICENSE
install -Dm644 %{SOURCE2} %{buildroot}%{_docdir}/dhs/NOTICE
install -Dm644 %{SOURCE3} %{buildroot}%{_docdir}/dhs/README.md

%files
%{_bindir}/dhs
%dir %{_docdir}/dhs
%{_docdir}/dhs/LICENSE
%{_docdir}/dhs/NOTICE
%{_docdir}/dhs/README.md

%changelog
* $(LC_ALL=C date '+%a %b %d %Y') Necta <179446771+Necta14@users.noreply.github.com> - ${1}-1
- First packaged release.
EOF
}

# stage <arch> — lays out an rpmbuild tree under a temporary directory and echoes its path
stage() {
  local arch="$1" top
  top="$(mktemp -d)"
  mkdir -p "$top"/{SPECS,SOURCES,RPMS,BUILD,BUILDROOT}
  cp "$DIST/build/linux_${arch}/dhs" "$top/SOURCES/dhs"
  cp LICENSE NOTICE README.md "$top/SOURCES/"
  spec "$VERSION" > "$top/SPECS/dhs.spec"
  echo "$top"
}

echo "── rpm ${VERSION}"
for arch in amd64 arm64; do
  src="$DIST/build/linux_${arch}/dhs"
  [ -f "$src" ] || { echo "missing $src — run packaging/build.sh first"; exit 1; }
  target="$(rpm_arch "$arch")"
  top="$(stage "$arch")"

  if command -v rpmbuild >/dev/null 2>&1; then
    rpmbuild --define "_topdir $top" --target "$target" -bb "$top/SPECS/dhs.spec" >"$top/log" 2>&1 \
      || { echo "rpmbuild failed:"; tail -20 "$top/log"; exit 1; }
  elif command -v docker >/dev/null 2>&1; then
    # The container builds as root, so hand the result back to the invoking user before leaving,
    # otherwise copying it out fails with a permission error.
    docker run --rm -v "$top:/top" fedora:latest bash -lc \
      "dnf install -q -y rpm-build >/dev/null 2>&1 \
       && rpmbuild --define '_topdir /top' --target $target -bb /top/SPECS/dhs.spec \
       && chown -R $(id -u):$(id -g) /top" \
      >"$top/log" 2>&1 || { echo "containerised rpmbuild failed:"; tail -20 "$top/log"; exit 1; }
  else
    echo "neither rpmbuild nor docker is available"; exit 1
  fi

  out="$(find "$top/RPMS" -name '*.rpm' | head -1)"
  cp "$out" "$DIST/"
  printf '   %-24s %s\n' "$(basename "$out")" "$(du -h "$out" | cut -f1)"
  rm -rf "$top"
done

( cd "$DIST" && sha256sum ./*.rpm >> SHA256SUMS )
