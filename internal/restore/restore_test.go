package restore

// NU se rulează pe laptopul userului (AGENTS.md, regula 7) — doar pe Codespaces.

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/necta/dhs/internal/pack"
	"github.com/necta/dhs/internal/scan"
	"github.com/necta/dhs/internal/system"
)

func TestSanitizeWindows(t *testing.T) {
	cases := map[string]string{
		"raport: final.docx":   "raport_ final.docx",
		"a/b<c>d.txt":          "a/b_c_d.txt",
		"con.txt":              "_con.txt",
		"CON":                  "_CON",
		"lpt1.log":             "_lpt1.log",
		"nume.":                "nume",
		"nume  ":               "nume",
		"normal/fisier.txt":    "normal/fisier.txt",
		"cu diacritice ăîș.md": "cu diacritice ăîș.md",
		"../evadare":           "_../evadare",
	}
	for in, want := range cases {
		got, changed := Sanitize(in, system.Windows)
		if got != want {
			t.Errorf("Sanitize(%q, windows) = %q, aștept %q", in, got, want)
		}
		if changed != (in != want) {
			t.Errorf("Sanitize(%q): changed=%v", in, changed)
		}
	}
}

func TestSanitizeLinuxTouchesAlmostNothing(t *testing.T) {
	for _, in := range []string{"raport: final.docx", "a<b>c", "con.txt", "nume.", "CON"} {
		if got, changed := Sanitize(in, system.Linux); got != in || changed {
			t.Errorf("pe Linux %q nu trebuie schimbat, am %q", in, got)
		}
	}
	if got, _ := Sanitize("a\x00b", system.Linux); got != "a_b" {
		t.Errorf("NUL trebuie înlocuit și pe Linux: %q", got)
	}
}

func TestWithSuffix(t *testing.T) {
	cases := map[string]string{
		"raport.docx":     "raport (DHS).docx",
		"dir/raport.docx": "dir/raport (DHS).docx",
		"fara-extensie":   "fara-extensie (DHS)",
		".bashrc":         ".bashrc (DHS)",
		"arhiva.tar.gz":   "arhiva.tar (DHS).gz",
	}
	for in, want := range cases {
		if got := WithSuffix(in, " (DHS)"); got != want {
			t.Errorf("WithSuffix(%q) = %q, aștept %q", in, got, want)
		}
	}
}

func TestFoldKey(t *testing.T) {
	if FoldKey("A.txt", system.Windows) != FoldKey("a.txt", system.Windows) {
		t.Error("pe Windows A.txt și a.txt sunt același fișier")
	}
	if FoldKey("A.txt", system.Linux) == FoldKey("a.txt", system.Linux) {
		t.Error("pe Linux A.txt și a.txt sunt fișiere diferite")
	}
}

// pkg scrie un pachet mic cu intrările date și îl deschide.
func pkg(t *testing.T, files map[string][]byte) (*pack.Reader, map[string][]byte) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w, err := pack.NewWriter(pack.Options{
		Dir: dir, Level: scan.LevelBalanced, Passphrase: "",
		Source:     system.Info{OS: system.Linux, Name: "Sursă"},
		VolumeSize: 200 << 10, BlockSize: 64 << 10, Workers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range files {
		root, rel := pack.Root("documente"), path
		if i := bytes.IndexByte([]byte(path), '/'); i > 0 {
			root, rel = pack.Root(path[:i]), path[i+1:]
		}
		data := data
		err := w.Add(context.Background(), pack.File{
			Root: root, Path: rel, Orig: "/src/" + path, Size: int64(len(data)),
			Mode: 0o640, ModTime: time.Unix(1_600_000_000, 0), Class: scan.Text,
			Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := pack.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	return r, files
}

func roots(t *testing.T) (*pack.RootMap, string) {
	t.Helper()
	home := t.TempDir()
	docs := filepath.Join(home, "Documents")
	pics := filepath.Join(home, "Pictures")
	for _, d := range []string{docs, pics} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return pack.NewRootMap(home, []system.Location{
		{Kind: system.Documents, Path: docs, Exists: true},
		{Kind: system.Pictures, Path: pics, Exists: true},
	}), home
}

func TestPlanAndExecute(t *testing.T) {
	r, files := pkg(t, map[string][]byte{
		"documente/raport.txt":     []byte("raport"),
		"documente/sub/note.md":    []byte("note"),
		"imagini/poza.jpg":         []byte("nu e chiar jpg"),
		"descarcari/instalator.sh": []byte("echo salut"), // rădăcină inexistentă pe destinație
		"alt/etc/hosts":            []byte("127.0.0.1 localhost"),
	})
	rm, home := roots(t)

	// Un conflict: raport.txt există deja, cu alt conținut.
	existing := filepath.Join(home, "Documents", "raport.txt")
	if err := os.WriteFile(existing, []byte("VECHI"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := Build(Options{Reader: r, Roots: rm, Target: system.Linux})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files != 5 || plan.Conflicts != 1 || plan.Skipped != 0 {
		t.Errorf("plan: %d fișiere, %d conflicte, %d sărite", plan.Files, plan.Conflicts, plan.Skipped)
	}
	if len(plan.Unknown) != 1 || plan.Unknown[0] != "descarcari" {
		t.Errorf("rădăcini necunoscute = %v, aștept [descarcari]", plan.Unknown)
	}

	rep, err := plan.Execute(context.Background(), ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 0 || rep.Files != 5 {
		t.Fatalf("eșuate %+v, fișiere %d", rep.Failed, rep.Files)
	}

	check := func(path string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: %q, aștept %q", path, got, want)
		}
	}
	// Existentul e neatins; restauratul e alături.
	check(existing, []byte("VECHI"))
	check(filepath.Join(home, "Documents", "raport (DHS).txt"), files["documente/raport.txt"])
	check(filepath.Join(home, "Documents", "sub", "note.md"), files["documente/sub/note.md"])
	check(filepath.Join(home, "Pictures", "poza.jpg"), files["imagini/poza.jpg"])
	// Rădăcina inexistentă și „alt" merg sub DHS-restaurat, nu în căi absolute.
	check(filepath.Join(home, "DHS-restaurat", "descarcari", "instalator.sh"), files["descarcari/instalator.sh"])
	check(filepath.Join(home, "DHS-restaurat", "etc", "hosts"), files["alt/etc/hosts"])

	// Metadate.
	st, err := os.Stat(filepath.Join(home, "Documents", "sub", "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o640 {
		t.Errorf("mod = %o, aștept 640", st.Mode().Perm())
	}
	if !st.ModTime().Equal(time.Unix(1_600_000_000, 0)) {
		t.Errorf("mtime = %v, aștept 1600000000", st.ModTime())
	}

	// Niciun temporar rămas.
	filepath.WalkDir(home, func(p string, d os.DirEntry, _ error) error {
		if d != nil && filepath.Ext(d.Name()) == ".dhs-tmp" {
			t.Errorf("temporar rămas: %s", p)
		}
		return nil
	})
}

func TestPlanConflictPolicies(t *testing.T) {
	r, files := pkg(t, map[string][]byte{"documente/a.txt": []byte("nou")})
	rm, home := roots(t)
	dest := filepath.Join(home, "Documents", "a.txt")
	if err := os.WriteFile(dest, []byte("vechi"), 0o644); err != nil {
		t.Fatal(err)
	}

	skip, _ := Build(Options{Reader: r, Roots: rm, Target: system.Linux, Conflict: Skip})
	if skip.Files != 0 || skip.Skipped != 1 || skip.Items[0].Action != SkipIt {
		t.Errorf("sari: %+v", skip.Items[0])
	}
	if _, err := skip.Execute(context.Background(), ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "vechi" {
		t.Error("„sari” a atins fișierul existent")
	}

	over, _ := Build(Options{Reader: r, Roots: rm, Target: system.Linux, Conflict: Overwrite})
	if over.Items[0].Action != Replace || over.Items[0].Dest != dest {
		t.Errorf("suprascrie: %+v", over.Items[0])
	}
	if _, err := over.Execute(context.Background(), ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); !bytes.Equal(b, files["documente/a.txt"]) {
		t.Error("„suprascrie” nu a înlocuit fișierul")
	}
}

func TestPlanCaseCollisionOnWindows(t *testing.T) {
	r, _ := pkg(t, map[string][]byte{
		"documente/Raport.txt": []byte("1"),
		"documente/raport.txt": []byte("2"),
	})
	rm, _ := roots(t)
	plan, err := Build(Options{Reader: r, Roots: rm, Target: system.Windows})
	if err != nil {
		t.Fatal(err)
	}
	dests := map[string]bool{}
	for _, it := range plan.Items {
		dests[FoldKey(it.Dest, system.Windows)] = true
	}
	if len(dests) != 2 {
		t.Errorf("două intrări care se ciocnesc pe Windows trebuie să ajungă în două fișiere: %+v", plan.Items)
	}
	if plan.Renamed != 1 {
		t.Errorf("redenumite = %d, aștept 1", plan.Renamed)
	}
}

func TestPlanFilter(t *testing.T) {
	r, _ := pkg(t, map[string][]byte{"documente/a.txt": []byte("a"), "imagini/b.jpg": []byte("b")})
	rm, _ := roots(t)
	plan, err := Build(Options{Reader: r, Roots: rm, Target: system.Linux, Filter: func(e *pack.Entry) bool { return e.Root == "imagini" }})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files != 1 || plan.Items[0].Entry.Root != "imagini" {
		t.Errorf("filtru: %+v", plan.Items)
	}
}

func TestParseConflict(t *testing.T) {
	for in, want := range map[string]Conflict{"": KeepBoth, "alaturi": KeepBoth, "sari": Skip, "suprascrie": Overwrite} {
		got, err := ParseConflict(in)
		if err != nil || got != want {
			t.Errorf("ParseConflict(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseConflict("altceva"); err == nil {
		t.Error("politica necunoscută trebuie refuzată")
	}
}
