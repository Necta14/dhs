#!/usr/bin/env bash
# Full flow on a single machine, without touching the real profile: synthetic profile in a temporary
# HOME → scan → backup → verify → list → restore into ANOTHER HOME → sha256 comparison → a second
# restore, to see the "keep-both" conflict policy → wrong passphrase refused → unencrypted level 1.
#
#   bash scripts/e2e.sh /tmp/dhs
set -euo pipefail
DHS="${1:-/tmp/dhs}"
BASE=$(mktemp -d /tmp/dhs-e2e.XXXXXX)
SRC="$BASE/source"; DST="$BASE/destination"; MEDIA="$BASE/ssd"
trap 'echo; echo "(test files remain in $BASE)"' EXIT

mkdir -p "$SRC/Documents/sub" "$SRC/Pictures" "$SRC/Downloads" "$DST" "$MEDIA"
head -c 3M /dev/urandom > "$SRC/Pictures/photo.jpg"          # incompressible
cp "$SRC/Pictures/photo.jpg" "$SRC/Downloads/copy.jpg"        # duplicate
seq 1 300000 > "$SRC/Documents/numbers.txt"                   # text, compressible
printf 'short note with diacritics ăîșțâ\n' > "$SRC/Documents/sub/note.md"
head -c 5M /dev/urandom > "$SRC/Downloads/big.bin"            # unknown class, incompressible
: > "$SRC/Documents/empty.txt"                                 # empty
echo 'test-passphrase-for-codespaces' > "$BASE/passphrase"

echo "══ source: HOME=$SRC ══"
export HOME="$SRC"; unset XDG_CONFIG_HOME
"$DHS" version
"$DHS" scan --dest "$MEDIA"
"$DHS" backup --dest "$MEDIA" --name test --passphrase-file "$BASE/passphrase" --yes --verify
echo; echo "── dhs.json (must not contain file names or the user) ──"
cat "$MEDIA/test.dhs/dhs.json"
# Look for real sensitive values (file names, the HOME path, the user), not words from the JSON schema.
for leak in numbers photo note "big.bin" "$SRC" "$(id -un)"; do
  if grep -qF -- "$leak" "$MEDIA/test.dhs/dhs.json" "$MEDIA/test.dhs/SHA256SUMS" "$MEDIA/test.dhs/journal.jsonl"; then
    echo "ERROR: unencrypted files contain \"$leak\""; exit 1
  fi
done
echo; ls -l "$MEDIA/test.dhs" "$MEDIA/test.dhs/volumes"
if ls "$MEDIA/test.dhs/volumes"/*.tmp >/dev/null 2>&1; then echo "ERROR: a .tmp volume was left behind"; exit 1; fi
# With a 3 MiB duplicate and 2 MiB of compressible text, the package must be smaller than the source.
RAW=$(du -sb "$SRC" | cut -f1); PKG=$(du -sb "$MEDIA/test.dhs" | cut -f1)
echo "source: $RAW bytes · package on disk: $PKG bytes"
if [ "$PKG" -ge "$RAW" ]; then echo "ERROR: the package is larger than the source — dedup or compression is broken"; exit 1; fi

echo; echo "══ verify + list ══"
"$DHS" verify "$MEDIA/test.dhs" --passphrase-file "$BASE/passphrase"
"$DHS" list "$MEDIA/test.dhs" --passphrase-file "$BASE/passphrase" --all

echo; echo "══ destination: HOME=$DST ══"
export HOME="$DST"
"$DHS" restore "$MEDIA/test.dhs" --passphrase-file "$BASE/passphrase" --dry-run
"$DHS" restore "$MEDIA/test.dhs" --passphrase-file "$BASE/passphrase" --yes

echo; echo "══ comparison ══"
(cd "$SRC" && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort) > "$BASE/source.sha"
(cd "$DST" && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort) > "$BASE/dest.sha"
if diff "$BASE/source.sha" "$BASE/dest.sha"; then
  echo "E2E OK: $(wc -l < "$BASE/dest.sha") files identical bit for bit"
else
  echo "ERROR: restored files differ from the source"; exit 1
fi
if find "$DST" -name '*.dhs-tmp' | grep -q .; then echo "ERROR: a .dhs-tmp temporary was left behind"; exit 1; fi

echo; echo "══ second restore: conflict → keep both ══"
"$DHS" restore "$MEDIA/test.dhs" --passphrase-file "$BASE/passphrase" --yes
ls -la "$DST/Documents"
test -f "$DST/Documents/numbers (DHS).txt" || { echo "ERROR: the \" (DHS)\" copy is missing"; exit 1; }
cmp "$DST/Documents/numbers.txt" "$DST/Documents/numbers (DHS).txt" && echo "the copy beside is identical to the original"

echo; echo "══ wrong passphrase → refused ══"
echo 'another-wrong-passphrase' > "$BASE/wrong-passphrase"
# dhs MUST exit with an error here; under pipefail a pipeline would report dhs's (correct) error as a test failure.
set +e
OUT=$("$DHS" verify "$MEDIA/test.dhs" --passphrase-file "$BASE/wrong-passphrase" 2>&1)
CODE=$?
set -e
echo "$OUT"
if [ "$CODE" -ne 0 ] && echo "$OUT" | grep -qi 'wrong passphrase'; then
  echo "refused correctly (exit $CODE)"
else
  echo "ERROR: the wrong passphrase was not refused (exit $CODE)"; exit 1
fi

echo; echo "══ unencrypted package, level 1 ══"
export HOME="$SRC"
"$DHS" backup --dest "$MEDIA" --name plain --level 1 --no-encrypt --yes
"$DHS" verify "$MEDIA/plain.dhs"
"$DHS" list "$MEDIA/plain.dhs" | head -5

echo; echo "E2E: everything passed"
