# Testing — the dedicated session on GitHub Codespaces

> Rule (AGENTS.md, 7): tests are **not** run on the maintainer's machine. Neither `go test` nor
> the `dhs` binary on their data. They run on two Codespaces, in a separate session. This document
> is what has to be done there, in order, so that the session is short and complete.
>
> **First green round: 2026-09-03, commit `3a39aef`**, on the 4-core Codespace — everything in
> `scripts/codespace-tests.sh`, including `-race` and the e2e flow. The earlier rounds found and
> fixed: a nil pointer at creation, four data races in index serialisation, a doubled counter, and
> production-strength scrypt that made the suite take 10 minutes.

## Migration between distributions — `scripts/e2e-distro.sh`

Backup on the Ubuntu host, restore inside Docker containers of **Arch, Fedora, Debian, openSUSE
and Alpine (musl)**, sha256 comparison on relative paths. Each container has only the static binary
and the package from the "SSD" — a freshly installed system. **Green 2026-09-03, commit
`6a55e1d`**: five out of five, identical bit for bit. For a single distribution:
`bash scripts/e2e-distro.sh alpine:latest`.

What it does not cover: Windows (Codespaces has no Windows VMs) and localised XDG
(`user-dirs.dirs`) — the containers use the default names.

## The shortest route: from your machine, through `gh`

```bash
GH_TOKEN=$(gh auth token -u <user>) gh codespace ssh -c <codespace> -- \
  'cd /workspaces/dhs && git pull -q && bash scripts/codespace-tests.sh'
```

`gh` needs the `codespace` scope (`gh auth refresh -s codespace`). If `gh` holds several accounts,
pick the one that owns the Codespaces per command, through `GH_TOKEN=$(gh auth token -u <user>)`,
without switching the active account. The tests run in the Codespace; your machine only sends the
command.

The script publishes its results on the branch `tests/<codespace-name>`, in the folder
`test-results/` (`SUMMARY.md` plus one log per step), so they can be read from anywhere with a
plain `git fetch` — no `gh`, no SSH.

## Why two Codespaces

1. **The source** — plays the role of the system you are leaving. `dhs backup` runs here.
2. **The destination** — plays the role of the freshly installed system. `dhs restore` runs here.

The package travels from one to the other as files (a `tar` of the `.dhs` directory, or a shared
mount). That way we test exactly the real flow: two machines, no file in common except the
package.

Codespaces are Linux. For real Windows we need a VM or a Windows runner in CI — until then, on
Windows we only check that it **compiles** (`GOOS=windows go build`).

## Step 0 — preparation, on both

```bash
git clone https://github.com/Necta14/dhs.git && cd dhs
go version                       # ≥ 1.26
gofmt -l . && go vet ./...       # must stay silent
go build -ldflags="-s -w" -o dhs ./cmd/dhs
ls -l dhs                        # under 15 MiB, Rule #4
```

## Step 1 — unit tests (on either)

```bash
go test ./... -count=1 -v 2>&1 | tee /tmp/go-test.log
go test ./... -race -count=1     # the race detector — the compression pipeline is concurrent
```

What must come out green, and what each one proves:

| Test | Proves |
|---|---|
| `TestRoundTripEncrypted` (×3 levels) | write → verify → extract, identical bit for bit, at every level |
| `TestRoundTripUnencrypted` | the unencrypted mode works the same way |
| `TestManifestLeaksNothing` | `dhs.json`, `SHA256SUMS`, `journal.jsonl` contain no file names or user name; `index.dhsi` is encrypted |
| `TestPassphraseErrors` | no passphrase → `ErrNeedPassphrase`; wrong passphrase → `ErrBadPassphrase`; a short passphrase is refused at write time |
| `TestDedup` | the identical file is marked `Dup`, points at the same blocks, is extracted whole |
| `TestLargeFileSpansVolumes` | a file larger than a volume is cut and rebuilt correctly |
| `TestFilterSkipsUnneededVolumes` | selective extraction writes only what was asked for |
| `TestCorruptionIsDetected` | a corrupted byte is caught by `Verify`; on extraction **only** the files in the corrupted block fail, and they never reach `Commit` |
| `TestAbortLeavesIncompletePackage` | after an interruption: incomplete manifest, the in-progress volume left as `.tmp`, nothing that looks valid |
| `TestRefusesNonEmptyDir` | we do not overwrite a directory that has contents |
| `TestSizeMismatchIsRecorded` | a file that grew between scan and backup is recorded at its real size |
| `restore.TestPlanAndExecute` | the plan resolves destinations, a conflict keeps the existing file and writes beside it, non-existent roots go under `DHS-restored`, mode and mtime are restored, no `.dhs-tmp` is left behind |
| `restore.TestPlanConflictPolicies` | `skip` touches nothing; `overwrite` replaces |
| `restore.TestPlanCaseCollisionOnWindows` | `Report.txt` and `report.txt` end up as two files on Windows |
| `restore.TestSanitizeWindows` | forbidden characters, reserved names, trailing dots — adapted |
| `internal/scan`, `internal/system` | classification, exclusions, secrets, inventory, estimate, OS detection |

If anything fails, **do not move on to step 2**. Note it in `docs/NOTES.md` and fix it first.

## Step 2 — the real flow, source → destination

### On the source

```bash
# a test profile, with something from every class
mkdir -p ~/Documents ~/Pictures ~/Downloads
head -c 5M /dev/urandom > ~/Pictures/photo.jpg           # incompressible
cp ~/Pictures/photo.jpg ~/Downloads/copy.jpg             # duplicate
seq 1 2000000 > ~/Documents/numbers.txt                  # text, compressible
head -c 300M /dev/urandom > ~/Downloads/big.bin          # larger than a volume? not at 3.5 GiB —
                                                         # for that, the unit test is what counts

./dhs scan --dest /tmp                                   # the estimate, without writing
echo 'test-passphrase-for-codespaces' > /tmp/passphrase
./dhs backup --dest /tmp --name test --passphrase-file /tmp/passphrase --yes --verify
./dhs verify /tmp/test.dhs --passphrase-file /tmp/passphrase
cat /tmp/test.dhs/dhs.json                               # no file names, no user
cat /tmp/test.dhs/SHA256SUMS
tar -cf /tmp/test.dhs.tar -C /tmp test.dhs
```

Checks by eye: the manifest says `"complete": true`; `volumes/` contains only `.dhsv`, no `.tmp`;
`SHA256SUMS` lists every file in the package.

### Interruption (still on the source)

```bash
head -c 2G /dev/urandom > ~/Downloads/huge.bin
./dhs backup --dest /tmp --name interrupted --passphrase-file /tmp/passphrase --yes &
sleep 3; kill -INT %1; wait
cat /tmp/interrupted.dhs/dhs.json | grep complete        # false
ls /tmp/interrupted.dhs/volumes/                         # finished volumes .dhsv, the one in progress .tmp
./dhs verify /tmp/interrupted.dhs --passphrase-file /tmp/passphrase   # reports "incomplete", does not crash
```

### On the destination

```bash
tar -xf /tmp/test.dhs.tar -C /tmp
./dhs verify /tmp/test.dhs --passphrase-file /tmp/passphrase
./dhs list /tmp/test.dhs --passphrase-file /tmp/passphrase --all
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase --dry-run
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase
sha256sum ~/Pictures/photo.jpg ~/Downloads/copy.jpg ~/Documents/numbers.txt

# conflict: run again — the existing file stays, the restored one appears with " (DHS)"
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase --yes
ls ~/Documents/                                          # numbers.txt and "numbers (DHS).txt"
./dhs restore /tmp/test.dhs --passphrase-file /tmp/passphrase --conflicts skip --dry-run
```

The checksums must be identical to the ones on the source. **That is the test that counts.**

## Step 3 — what gets written down at the end

In `docs/NOTES.md`: the version tested (commit), what passed, what failed, timings (how long the
backup took, on how many GiB, at which level), the binary size. Any problem goes in
`docs/NOTES.md`.
