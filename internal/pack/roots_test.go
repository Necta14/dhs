package pack

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Necta14/dhs/internal/system"
)

func TestRootMapSplitJoin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("căile de test sunt POSIX")
	}
	home := "/home/x"
	m := NewRootMap(home, []system.Location{
		{Kind: system.Documents, Path: "/home/x/Documente"},
		{Kind: system.Pictures, Path: "/home/x/Documente/Poze"}, // imbricat: prefixul lung câștigă
		{Kind: system.Config, Path: "/home/x/.config"},
	})

	cases := []struct {
		abs  string
		root Root
		rel  string
	}{
		{"/home/x/Documente/a/b.txt", "documente", "a/b.txt"},
		{"/home/x/Documente/Poze/vara.jpg", "imagini", "vara.jpg"},
		{"/home/x/Documente", "documente", "."},
		{"/home/x/.config/app/set.ini", "config", "app/set.ini"},
		{"/home/x/proiect/main.go", RootOther, "home/x/proiect/main.go"},
		{"/etc/hosts", RootOther, "etc/hosts"},
	}
	for _, c := range cases {
		root, rel := m.Split(c.abs)
		if root != c.root || rel != c.rel {
			t.Errorf("Split(%q) = (%s, %s), aștept (%s, %s)", c.abs, root, rel, c.root, c.rel)
		}
	}

	if p, ok := m.Join("documente", "a/b.txt"); !ok || p != "/home/x/Documente/a/b.txt" {
		t.Errorf("Join documente = %q %v", p, ok)
	}
	if p, ok := m.Join(RootOther, "etc/hosts"); !ok || p != filepath.Join(home, "DHS-restaurat", "etc/hosts") {
		t.Errorf("Join alt = %q — fișierele din afara profilului merg în DHS-restaurat, nu în calea originală", p)
	}
	if _, ok := m.Join("video", "x"); ok {
		t.Error("o rădăcină necunoscută pe sistemul destinație nu se poate rezolva")
	}
}

func TestOtherKeyWindows(t *testing.T) {
	// Calea Windows trebuie să devină portabilă chiar și când o citim de pe Linux.
	if runtime.GOOS != "windows" {
		t.Skip("filepath.VolumeName recunoaște literele de unitate doar pe Windows")
	}
	if got := otherKey(`C:\Users\x\a.txt`); got != "C/Users/x/a.txt" {
		t.Errorf("otherKey = %q", got)
	}
}
