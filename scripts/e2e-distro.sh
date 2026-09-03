#!/usr/bin/env bash
# Migrare între distribuții, cu Docker: backup pe gazdă (Ubuntu), restaurare în containere de Arch,
# Fedora, Debian, openSUSE și Alpine (musl — testul cel mai dur pentru un binar static), apoi
# comparare sha256 pe căi relative. Fiecare container e un „sistem proaspăt instalat": doar
# binarul dhs și pachetul de pe „SSD", nimic altceva.
#
#   bash scripts/e2e-distro.sh              # toate distribuțiile
#   bash scripts/e2e-distro.sh archlinux    # doar una
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGES=("$@")
[ ${#IMAGES[@]} -gt 0 ] || IMAGES=(archlinux:latest fedora:latest debian:stable-slim opensuse/tumbleweed alpine:latest)

BASE=$(mktemp -d /tmp/dhs-distro.XXXXXX)
SRC="$BASE/sursa"; MEDIA="$BASE/ssd"
mkdir -p "$SRC/Documents/sub" "$SRC/Pictures" "$SRC/Downloads" "$MEDIA"
trap 'echo; echo "(fișierele testului rămân în $BASE)"' EXIT

echo "══ binar static (CGO_ENABLED=0) ══"
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BASE/dhs" ./cmd/dhs
file "$BASE/dhs" 2>/dev/null || true
ls -l "$BASE/dhs" | awk '{printf "%.1f MiB\n", $5/1048576}'

echo; echo "══ profil sintetic + backup pe gazdă ══"
head -c 2M /dev/urandom > "$SRC/Pictures/poza.jpg"
cp "$SRC/Pictures/poza.jpg" "$SRC/Downloads/copie.jpg"
seq 1 200000 > "$SRC/Documents/numere.txt"
printf 'notă cu diacritice ăîșțâ și „ghilimele”\n' > "$SRC/Documents/sub/nota.md"
printf 'nume cu spații și : două puncte\n' > "$SRC/Documents/raport: final.txt"   # ilegal pe Windows, legal aici
head -c 3M /dev/urandom > "$SRC/Downloads/mare.bin"
: > "$SRC/Documents/gol.txt"
echo 'fraza-de-test-intre-distributii' > "$MEDIA/parola"

HOME="$SRC" "$BASE/dhs" version
HOME="$SRC" "$BASE/dhs" backup --dest "$MEDIA" --nume test --parola-fisier "$MEDIA/parola" --da --verifica | tail -4
(cd "$SRC" && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort) > "$BASE/sursa.sha"
echo "sursă: $(wc -l < "$BASE/sursa.sha") fișiere"

FAILS=0
for IMG in "${IMAGES[@]}"; do
  TAG=$(echo "$IMG" | tr '/:' '__')
  echo; echo "══════════ $IMG ══════════"
  docker pull -q "$IMG" >/dev/null
  # Containerul e curat: are doar dhs (read-only) și pachetul (read-only). HOME e /root.
  if docker run --rm \
      -v "$BASE/dhs:/usr/local/bin/dhs:ro" \
      -v "$MEDIA:/ssd:ro" \
      -e HOME=/root \
      "$IMG" sh -c '
        set -e
        dhs version
        dhs restore /ssd/test.dhs --parola-fisier /ssd/parola --dry-run | grep -E "Migrare|De scris|Redenumite" || true
        dhs restore /ssd/test.dhs --parola-fisier /ssd/parola --da | tail -2
        cd /root && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort
      ' > "$BASE/$TAG.out" 2>&1
  then
    grep -E "^[0-9a-f]{64}  " "$BASE/$TAG.out" > "$BASE/$TAG.sha"
    if diff "$BASE/sursa.sha" "$BASE/$TAG.sha" > "$BASE/$TAG.diff"; then
      echo "✅ $IMG — $(wc -l < "$BASE/$TAG.sha") fișiere identice bit cu bit"
      grep -E "^sistem:" "$BASE/$TAG.out" | head -1
    else
      echo "❌ $IMG — fișierele diferă:"; cat "$BASE/$TAG.diff"; FAILS=$((FAILS+1))
    fi
  else
    echo "❌ $IMG — restaurarea a eșuat:"; tail -20 "$BASE/$TAG.out"; FAILS=$((FAILS+1))
  fi
done

echo
if [ "$FAILS" -eq 0 ]; then
  echo "DISTRO E2E: toate distribuțiile au trecut (${#IMAGES[@]})"
else
  echo "DISTRO E2E: $FAILS distribuții au picat"; exit 1
fi
