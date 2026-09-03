package restore

// NOT run on the user's laptop (AGENTS.md, rule 7) — only on GitHub Codespaces.

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

func TestSanitizeWindows(t *testing.T) {
	cases := map[string]string{
		"report: final.docx": "report_ final.docx",
		"a/b<c>d.txt":        "a/b_c_d.txt",
		"con.txt":            "_con.txt",
		"CON":                "_CON",
		"lpt1.log":           "_lpt1.log",
		"name.":              "name",
		"name  ":             "name",
		"normal/file.txt":    "normal/file.txt",
		"diacritics ăîș.md":  "diacritics ăîș.md",
		"../escape":          "_../escape",
	}
	for in, want := range cases {
		got, changed := Sanitize(in, system.Windows)
		if got != want {
			t.Errorf("Sanitize(%q, windows) = %q, want %q", in, got, want)
		}
		if changed != (in != want) {
			t.Errorf("Sanitize(%q): changed=%v", in, changed)
		}
	}
}

func TestSanitizeLinuxTouchesAlmostNothing(t *testing.T) {
	for _, in := range []string{"report: final.docx", "a<b>c", "con.txt", "name.", "CON"} {
		if got, changed := Sanitize(in, system.Linux); got != in || changed {
			t.Errorf("on Linux %q must not be changed, got %q", in, got)
		}
	}
	if got, _ := Sanitize("a\x00b", system.Linux); got != "a_b" {
		t.Errorf("NUL must be replaced on Linux too: %q", got)
	}
}

func TestWithSuffix(t *testing.T) {
	cases := map[string]string{
		"report.docx":     "report (DHS).docx",
		"dir/report.docx": "dir/report (DHS).docx",
		"no-extension":    "no-extension (DHS)",
		".bashrc":         ".bashrc (DHS)",
		"archive.tar.gz":  "archive.tar (DHS).gz",
	}
	for in, want := range cases {
		if got := WithSuffix(in, " (DHS)"); got != want {
			t.Errorf("WithSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFoldKey(t *testing.T) {
	if FoldKey("A.txt", system.Windows) != FoldKey("a.txt", system.Windows) {
		t.Error("on Windows A.txt and a.txt are the same file")
	}
	if FoldKey("A.txt", system.Linux) == FoldKey("a.txt", system.Linux) {
		t.Error("on Linux A.txt and a.txt are different files")
	}
}

// pkg writes a small package with the given entries and opens it.
func pkg(t *testing.T, files map[string][]byte) (*pack.Reader, map[string][]byte) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "p.dhs")
	w, err := pack.NewWriter(pack.Options{
		Dir: dir, Level: scan.LevelBalanced, Passphrase: "",
		Source:     system.Info{OS: system.Linux, Name: "Source"},
		VolumeSize: 200 << 10, BlockSize: 64 << 10, Workers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range files {
		root, rel := pack.Root("documents"), path
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
		"documents/report.txt":   []byte("report"),
		"documents/sub/notes.md": []byte("notes"),
		"pictures/photo.jpg":     []byte("not really a jpg"),
		"downloads/installer.sh": []byte("echo hello"), // root that does not exist on the destination
		"other/etc/hosts":        []byte("127.0.0.1 localhost"),
	})
	rm, home := roots(t)

	// One conflict: report.txt already exists, with different content.
	existing := filepath.Join(home, "Documents", "report.txt")
	if err := os.WriteFile(existing, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := Build(Options{Reader: r, Roots: rm, Target: system.Linux})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files != 5 || plan.Conflicts != 1 || plan.Skipped != 0 {
		t.Errorf("plan: %d files, %d conflicts, %d skipped", plan.Files, plan.Conflicts, plan.Skipped)
	}
	if len(plan.Unknown) != 1 || plan.Unknown[0] != "downloads" {
		t.Errorf("unknown roots = %v, want [downloads]", plan.Unknown)
	}

	rep, err := plan.Execute(context.Background(), ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 0 || rep.Files != 5 {
		t.Fatalf("failed %+v, files %d", rep.Failed, rep.Files)
	}

	check := func(path string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
	// The existing file is untouched; the restored one sits beside it.
	check(existing, []byte("OLD"))
	check(filepath.Join(home, "Documents", "report (DHS).txt"), files["documents/report.txt"])
	check(filepath.Join(home, "Documents", "sub", "notes.md"), files["documents/sub/notes.md"])
	check(filepath.Join(home, "Pictures", "photo.jpg"), files["pictures/photo.jpg"])
	// The missing root and "other" go under DHS-restored, not to absolute paths.
	check(filepath.Join(home, "DHS-restored", "downloads", "installer.sh"), files["downloads/installer.sh"])
	check(filepath.Join(home, "DHS-restored", "etc", "hosts"), files["other/etc/hosts"])

	// Metadata.
	st, err := os.Stat(filepath.Join(home, "Documents", "sub", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 640", st.Mode().Perm())
	}
	if !st.ModTime().Equal(time.Unix(1_600_000_000, 0)) {
		t.Errorf("mtime = %v, want 1600000000", st.ModTime())
	}

	// No temporary left behind.
	filepath.WalkDir(home, func(p string, d os.DirEntry, _ error) error {
		if d != nil && filepath.Ext(d.Name()) == ".dhs-tmp" {
			t.Errorf("temporary left behind: %s", p)
		}
		return nil
	})
}

func TestPlanConflictPolicies(t *testing.T) {
	r, files := pkg(t, map[string][]byte{"documents/a.txt": []byte("new")})
	rm, home := roots(t)
	dest := filepath.Join(home, "Documents", "a.txt")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	skip, _ := Build(Options{Reader: r, Roots: rm, Target: system.Linux, Conflict: Skip})
	if skip.Files != 0 || skip.Skipped != 1 || skip.Items[0].Action != SkipIt {
		t.Errorf("skip: %+v", skip.Items[0])
	}
	if _, err := skip.Execute(context.Background(), ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "old" {
		t.Error("\"skip\" touched the existing file")
	}

	over, _ := Build(Options{Reader: r, Roots: rm, Target: system.Linux, Conflict: Overwrite})
	if over.Items[0].Action != Replace || over.Items[0].Dest != dest {
		t.Errorf("overwrite: %+v", over.Items[0])
	}
	if _, err := over.Execute(context.Background(), ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); !bytes.Equal(b, files["documents/a.txt"]) {
		t.Error("\"overwrite\" did not replace the file")
	}
}

func TestPlanCaseCollisionOnWindows(t *testing.T) {
	r, _ := pkg(t, map[string][]byte{
		"documents/Report.txt": []byte("1"),
		"documents/report.txt": []byte("2"),
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
		t.Errorf("two entries that collide on Windows must end up in two files: %+v", plan.Items)
	}
	if plan.Renamed != 1 {
		t.Errorf("renamed = %d, want 1", plan.Renamed)
	}
}

func TestPlanFilter(t *testing.T) {
	r, _ := pkg(t, map[string][]byte{"documents/a.txt": []byte("a"), "pictures/b.jpg": []byte("b")})
	rm, _ := roots(t)
	plan, err := Build(Options{Reader: r, Roots: rm, Target: system.Linux, Filter: func(e *pack.Entry) bool { return e.Root == "pictures" }})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files != 1 || plan.Items[0].Entry.Root != "pictures" {
		t.Errorf("filter: %+v", plan.Items)
	}
}

func TestParseConflict(t *testing.T) {
	for in, want := range map[string]Conflict{"": KeepBoth, "keep-both": KeepBoth, "skip": Skip, "overwrite": Overwrite} {
		got, err := ParseConflict(in)
		if err != nil || got != want {
			t.Errorf("ParseConflict(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseConflict("something-else"); err == nil {
		t.Error("an unknown policy must be refused")
	}
}
