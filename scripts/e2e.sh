#!/usr/bin/env bash
# Flux complet pe o singură mașină, fără să atingă profilul real: profil sintetic într-un HOME
# temporar → scan → backup → verify → list → restore în ALT HOME → comparare sha256 → a doua
# restaurare, ca să vedem politica de conflict „alături”.
#
#   bash scripts/e2e.sh /tmp/dhs
set -euo pipefail
DHS="${1:-/tmp/dhs}"
BASE=$(mktemp -d /tmp/dhs-e2e.XXXXXX)
SRC="$BASE/sursa"; DST="$BASE/destinatie"; MEDIA="$BASE/ssd"
trap 'echo; echo "(fișierele testului rămân în $BASE)"' EXIT

mkdir -p "$SRC/Documents/sub" "$SRC/Pictures" "$SRC/Downloads" "$DST" "$MEDIA"
head -c 3M /dev/urandom > "$SRC/Pictures/poza.jpg"          # incompresibil
cp "$SRC/Pictures/poza.jpg" "$SRC/Downloads/copie.jpg"       # duplicat
seq 1 300000 > "$SRC/Documents/numere.txt"                   # text, comprimabil
printf 'notă scurtă cu diacritice ăîșțâ\n' > "$SRC/Documents/sub/nota.md"
head -c 5M /dev/urandom > "$SRC/Downloads/mare.bin"          # necunoscut, incompresibil
: > "$SRC/Documents/gol.txt"                                  # gol
echo 'fraza-de-test-pentru-codespaces' > "$BASE/parola"

echo "══ sursa: HOME=$SRC ══"
export HOME="$SRC"; unset XDG_CONFIG_HOME
"$DHS" version
"$DHS" scan --dest "$MEDIA"
"$DHS" backup --dest "$MEDIA" --nume test --parola-fisier "$BASE/parola" --da --verifica
echo; echo "── dhs.json (nu trebuie să conțină nume de fișiere sau user) ──"
cat "$MEDIA/test.dhs/dhs.json"
if grep -qE 'numere|poza|sursa|codespace' "$MEDIA/test.dhs/dhs.json"; then
  echo "EROARE: manifestul scurge nume"; exit 1
fi
echo; ls -l "$MEDIA/test.dhs" "$MEDIA/test.dhs/volume"
if ls "$MEDIA/test.dhs/volume"/*.tmp >/dev/null 2>&1; then echo "EROARE: volum .tmp rămas"; exit 1; fi

echo; echo "══ verify + list ══"
"$DHS" verify "$MEDIA/test.dhs" --parola-fisier "$BASE/parola"
"$DHS" list "$MEDIA/test.dhs" --parola-fisier "$BASE/parola" --tot

echo; echo "══ destinația: HOME=$DST ══"
export HOME="$DST"
"$DHS" restore "$MEDIA/test.dhs" --parola-fisier "$BASE/parola" --dry-run
"$DHS" restore "$MEDIA/test.dhs" --parola-fisier "$BASE/parola" --da

echo; echo "══ comparare ══"
(cd "$SRC" && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort) > "$BASE/sursa.sha"
(cd "$DST" && find Documents Pictures Downloads -type f -exec sha256sum {} + | sort) > "$BASE/dest.sha"
if diff "$BASE/sursa.sha" "$BASE/dest.sha"; then
  echo "E2E OK: $(wc -l < "$BASE/dest.sha") fișiere identice bit cu bit"
else
  echo "EROARE: fișierele restaurate diferă de sursă"; exit 1
fi
if find "$DST" -name '*.dhs-tmp' | grep -q .; then echo "EROARE: temporar .dhs-tmp rămas"; exit 1; fi

echo; echo "══ a doua restaurare: conflict → alături ══"
"$DHS" restore "$MEDIA/test.dhs" --parola-fisier "$BASE/parola" --da
ls -la "$DST/Documents"
test -f "$DST/Documents/numere (DHS).txt" || { echo "EROARE: lipsește copia „ (DHS)”"; exit 1; }
cmp "$DST/Documents/numere.txt" "$DST/Documents/numere (DHS).txt" && echo "copia alături e identică cu originalul"

echo; echo "══ frază greșită → refuz ══"
echo 'alta-fraza-gresita' > "$BASE/parola-gresita"
if "$DHS" verify "$MEDIA/test.dhs" --parola-fisier "$BASE/parola-gresita" 2>&1 | tee /dev/stderr | grep -qi 'greșită'; then
  echo "refuzat corect"
else
  echo "EROARE: fraza greșită nu a fost refuzată"; exit 1
fi

echo; echo "══ pachet necriptat, nivel 1 ══"
export HOME="$SRC"
"$DHS" backup --dest "$MEDIA" --nume clar --nivel 1 --fara-criptare --da
"$DHS" verify "$MEDIA/clar.dhs"
"$DHS" list "$MEDIA/clar.dhs" | head -5

echo; echo "E2E: totul a trecut"
