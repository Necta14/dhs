package pack

// The tests here are NOT run on the maintainer's laptop, by project rule. They are written for the
// dedicated session on GitHub Codespaces. All of them write only into t.TempDir().

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Necta14/dhs/internal/passphrase"
	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

// scrypt with the production work factor (2^18) costs ~1 s and 256 MiB per volume; under the race
// detector, 20 times as much. The suite opens dozens of small volumes, so we lower the factor to
// 2^10 — here we test the format, not the strength of the passphrase. The product never changes it.
func init() { passphrase.WorkFactor = 10 }

const testPass = "a-test-passphrase-that-is-long-enough"

// Small sizes, to force many blocks and many volumes out of little data.
const (
	tBlock  = 64 << 10
	tVolume = 200 << 10
)

type fixture struct {
	name  string
	root  Root
	class scan.Class
	data  []byte
	dup   bool
}

func text(n int) []byte {
	line := []byte("this line repeats so that it compresses well, with non-ASCII characters: äöü ñ é\n")
	out := make([]byte, 0, n+len(line))
	for len(out) < n {
		out = append(out, line...)
	}
	return out[:n]
}

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func fixtures() []fixture {
	rnd := random(300 << 10) // 300 KiB: larger than a block, larger than a test volume
	return []fixture{
		{name: "documents/report.txt", root: "documents", class: scan.Text, data: text(150 << 10)},
		{name: "documents/short.md", root: "documents", class: scan.Text, data: text(900)},
		{name: "pictures/photo.jpg", root: "pictures", class: scan.Incompressible, data: rnd},
		{name: "pictures/copy.jpg", root: "pictures", class: scan.Incompressible, data: rnd, dup: true},
		{name: "config/app/set.bin", root: "config", class: scan.Binary, data: append(text(40<<10), random(20<<10)...)},
		{name: "downloads/unknown-text", root: "downloads", class: scan.Unknown, data: text(70 << 10)},
		{name: "downloads/unknown-bin", root: "downloads", class: scan.Unknown, data: random(70 << 10)},
		{name: "documents/empty.txt", root: "documents", class: scan.Text, data: nil},
		{name: "home/x/other.log", root: RootOther, class: scan.Text, data: text(5 << 10)},
	}
}

func writePackage(t *testing.T, dir, pass string, level scan.Level, fx []fixture) *Writer {
	t.Helper()
	w, err := NewWriter(Options{
		Dir:        dir,
		Level:      level,
		Passphrase: pass,
		Source:     system.Info{OS: system.Linux, Name: "Test Linux", Arch: "amd64", User: "SECRET-USER", Hostname: "SECRET-HOST"},
		VolumeSize: tVolume,
		BlockSize:  tBlock,
		Workers:    3,
		Tool:       "dhs test",
		Now:        func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fx {
		f := f
		err := w.Add(context.Background(), File{
			Root: f.root, Path: filepath.ToSlash(f.name), Orig: "/orig/" + f.name,
			Size: int64(len(f.data)), Mode: 0o644, ModTime: time.Unix(1_700_000_000, 0),
			Class: f.class, MaybeDup: f.dup,
			Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(f.data)), nil },
		})
		if err != nil {
			t.Fatalf("Add %s: %v", f.name, err)
		}
	}
	return w
}

// memSink is an in-memory destination, so that the tests do not depend on the disk.
type memSink struct {
	buf       bytes.Buffer
	committed bool
	aborted   bool
}

func (m *memSink) Write(p []byte) (int, error) { return m.buf.Write(p) }
func (m *memSink) Commit() error               { m.committed = true; return nil }
func (m *memSink) Abort() error                { m.aborted = true; return nil }

func extractAll(t *testing.T, r *Reader, filter func(*Entry) bool) (map[string]*memSink, *ExtractReport) {
	t.Helper()
	sinks := map[string]*memSink{}
	rep, err := r.Extract(context.Background(), ExtractOptions{
		Filter: filter,
		Open: func(e *Entry) (Sink, error) {
			s := &memSink{}
			sinks[e.Path] = s
			return s, nil
		},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return sinks, rep
}

func TestRoundTripEncrypted(t *testing.T) {
	for _, level := range []scan.Level{scan.LevelCompatible, scan.LevelBalanced, scan.LevelMax} {
		t.Run(level.String(), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "p.dhs")
			fx := fixtures()
			w := writePackage(t, dir, testPass, level, fx)
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			r, err := Open(dir, testPass)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if !r.Complete() {
				t.Error("package must be marked complete")
			}
			if r.Manifest.Volumes < 2 {
				t.Errorf("want >=2 volumes with a small VolumeSize, got %d", r.Manifest.Volumes)
			}
			if r.Manifest.Files != int64(len(fx)) {
				t.Errorf("files in manifest = %d, want %d", r.Manifest.Files, len(fx))
			}
			if r.Manifest.Cipher != CipherAgeScrypt.String() || r.Manifest.Level != level {
				t.Errorf("manifest: cipher=%s level=%v", r.Manifest.Cipher, r.Manifest.Level)
			}

			rep, err := r.Verify(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if !rep.OK() {
				t.Fatalf("Verify: %v", rep.Problems)
			}

			sinks, xrep := extractAll(t, r, nil)
			if len(xrep.Failed) != 0 {
				t.Fatalf("failed: %+v", xrep.Failed)
			}
			if xrep.Files != len(fx) {
				t.Errorf("extracted %d, want %d", xrep.Files, len(fx))
			}
			for _, f := range fx {
				s := sinks[filepath.ToSlash(f.name)]
				if s == nil {
					t.Errorf("%s: was not extracted", f.name)
					continue
				}
				if !s.committed || s.aborted {
					t.Errorf("%s: committed=%v aborted=%v", f.name, s.committed, s.aborted)
				}
				if !bytes.Equal(s.buf.Bytes(), f.data) {
					t.Errorf("%s: content differs (%d vs %d bytes)", f.name, s.buf.Len(), len(f.data))
				}
			}
		})
	}
}

func TestRoundTripUnencrypted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	fx := fixtures()
	w := writePackage(t, dir, "", scan.LevelBalanced, fx)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Manifest.Cipher != CipherNone.String() {
		t.Errorf("cipher = %s, want none", r.Manifest.Cipher)
	}
	sinks, rep := extractAll(t, r, nil)
	if len(rep.Failed) != 0 || len(sinks) != len(fx) {
		t.Fatalf("failed %d, extracted %d", len(rep.Failed), len(sinks))
	}
}

func TestManifestLeaksNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fixtures())
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{manifestName, sumsName, journalName} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"SECRET-USER", "SECRET-HOST", "report.txt", "photo.jpg", "/orig/"} {
			if bytes.Contains(b, []byte(secret)) {
				t.Errorf("%s contains %q — the unencrypted files must not reveal what is in the package", name, secret)
			}
		}
	}
	// index.dhsi is encrypted: the names do not appear in plaintext.
	b, err := os.ReadFile(filepath.Join(dir, indexName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("report.txt")) {
		t.Error("index.dhsi contains a file name in plaintext")
	}
}

func TestPassphraseErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fixtures()[:2])
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, ""); !errors.Is(err, ErrNeedPassphrase) {
		t.Errorf("without passphrase: %v, want ErrNeedPassphrase", err)
	}
	if _, err := Open(dir, "wrong-but-long-enough"); !errors.Is(err, ErrBadPassphrase) {
		t.Errorf("wrong passphrase: %v, want ErrBadPassphrase", err)
	}
	if _, err := NewWriter(Options{Dir: filepath.Join(t.TempDir(), "x"), Level: scan.LevelBalanced, Passphrase: "short"}); err == nil {
		t.Error("a 5-character passphrase must be refused when writing")
	}
}

func TestDedup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	fx := fixtures()
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fx)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, testPass)
	if err != nil {
		t.Fatal(err)
	}
	var orig, dup *Entry
	for _, e := range r.Index.Entries {
		switch e.Path {
		case "pictures/photo.jpg":
			orig = e
		case "pictures/copy.jpg":
			dup = e
		}
	}
	if orig == nil || dup == nil {
		t.Fatal("entries are missing")
	}
	if !dup.Dup || orig.Dup {
		t.Errorf("the copy must be marked Dup, the original not: %v / %v", dup.Dup, orig.Dup)
	}
	if len(dup.Parts) != len(orig.Parts) {
		t.Fatalf("the copy has %d parts, the original %d", len(dup.Parts), len(orig.Parts))
	}
	for i := range dup.Parts {
		if dup.Parts[i] != orig.Parts[i] {
			t.Errorf("part %d differs: %+v vs %+v", i, dup.Parts[i], orig.Parts[i])
		}
	}
	if len(orig.Parts) < 2 {
		t.Errorf("300 KiB in 64 KiB blocks must yield >=2 parts, got %d", len(orig.Parts))
	}
	// Both are extracted whole, although the data sits on disk only once.
	sinks, rep := extractAll(t, r, func(e *Entry) bool { return e.Root == "pictures" })
	if len(rep.Failed) != 0 {
		t.Fatalf("%+v", rep.Failed)
	}
	if !bytes.Equal(sinks["pictures/photo.jpg"].buf.Bytes(), sinks["pictures/copy.jpg"].buf.Bytes()) {
		t.Error("the original and the copy differ after extraction")
	}
}

func TestLargeFileSpansVolumes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	big := random(700 << 10) // 3.5 test volumes
	w := writePackage(t, dir, testPass, scan.LevelBalanced, []fixture{
		{name: "pictures/big.jpg", root: "pictures", class: scan.Incompressible, data: big},
	})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, testPass)
	if err != nil {
		t.Fatal(err)
	}
	if r.Manifest.Volumes < 3 {
		t.Errorf("700 KiB in 200 KiB volumes must yield >=3 volumes, got %d", r.Manifest.Volumes)
	}
	e := r.Index.Entries[0]
	volumes := map[uint32]bool{}
	for _, p := range e.Parts {
		volumes[r.Index.Blocks[p.Block].Volume] = true
	}
	if len(volumes) < 2 {
		t.Errorf("the file must span >=2 volumes, it sits in %d", len(volumes))
	}
	sinks, rep := extractAll(t, r, nil)
	if len(rep.Failed) != 0 {
		t.Fatalf("%+v", rep.Failed)
	}
	if !bytes.Equal(sinks["pictures/big.jpg"].buf.Bytes(), big) {
		t.Error("the file spanning volumes was not rebuilt identically")
	}
}

func TestFilterSkipsUnneededVolumes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fixtures())
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, testPass)
	if err != nil {
		t.Fatal(err)
	}
	sinks, rep := extractAll(t, r, func(e *Entry) bool { return e.Path == "documents/short.md" })
	if len(sinks) != 1 || rep.Files != 1 || len(rep.Failed) != 0 {
		t.Fatalf("filter: %d destinations, %d files, %d failed", len(sinks), rep.Files, len(rep.Failed))
	}
	if !bytes.Equal(sinks["documents/short.md"].buf.Bytes(), text(900)) {
		t.Error("content differs")
	}
}

func TestCorruptionIsDetected(t *testing.T) {
	// Without encryption, so that we can damage one byte inside a block and see that only the
	// files in that block fail. With age, any changed byte invalidates the whole chunk — and that
	// is fine.
	dir := filepath.Join(t.TempDir(), "p.dhs")
	fx := fixtures()
	w := writePackage(t, dir, "", scan.LevelBalanced, fx)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	// Damage one byte of the payload of the first block of the first volume.
	first := r.Index.Blocks[0]
	path := volumeFile(dir, first.Volume)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	at := first.Pos + blockHeaderSize + first.Stored/2
	var b [1]byte
	if _, err := f.ReadAt(b[:], at); err != nil {
		t.Fatal(err)
	}
	b[0] ^= 0xFF
	if _, err := f.WriteAt(b[:], at); err != nil {
		t.Fatal(err)
	}
	f.Close()

	rep, err := r.Verify(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() {
		t.Fatal("Verify must catch the damaged byte")
	}
	found := false
	for _, p := range rep.Problems {
		if bytes.Contains([]byte(p), []byte(volumeName(first.Volume))) || bytes.Contains([]byte(p), []byte("volume")) {
			found = true
		}
	}
	if !found {
		t.Errorf("the problems do not point at the volume: %v", rep.Problems)
	}

	sinks, xrep := extractAll(t, r, nil)
	if len(xrep.Failed) == 0 {
		t.Fatal("Extract must report the files in the damaged block")
	}
	for _, fl := range xrep.Failed {
		if s := sinks[fl.Entry.Path]; s != nil && s.committed {
			t.Errorf("%s failed but reached Commit", fl.Entry.Path)
		}
	}
	if xrep.Files == 0 {
		t.Error("the files in the intact blocks must pass")
	}
}

func TestAbortLeavesIncompletePackage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fixtures())
	w.Abort(errors.New("test: cable yanked"))

	m, err := Peek(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Complete {
		t.Error("the manifest of an aborted package must not say complete")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, volumeDir))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".dhsv" {
			// A finished volume is valid; only the one in progress may remain .tmp.
			continue
		}
		if filepath.Ext(e.Name()) != ".tmp" {
			t.Errorf("unexpected file in volumes/: %s", e.Name())
		}
	}
	if err := w.Close(); err == nil {
		t.Error("Close after Abort must return the error")
	}
}

func TestRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "something"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(Options{Dir: dir, Level: scan.LevelBalanced}); err == nil {
		t.Error("a directory with content must be refused — we overwrite nothing")
	}
}

func TestDuplicateEntryRefused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	fx := fixtures()[:1]
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fx)
	f := fx[0]
	err := w.Add(context.Background(), File{
		Root: f.root, Path: f.name, Size: 1, Class: scan.Text,
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("x"))), nil },
	})
	if err == nil {
		t.Error("the same path twice must be refused")
	}
	w.Abort(nil)
}

func TestSizeMismatchIsRecorded(t *testing.T) {
	// The file grew between scan and backup: we record how much we read, not how much we thought.
	dir := filepath.Join(t.TempDir(), "p.dhs")
	data := text(10 << 10)
	w := writePackage(t, dir, testPass, scan.LevelBalanced, nil)
	err := w.Add(context.Background(), File{
		Root: "documents", Path: "grown.txt", Size: 100, Class: scan.Text,
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, testPass)
	if err != nil {
		t.Fatal(err)
	}
	if r.Index.Entries[0].Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d (what was actually read)", r.Index.Entries[0].Size, len(data))
	}
}

// A duplicate of a file whose bytes are still sitting in an open solid block used to be written
// with no parts at all: the dedup branch copied the original's parts before the block was flushed,
// and every part that arrived afterwards reached the original alone. Restoring such a file failed
// with "empty file with a nonzero hash", so the copy was lost from the package while the rest of
// it verified perfectly.
//
// The fixtures above never caught it, because their duplicate is Incompressible and stored
// directly, receiving its part immediately. Anything compressible goes through a solid block.
// Found by running the Windows binary under Wine, where two identical binary-class files took
// exactly this path.
func TestDuplicateInSolidBlockKeepsItsParts(t *testing.T) {
	body := text(120 << 10)
	fx := []fixture{
		{name: "documents/original.txt", root: "documents", class: scan.Text, data: body},
		{name: "documents/copy.txt", root: "documents", class: scan.Text, data: body, dup: true},
		{name: "documents/filler.txt", root: "documents", class: scan.Text, data: text(3 << 10)},
		{name: "config/app.bin", root: "config", class: scan.Binary, data: append(text(30<<10), random(10<<10)...)},
		{name: "config/app-copy.bin", root: "config", class: scan.Binary, data: append(text(30<<10), random(10<<10)...), dup: true},
	}
	// The two config files must be byte-identical for the second to deduplicate at all.
	fx[4].data = fx[3].data

	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fx)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := Open(dir, testPass)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	dups := 0
	for _, e := range r.Index.Entries {
		if !e.Dup {
			continue
		}
		dups++
		if len(e.Parts) == 0 {
			t.Errorf("%s: duplicate entry carries no parts, so it cannot be restored", e.Path)
		}
	}
	if dups != 2 {
		t.Fatalf("entries marked as duplicates = %d, want 2", dups)
	}

	rep, err := r.Verify(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("Verify: %v", rep.Problems)
	}

	sinks, xrep := extractAll(t, r, nil)
	if len(xrep.Failed) != 0 {
		t.Fatalf("failed to restore: %+v", xrep.Failed)
	}
	if xrep.Files != len(fx) {
		t.Errorf("restored %d files, want %d", xrep.Files, len(fx))
	}
	for _, f := range fx {
		s := sinks[filepath.ToSlash(f.name)]
		if s == nil {
			t.Errorf("%s: was not restored", f.name)
			continue
		}
		if !bytes.Equal(s.buf.Bytes(), f.data) {
			t.Errorf("%s: content differs (%d bytes restored, %d expected)", f.name, s.buf.Len(), len(f.data))
		}
	}
}
