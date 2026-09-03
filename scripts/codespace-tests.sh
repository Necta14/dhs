#!/usr/bin/env bash
# Runs the checks from docs/TESTING.md on a Codespace and publishes the results on the branch
# tests/<codespace-name>, so they can be read from anywhere with `git fetch` — no gh, no SSH needed.
#
#   cd /workspaces/dhs && git pull && bash scripts/codespace-tests.sh
#
# Never run on the maintainer's machine (AGENTS.md, rule 7).
set -uo pipefail
cd "$(dirname "$0")/.."

NAME="${CODESPACE_NAME:-$(hostname)}"
OUT="test-results"
FAILS=0
rm -rf "$OUT" && mkdir -p "$OUT"

{
  echo "# Results · $NAME · $(date -u +%FT%TZ)"
  echo
  echo "- commit: $(git rev-parse --short HEAD) · $(git log -1 --format=%s)"
  echo "- go: $(go version | awk '{print $3}')"
  echo "- machine: $(nproc) cores · $(free -h | awk '/Mem:/{print $2}') RAM · $(uname -sr)"
  echo
  echo "| step | result |"
  echo "|---|---|"
} | tee "$OUT/SUMMARY.md"

# step <name> <command...> — runs it, keeps the log, records the outcome in the summary.
step() {
  local name="$1"; shift
  local t0=$SECONDS
  echo; echo "══ $name ══"
  if "$@" > "$OUT/$name.log" 2>&1; then
    echo "| $name | ✅ $((SECONDS - t0)) s |" | tee -a "$OUT/SUMMARY.md"
  else
    local code=$?
    FAILS=$((FAILS + 1))
    echo "| $name | ❌ exit $code · $((SECONDS - t0)) s |" | tee -a "$OUT/SUMMARY.md"
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
  echo "## Binaries"
  ls -l /tmp/dhs /tmp/dhs.exe 2>/dev/null | awk '{printf "- %s: %.1f MiB\n", $NF, $5/1048576}'
  echo
  echo "## Tests (excerpt)"
  echo '```'
  grep -E '^(=== RUN|--- (PASS|FAIL|SKIP)|PASS|FAIL|ok|FAIL\s)' "$OUT/test.log" | grep -vE '^=== RUN' | tail -n 80
  echo '```'
  if [ "$FAILS" -gt 0 ]; then
    echo
    echo "## Failures: $FAILS"
    for f in "$OUT"/*.log; do
      n=$(basename "$f" .log)
      if grep -q "| $n | ❌" "$OUT/SUMMARY.md"; then
        echo; echo "### $n"; echo '```'; tail -n 60 "$f"; echo '```'
      fi
    done
  fi
} >> "$OUT/SUMMARY.md"

echo
echo "══ publish ══"
# In a `gh codespace ssh` session git has no credentials; gh has them (GITHUB_TOKEN) and can lend them.
command -v gh >/dev/null 2>&1 && gh auth setup-git >/dev/null 2>&1 || true
BRANCH="tests/$NAME"
git stash -q --include-untracked 2>/dev/null || true
git checkout -q -B "$BRANCH"
git stash pop -q 2>/dev/null || true
git add -f "$OUT"
git -c user.name="codespace $NAME" -c user.email="codespace@dhs.local" \
    commit -q -m "tests: $NAME · $(git rev-parse --short HEAD) · $FAILS failures" && \
git push -q -f -u origin "$BRANCH" && echo "Results are on branch $BRANCH"
git checkout -q main
git stash pop -q 2>/dev/null || true

echo
echo "Done: $FAILS failures. Summary: $OUT/SUMMARY.md"
exit "$FAILS"
