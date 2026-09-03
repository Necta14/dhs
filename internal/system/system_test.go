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
		t.Errorf("OS = %q, want %q", i.OS, runtime.GOOS)
	}
	if i.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", i.Arch, runtime.GOARCH)
	}
	if i.Name == "" {
		t.Error("system name must not be empty")
	}
	if i.Home == "" {
		t.Error("home directory must not be empty")
	}
	if st, err := os.Stat(i.Home); err != nil || !st.IsDir() {
		t.Errorf("Home = %q, but it is not an accessible directory", i.Home)
	}
	if i.String() == "" {
		t.Error("String() must not be empty")
	}
}

func TestLocationsCoversEveryKind(t *testing.T) {
	i, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	locs := Locations(i)
	if len(locs) != len(KindOrder) {
		t.Fatalf("locations = %d, want %d", len(locs), len(KindOrder))
	}
	for n, l := range locs {
		if l.Kind != KindOrder[n] {
			t.Errorf("location %d is %q, want %q — the order matters for the interface", n, l.Kind, KindOrder[n])
		}
		if l.Path == "" {
			t.Errorf("location %q has an empty path", l.Kind)
		}
		if !filepath.IsAbs(l.Path) {
			t.Errorf("location %q has a relative path: %q", l.Kind, l.Path)
		}
	}
}

func TestSpaceOf(t *testing.T) {
	v, err := SpaceOf(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if v.Total <= 0 {
		t.Errorf("total = %d, want positive", v.Total)
	}
	if v.Free < 0 || v.Free > v.Total {
		t.Errorf("free = %d, total = %d — inconsistent values", v.Free, v.Total)
	}
}

func TestFAT32FileLimit(t *testing.T) {
	if got := FSFAT32.MaxFileSize(); got != 4<<30-1 {
		t.Errorf("FAT32 limit = %d, want %d", got, 4<<30-1)
	}
	for _, k := range []FSKind{FSNTFS, FSExt, FSBtrfs, FSExFAT} {
		if k.MaxFileSize() != 0 {
			t.Errorf("%s has no practical file size limit", k)
		}
	}
}
