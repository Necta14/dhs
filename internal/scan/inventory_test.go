package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// tree construiește un arbore de probă și întoarce rădăcina.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]int{
		"Documents/raport.txt":             5000,
		"Documents/contract.docx":          20000,
		"Pictures/vacanta.jpg":             900000,
		"Pictures/schita.bmp":              40000,
		"proiect/main.go":                  3000,
		"proiect/node_modules/pachet/a.js": 700000,
		"proiect/.git/objects/abc":         250000,
		".cache/browser/blob":              400000,
		".ssh/id_rsa":                      2000,
		".env":                             300,
	}
	for rel, size := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWalkClassifiesAndExcludes(t *testing.T) {
	root := tree(t)
	res, err := Walk(Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}

	// Incluse: raport.txt, contract.docx, vacanta.jpg, schita.bmp, main.go = 5 fișiere.
	if res.Files != 5 {
		t.Errorf("fișiere incluse = %d, aștept 5 (%+v)", res.Files, res.ByClass)
	}
	wantBytes := int64(5000 + 20000 + 900000 + 40000 + 3000)
	if res.Bytes != wantBytes {
		t.Errorf("octeți = %d, aștept %d", res.Bytes, wantBytes)
	}

	// Secretele sunt numărate, dar nu incluse.
	if res.Secrets.Files != 2 {
		t.Errorf("secrete = %d, aștept 2 (id_rsa, .env)", res.Secrets.Files)
	}

	// Excluderile au tăiat node_modules, .git și .cache.
	if got := res.SkippedBytes(); got != 700000+250000+400000 {
		t.Errorf("octeți excluși = %d, aștept %d", got, 700000+250000+400000)
	}
	byDir := map[string]int64{}
	for _, s := range res.Skipped {
		byDir[s.Dir] = s.Bytes
	}
	for _, dir := range []string{"node_modules", ".git", ".cache"} {
		if byDir[dir] == 0 {
			t.Errorf("%q nu apare între excluderi: %+v", dir, byDir)
		}
	}
	// Excluderile se raportează descrescător, ca utilizatorul să vadă întâi ce contează.
	for i := 1; i < len(res.Skipped); i++ {
		if res.Skipped[i-1].Bytes < res.Skipped[i].Bytes {
			t.Error("excluderile nu sunt sortate descrescător")
		}
	}

	if res.ByClass[Incompressible].Files != 2 { // jpg + docx
		t.Errorf("incompresibile = %d, aștept 2", res.ByClass[Incompressible].Files)
	}
	if res.ByClass[Text].Files != 3 { // txt + bmp + go
		t.Errorf("text = %d, aștept 3", res.ByClass[Text].Files)
	}
}

func TestWalkIncludeSecrets(t *testing.T) {
	root := tree(t)
	res, err := Walk(Options{Roots: []string{root}, IncludeSecrets: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 7 {
		t.Errorf("fișiere = %d, aștept 7 cu secrete incluse", res.Files)
	}
}

func TestWalkLargestAndCallbacks(t *testing.T) {
	root := tree(t)
	var seen int
	res, err := Walk(Options{
		Roots:   []string{root},
		TopN:    3,
		OnEntry: func(Entry) { seen++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != int(res.Files) {
		t.Errorf("OnEntry chemat de %d ori, aștept %d", seen, res.Files)
	}
	if len(res.Largest) != 3 {
		t.Fatalf("cele mai mari = %d, aștept 3", len(res.Largest))
	}
	if filepath.Base(res.Largest[0].Path) != "vacanta.jpg" {
		t.Errorf("cel mai mare fișier = %s, aștept vacanta.jpg", res.Largest[0].Path)
	}
	for i := 1; i < len(res.Largest); i++ {
		if res.Largest[i-1].Size < res.Largest[i].Size {
			t.Error("lista celor mai mari nu e sortată descrescător")
		}
	}
}

func TestWalkAllowExclusion(t *testing.T) {
	root := tree(t)
	ex := DefaultExcluder()
	ex.Allow("node_modules")
	res, err := Walk(Options{Roots: []string{root}, Excluder: ex})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 6 {
		t.Errorf("fișiere = %d, aștept 6 după ce am readus node_modules", res.Files)
	}
}

func TestWalkSkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("legăturile simbolice cer drepturi speciale pe Windows")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	// O legătură către rădăcina proprie ar duce la ciclu infinit dacă am urma-o.
	if err := os.Symlink(root, filepath.Join(root, "bucla")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "copie.txt")); err != nil {
		t.Fatal(err)
	}

	res, err := Walk(Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 1 || res.Bytes != 100 {
		t.Errorf("fișiere = %d, octeți = %d; aștept 1 și 100 (legăturile nu se urmăresc)", res.Files, res.Bytes)
	}
	if res.Symlinks != 2 {
		t.Errorf("legături = %d, aștept 2", res.Symlinks)
	}
}

func TestWalkSingleFileRoot(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "unul.jpg")
	if err := os.WriteFile(p, make([]byte, 42), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Walk(Options{Roots: []string{p}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 1 || res.Bytes != 42 {
		t.Errorf("un fișier dat direct trebuie inclus: %d fișiere, %d octeți", res.Files, res.Bytes)
	}
}

func TestWalkNoRoots(t *testing.T) {
	if _, err := Walk(Options{}); err == nil {
		t.Error("aștept eroare fără rădăcini")
	}
}

func TestHomeRootsDropsNestedAndMissing(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "Documents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := HomeRoots([]string{root, sub, filepath.Join(root, "nu-exista"), root})
	if len(got) != 1 || got[0] != filepath.Clean(root) {
		t.Errorf("HomeRoots = %v, aștept doar %q", got, root)
	}
}
