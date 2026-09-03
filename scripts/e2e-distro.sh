#!/usr/bin/env bash
# Migration across distributions, with Docker: backup on the host (Ubuntu), restore inside containers
# of Arch, Fedora, Debian, openSUSE and Alpine (musl — the hardest test for a static binary), then
# sha256 comparison on relative paths. Every container is a "freshly installed system": only the dhs
# binary and the package from the "SSD", nothing else.
#
#   bash scripts/e2e-distro.sh              # all distributions
#   bash scripts/e2e-distro.sh archlinux    # just one
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGES=("$@")
[ ${#IMAGES[@]} -gt 0 ] || IMAGES=(archlinux:latest fedora:latest debian:stable-slim opensuse/tumbleweed alpine:latest)

BASE=$(mktemp -d /tmp/dhs-distro.XXXXXX)
SRC="$BASE/source"; MEDIA="$BASE/ssd"
mkdir -p "$SRC/Documents/sub" "$SRC/Pictures" "$SRC/Downloads" "$MEDIA"
trap 'echo; echo "(test files remain in $BASE)"' EXIT

echo "══ static binary (CGO_ENABLED=0) ══"
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BASE/dhs" ./cmd/dhs
file "$BASE/dhs" 2>/dev/null || true
ls -l "$BASE/dhs" | awk '{printf "%.1f MiB\n", $5/1048576}'

echo; echo "══ synthetic profile + backup on the host ══"
head -c 2M /dev/urandom > "$SRC/Pictures/photo.jpg"
cp "$SRC/Pictures/photo.jpg" "$SRC/Downloads/copy.jpg"
seq 1 200000 > "$SRC/Documents/numbers.txt"
printf 'note with diacritics ăîșțâ and “quotes”\n' > "$SRC/Documents/sub/note.md"
printf 'name with spaces and : a colon\n' > "$SRC/Documents/report: final.txt"   # illegal on Windows, fine here
head -c 3M /dev/urandom > "$SRC/Downloads/big.bin"
: > "$SRC/Documents/empty.txt"
echo 'test-passphrase-across-distributions' > "$MEDIA/passphrase"

HOME="$SRC" "$BASE/dhs" version
HOME="$SRC" "$BASE/dhs" backup --dest "$MEDIA" --name test --passphrase-file "$MEDIA/passphrase" --yes --verify | tail -4
(cd "$SRC" && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort) > "$BASE/source.sha"
echo "source: $(wc -l < "$BASE/source.sha") files"

FAILS=0
for IMG in "${IMAGES[@]}"; do
  TAG=$(echo "$IMG" | tr '/:' '__')
  echo; echo "══════════ $IMG ══════════"
  # A failed pull (registry, rate limit) is a failure of that distribution, not of the whole test.
  if ! docker pull -q "$IMG" > "$BASE/$TAG.pull" 2>&1; then
    echo "❌ $IMG — image could not be pulled:"; tail -3 "$BASE/$TAG.pull"; FAILS=$((FAILS+1)); continue
  fi
  # The container is clean: it has only dhs (read-only) and the package (read-only). HOME is /root,
  # mounted from the host so the checksums are computed here — minimal images lack even `find`.
  DEST="$BASE/$TAG-home"; mkdir -p "$DEST"
  if docker run --rm \
      -v "$BASE/dhs:/usr/local/bin/dhs:ro" \
      -v "$MEDIA:/ssd:ro" \
      -v "$DEST:/root" \
      -e HOME=/root \
      "$IMG" sh -c '
        set -e
        dhs version
        dhs restore /ssd/test.dhs --passphrase-file /ssd/passphrase --dry-run | grep -E "Migration|To write|Renamed" || true
        dhs restore /ssd/test.dhs --passphrase-file /ssd/passphrase --yes | tail -2
      ' > "$BASE/$TAG.out" 2>&1
  then
    sudo chown -R "$(id -u):$(id -g)" "$DEST" 2>/dev/null || true   # written as root inside the container
    (cd "$DEST" && find Documents Pictures Downloads -type f -exec sha256sum {} + 2>/dev/null | sort) > "$BASE/$TAG.sha" || true
    if diff "$BASE/source.sha" "$BASE/$TAG.sha" > "$BASE/$TAG.diff"; then
      echo "✅ $IMG — $(wc -l < "$BASE/$TAG.sha") files identical bit for bit"
      grep -E "^system:" "$BASE/$TAG.out" | head -1
    else
      echo "❌ $IMG — files differ:"; cat "$BASE/$TAG.diff"; FAILS=$((FAILS+1))
    fi
  else
    echo "❌ $IMG — restore failed:"; tail -20 "$BASE/$TAG.out"; FAILS=$((FAILS+1))
  fi
done

echo
if [ "$FAILS" -eq 0 ]; then
  echo "DISTRO E2E: all distributions passed (${#IMAGES[@]})"
else
  echo "DISTRO E2E: $FAILS distributions failed"; exit 1
fi
