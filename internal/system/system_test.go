package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetect(t *testing.T) {
	i, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	if string(i.OS) != runtime.GOOS {
		t.Errorf("OS = %q, aștept %q", i.OS, runtime.GOOS)
	}
	if i.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, aștept %q", i.Arch, runtime.GOARCH)
	}
	if i.Name == "" {
		t.Error("numele sistemului nu trebuie să fie gol")
	}
	if i.Home == "" {
		t.Error("directorul acasă nu trebuie să fie gol")
	}
	if st, err := os.Stat(i.Home); err != nil || !st.IsDir() {
		t.Errorf("Home = %q, dar nu e un director accesibil", i.Home)
	}
	if i.String() == "" {
		t.Error("String() nu trebuie să fie gol")
	}
}

func TestLocationsCoversEveryKind(t *testing.T) {
	i, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	locs := Locations(i)
	if len(locs) != len(KindOrder) {
		t.Fatalf("locuri = %d, aștept %d", len(locs), len(KindOrder))
	}
	for n, l := range locs {
		if l.Kind != KindOrder[n] {
			t.Errorf("locul %d e %q, aștept %q — ordinea contează pentru interfață", n, l.Kind, KindOrder[n])
		}
		if l.Path == "" {
			t.Errorf("locul %q are cale goală", l.Kind)
		}
		if !filepath.IsAbs(l.Path) {
			t.Errorf("locul %q are cale relativă: %q", l.Kind, l.Path)
		}
	}
}

func TestSpaceOf(t *testing.T) {
	v, err := SpaceOf(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if v.Total <= 0 {
		t.Errorf("total = %d, aștept pozitiv", v.Total)
	}
	if v.Free < 0 || v.Free > v.Total {
		t.Errorf("liber = %d, total = %d — valori incoerente", v.Free, v.Total)
	}
}

func TestFAT32FileLimit(t *testing.T) {
	if got := FSFAT32.MaxFileSize(); got != 4<<30-1 {
		t.Errorf("limita FAT32 = %d, aștept %d", got, 4<<30-1)
	}
	for _, k := range []FSKind{FSNTFS, FSExt, FSBtrfs, FSExFAT} {
		if k.MaxFileSize() != 0 {
			t.Errorf("%s nu are limită practică de fișier", k)
		}
	}
}
