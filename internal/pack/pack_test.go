package pack

// Testele de aici NU se rulează pe laptopul userului (AGENTS.md, regula 7). Sunt scrise pentru
// sesiunea dedicată de pe GitHub Codespaces. Toate scriu doar în t.TempDir().

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

// scrypt cu factorul de producție (2^18) costă ~1 s și 256 MiB la fiecare volum; sub detectorul
// de curse, de 20× mai mult. Suita deschide zeci de volume mici, deci coborâm factorul la 2^10 —
// aici testăm formatul, nu rezistența parolei. Produsul nu-l schimbă niciodată.
func init() { passphrase.WorkFactor = 10 }

const testPass = "o-fraza-de-test-suficient-de-lunga"

// Dimensiuni mici, ca să forțăm multe blocuri și multe volume cu date puține.
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
	line := []byte("linia asta se repetă ca să fie comprimabilă, cu diacritice: ăîșțâ\n")
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
	rnd := random(300 << 10) // 300 KiB: mai mare decât un bloc, mai mare decât un volum de test
	return []fixture{
		{name: "documente/raport.txt", root: "documente", class: scan.Text, data: text(150 << 10)},
		{name: "documente/scurt.md", root: "documente", class: scan.Text, data: text(900)},
		{name: "imagini/poza.jpg", root: "imagini", class: scan.Incompressible, data: rnd},
		{name: "imagini/copie.jpg", root: "imagini", class: scan.Incompressible, data: rnd, dup: true},
		{name: "config/app/set.bin", root: "config", class: scan.Binary, data: append(text(40<<10), random(20<<10)...)},
		{name: "descarcari/necunoscut-text", root: "descarcari", class: scan.Unknown, data: text(70 << 10)},
		{name: "descarcari/necunoscut-bin", root: "descarcari", class: scan.Unknown, data: random(70 << 10)},
		{name: "documente/gol.txt", root: "documente", class: scan.Text, data: nil},
		{name: "home/x/altceva.log", root: RootOther, class: scan.Text, data: text(5 << 10)},
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

// memSink e o destinație în memorie, ca testele să nu depindă de disc.
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
				t.Error("pachetul trebuie marcat complet")
			}
			if r.Manifest.Volumes < 2 {
				t.Errorf("aștept ≥2 volume cu VolumeSize mic, am %d", r.Manifest.Volumes)
			}
			if r.Manifest.Files != int64(len(fx)) {
				t.Errorf("fișiere în manifest = %d, aștept %d", r.Manifest.Files, len(fx))
			}
			if r.Manifest.Cipher != CipherAgeScrypt.String() || r.Manifest.Level != level {
				t.Errorf("manifest: cifru=%s nivel=%v", r.Manifest.Cipher, r.Manifest.Level)
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
				t.Fatalf("eșuate: %+v", xrep.Failed)
			}
			if xrep.Files != len(fx) {
				t.Errorf("extrase %d, aștept %d", xrep.Files, len(fx))
			}
			for _, f := range fx {
				s := sinks[filepath.ToSlash(f.name)]
				if s == nil {
					t.Errorf("%s: nu a fost extras", f.name)
					continue
				}
				if !s.committed || s.aborted {
					t.Errorf("%s: committed=%v aborted=%v", f.name, s.committed, s.aborted)
				}
				if !bytes.Equal(s.buf.Bytes(), f.data) {
					t.Errorf("%s: conținut diferit (%d vs %d octeți)", f.name, s.buf.Len(), len(f.data))
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
		t.Errorf("cifru = %s, aștept niciunul", r.Manifest.Cipher)
	}
	sinks, rep := extractAll(t, r, nil)
	if len(rep.Failed) != 0 || len(sinks) != len(fx) {
		t.Fatalf("eșuate %d, extrase %d", len(rep.Failed), len(sinks))
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
		for _, secret := range []string{"SECRET-USER", "SECRET-HOST", "raport.txt", "poza.jpg", "/orig/"} {
			if bytes.Contains(b, []byte(secret)) {
				t.Errorf("%s conține %q — fișierele necriptate nu au voie să spună ce e în pachet", name, secret)
			}
		}
	}
	// index.dhsi e criptat: numele nu apar în clar.
	b, err := os.ReadFile(filepath.Join(dir, indexName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("raport.txt")) {
		t.Error("index.dhsi conține nume de fișier în clar")
	}
}

func TestPassphraseErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fixtures()[:2])
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, ""); !errors.Is(err, ErrNeedPassphrase) {
		t.Errorf("fără frază: %v, aștept ErrNeedPassphrase", err)
	}
	if _, err := Open(dir, "gresita-dar-lunga-destul"); !errors.Is(err, ErrBadPassphrase) {
		t.Errorf("frază greșită: %v, aștept ErrBadPassphrase", err)
	}
	if _, err := NewWriter(Options{Dir: filepath.Join(t.TempDir(), "x"), Level: scan.LevelBalanced, Passphrase: "scurt"}); err == nil {
		t.Error("o frază de 5 caractere trebuie refuzată la scriere")
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
		case "imagini/poza.jpg":
			orig = e
		case "imagini/copie.jpg":
			dup = e
		}
	}
	if orig == nil || dup == nil {
		t.Fatal("lipsesc intrările")
	}
	if !dup.Dup || orig.Dup {
		t.Errorf("copia trebuie marcată Dup, originalul nu: %v / %v", dup.Dup, orig.Dup)
	}
	if len(dup.Parts) != len(orig.Parts) {
		t.Fatalf("copia are %d părți, originalul %d", len(dup.Parts), len(orig.Parts))
	}
	for i := range dup.Parts {
		if dup.Parts[i] != orig.Parts[i] {
			t.Errorf("partea %d diferă: %+v vs %+v", i, dup.Parts[i], orig.Parts[i])
		}
	}
	if len(orig.Parts) < 2 {
		t.Errorf("300 KiB în blocuri de 64 KiB trebuie să dea ≥2 părți, am %d", len(orig.Parts))
	}
	// Ambele se extrag întregi, deși datele sunt o singură dată pe disc.
	sinks, rep := extractAll(t, r, func(e *Entry) bool { return e.Root == "imagini" })
	if len(rep.Failed) != 0 {
		t.Fatalf("%+v", rep.Failed)
	}
	if !bytes.Equal(sinks["imagini/poza.jpg"].buf.Bytes(), sinks["imagini/copie.jpg"].buf.Bytes()) {
		t.Error("originalul și copia diferă după extragere")
	}
}

func TestLargeFileSpansVolumes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	big := random(700 << 10) // 3,5 volume de test
	w := writePackage(t, dir, testPass, scan.LevelBalanced, []fixture{
		{name: "imagini/mare.jpg", root: "imagini", class: scan.Incompressible, data: big},
	})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, testPass)
	if err != nil {
		t.Fatal(err)
	}
	if r.Manifest.Volumes < 3 {
		t.Errorf("700 KiB în volume de 200 KiB trebuie să dea ≥3 volume, am %d", r.Manifest.Volumes)
	}
	e := r.Index.Entries[0]
	volumes := map[uint32]bool{}
	for _, p := range e.Parts {
		volumes[r.Index.Blocks[p.Block].Volume] = true
	}
	if len(volumes) < 2 {
		t.Errorf("fișierul trebuie să se întindă pe ≥2 volume, e pe %d", len(volumes))
	}
	sinks, rep := extractAll(t, r, nil)
	if len(rep.Failed) != 0 {
		t.Fatalf("%+v", rep.Failed)
	}
	if !bytes.Equal(sinks["imagini/mare.jpg"].buf.Bytes(), big) {
		t.Error("fișierul întins pe volume nu s-a refăcut identic")
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
	sinks, rep := extractAll(t, r, func(e *Entry) bool { return e.Path == "documente/scurt.md" })
	if len(sinks) != 1 || rep.Files != 1 || len(rep.Failed) != 0 {
		t.Fatalf("filtru: %d destinații, %d fișiere, %d eșuate", len(sinks), rep.Files, len(rep.Failed))
	}
	if !bytes.Equal(sinks["documente/scurt.md"].buf.Bytes(), text(900)) {
		t.Error("conținut diferit")
	}
}

func TestCorruptionIsDetected(t *testing.T) {
	// Fără criptare, ca să putem strica un octet din interiorul unui bloc și să vedem că doar
	// fișierele din acel bloc pică. Cu age, orice octet schimbat invalidează tot chunk-ul — și e bine.
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

	// Stricăm un octet din payload-ul primului bloc al primului volum.
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
		t.Fatal("Verify trebuie să prindă octetul stricat")
	}
	found := false
	for _, p := range rep.Problems {
		if bytes.Contains([]byte(p), []byte(volumeName(first.Volume))) || bytes.Contains([]byte(p), []byte("volumul")) {
			found = true
		}
	}
	if !found {
		t.Errorf("problemele nu arată spre volum: %v", rep.Problems)
	}

	sinks, xrep := extractAll(t, r, nil)
	if len(xrep.Failed) == 0 {
		t.Fatal("Extract trebuie să raporteze fișierele din blocul stricat")
	}
	for _, fl := range xrep.Failed {
		if s := sinks[fl.Entry.Path]; s != nil && s.committed {
			t.Errorf("%s a eșuat dar a ajuns la Commit", fl.Entry.Path)
		}
	}
	if xrep.Files == 0 {
		t.Error("fișierele din blocurile intacte trebuie să treacă")
	}
}

func TestAbortLeavesIncompletePackage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w := writePackage(t, dir, testPass, scan.LevelBalanced, fixtures())
	w.Abort(errors.New("test: cablul smuls"))

	m, err := Peek(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Complete {
		t.Error("manifestul unui pachet anulat nu are voie să spună complet")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, volumeDir))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".dhsv" {
			// Un volum încheiat e valid; doar cel în curs poate rămâne .tmp.
			continue
		}
		if filepath.Ext(e.Name()) != ".tmp" {
			t.Errorf("fișier neașteptat în volume/: %s", e.Name())
		}
	}
	if err := w.Close(); err == nil {
		t.Error("Close după Abort trebuie să întoarcă eroarea")
	}
}

func TestRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ceva"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(Options{Dir: dir, Level: scan.LevelBalanced}); err == nil {
		t.Error("un director cu conținut trebuie refuzat — nu suprascriem nimic")
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
		t.Error("aceeași cale de două ori trebuie refuzată")
	}
	w.Abort(nil)
}

func TestSizeMismatchIsRecorded(t *testing.T) {
	// Fișierul a crescut între scan și backup: reținem cât am citit, nu cât credeam.
	dir := filepath.Join(t.TempDir(), "p.dhs")
	data := text(10 << 10)
	w := writePackage(t, dir, testPass, scan.LevelBalanced, nil)
	err := w.Add(context.Background(), File{
		Root: "documente", Path: "crescut.txt", Size: 100, Class: scan.Text,
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
		t.Errorf("Size = %d, aștept %d (cât s-a citit de fapt)", r.Index.Entries[0].Size, len(data))
	}
}
