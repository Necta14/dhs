#!/usr/bin/env bash
# Rulează verificările din docs/TESTARE.md pe un Codespace și publică rezultatele pe ramura
# teste/<nume-codespace>, ca să poată fi citite de oriunde cu `git fetch` — fără gh, fără SSH.
#
#   cd /workspaces/dhs && git pull && bash scripts/teste-codespace.sh
#
# Nu se rulează pe laptopul userului (AGENTS.md, regula 7).
set -uo pipefail
cd "$(dirname "$0")/.."

NAME="${CODESPACE_NAME:-$(hostname)}"
OUT="rezultate-teste"
FAILS=0
rm -rf "$OUT" && mkdir -p "$OUT"

{
  echo "# Rezultate · $NAME · $(date -u +%FT%TZ)"
  echo
  echo "- commit: $(git rev-parse --short HEAD) · $(git log -1 --format=%s)"
  echo "- go: $(go version | awk '{print $3}')"
  echo "- mașină: $(nproc) nuclee · $(free -h | awk '/Mem:/{print $2}') RAM · $(uname -sr)"
  echo
  echo "| pas | rezultat |"
  echo "|---|---|"
} | tee "$OUT/REZUMAT.md"

# step <nume> <comandă...> — rulează, salvează jurnalul, notează în rezumat.
step() {
  local name="$1"; shift
  local t0=$SECONDS
  echo; echo "══ $name ══"
  if "$@" > "$OUT/$name.log" 2>&1; then
    echo "| $name | ✅ $((SECONDS - t0)) s |" | tee -a "$OUT/REZUMAT.md"
  else
    local code=$?
    FAILS=$((FAILS + 1))
    echo "| $name | ❌ exit $code · $((SECONDS - t0)) s |" | tee -a "$OUT/REZUMAT.md"
  fi
  tail -n 30 "$OUT/$name.log"
}

step gofmt          bash -c 'out=$(gofmt -l .); [ -z "$out" ] || { echo "$out"; exit 1; }'
step vet            go vet ./...
step build-linux    go build -ldflags="-s -w" -o /tmp/dhs ./cmd/dhs
step build-windows  env GOOS=windows go build -ldflags="-s -w" -o /tmp/dhs.exe ./cmd/dhs
step build-arm64    env GOARCH=arm64 go build -o /dev/null ./cmd/dhs
step test           go test ./... -count=1 -v -timeout 10m
step race           go test ./... -race -count=1 -timeout 20m
step cli-version    /tmp/dhs version
step e2e            bash scripts/e2e.sh /tmp/dhs

{
  echo
  echo "## Binare"
  ls -l /tmp/dhs /tmp/dhs.exe 2>/dev/null | awk '{printf "- %s: %.1f MiB\n", $NF, $5/1048576}'
  echo
  echo "## Teste (extras)"
  echo '```'
  grep -E '^(=== RUN|--- (PASS|FAIL|SKIP)|PASS|FAIL|ok|FAIL\s)' "$OUT/test.log" | grep -vE '^=== RUN' | tail -n 80
  echo '```'
  if [ "$FAILS" -gt 0 ]; then
    echo
    echo "## Eșecuri: $FAILS"
    for f in "$OUT"/*.log; do
      n=$(basename "$f" .log)
      if grep -q "| $n | ❌" "$OUT/REZUMAT.md"; then
        echo; echo "### $n"; echo '```'; tail -n 60 "$f"; echo '```'
      fi
    done
  fi
} >> "$OUT/REZUMAT.md"

echo
echo "══ publicare ══"
# Într-o sesiune `gh codespace ssh`, git n-are credențiale; gh le are (GITHUB_TOKEN) și le poate da.
command -v gh >/dev/null 2>&1 && gh auth setup-git >/dev/null 2>&1 || true
BRANCH="teste/$NAME"
git stash -q --include-untracked 2>/dev/null || true
git checkout -q -B "$BRANCH"
git stash pop -q 2>/dev/null || true
git add -f "$OUT"
git -c user.name="codespace $NAME" -c user.email="codespace@dhs.local" \
    commit -q -m "teste: $NAME · $(git rev-parse --short HEAD) · $FAILS eșecuri" && \
git push -q -f -u origin "$BRANCH" && echo "Rezultatele sunt pe ramura $BRANCH"
git checkout -q main
git stash pop -q 2>/dev/null || true

echo
echo "Gata: $FAILS eșecuri. Rezumat: $OUT/REZUMAT.md"
exit "$FAILS"
