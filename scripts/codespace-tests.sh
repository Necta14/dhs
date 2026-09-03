#!/usr/bin/env bash
# Runs the checks from docs/TESTING.md on a Codespace and publishes the results on the branch
# tests/<codespace-name>, so they can be read from anywhere with `git fetch` — no gh, no SSH needed.
#
#   cd /workspaces/dhs && git pull && bash scripts/codespace-tests.sh
#
# Never run on the maintainer's machine (AGENTS.md, rule 7).
set -uo pipefail
cd "$(dirname "$0")/.."

# In a `gh codespace ssh` session the Codespace environment is not loaded; the name is still on disk.
if [ -z "${CODESPACE_NAME:-}" ] && [ -r /workspaces/.codespaces/shared/environment-variables.json ]; then
  CODESPACE_NAME="$(sed -n 's/.*"CODESPACE_NAME": *"\([^"]*\)".*/\1/p' \
    /workspaces/.codespaces/shared/environment-variables.json | head -1)"
fi
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
# The results go on the orphan branch tests/<name>, built in a separate worktree: the main checkout
# is never touched and test-results/ stays where it is whatever happens below.
# Credentials: an interactive Codespace shell has GITHUB_TOKEN; a `gh codespace ssh` session does
# not, so pass one in (GH_TOKEN=… or on stdin) or accept that the results stay local.
BRANCH="tests/$NAME"
publish() {
  export GIT_TERMINAL_PROMPT=0
  if [ -z "${GH_TOKEN:-}" ] && [ -z "${GITHUB_TOKEN:-}" ] && [ ! -t 0 ] && read -r -t 1 GH_TOKEN 2>/dev/null; then
    export GH_TOKEN
  fi
  command -v gh >/dev/null 2>&1 && gh auth setup-git >/dev/null 2>&1 || true

  local wt; wt="$(mktemp -d)"; rmdir "$wt"
  git branch -q -D "$BRANCH" >/dev/null 2>&1 || true
  git worktree add -q --detach "$wt" >/dev/null 2>&1 || { echo "Not published: could not create a worktree."; return 1; }
  local rc=0
  (
    set -e
    cd "$wt"
    git checkout -q --orphan "$BRANCH"
    git rm -rfq --cached . >/dev/null 2>&1 || true
    find . -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
    cp -r "$OLDPWD/$OUT" .
    git add -A
    git -c user.name="codespace $NAME" -c user.email="codespace@dhs.local" \
        commit -q -m "tests: $NAME · $(git -C "$OLDPWD" rev-parse --short HEAD) · $FAILS failures"
    git push -q -f -u origin "$BRANCH" 2>"$OLDPWD/$OUT/publish.log"
  ) || rc=$?
  git worktree remove -f "$wt" >/dev/null 2>&1 || rm -rf "$wt"
  git branch -q -D "$BRANCH" >/dev/null 2>&1 || true
  if [ "$rc" -eq 0 ]; then
    echo "Results are on branch $BRANCH:  git fetch origin $BRANCH && git show origin/$BRANCH:SUMMARY.md"
  else
    echo "Not published (no GitHub credentials in this session — see $OUT/publish.log)."
    echo "The results are still in $OUT/. To publish, run from the Codespace terminal, or pass a token:"
    echo "  gh auth token | gh codespace ssh -c <name> -- 'cd /workspaces/dhs && bash scripts/codespace-tests.sh'"
  fi
  return "$rc"
}
publish || true

echo
echo "Done: $FAILS failures. Summary: $OUT/SUMMARY.md"
echo
cat "$OUT/SUMMARY.md"
exit "$FAILS"
