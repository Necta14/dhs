package pack

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Necta14/dhs/internal/system"
)

func TestRootMapSplitJoin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test paths are POSIX")
	}
	home := "/home/x"
	m := NewRootMap(home, []system.Location{
		{Kind: system.Documents, Path: "/home/x/Documents"},
		{Kind: system.Pictures, Path: "/home/x/Documents/Photos"}, // nested: the long prefix wins
		{Kind: system.Config, Path: "/home/x/.config"},
	})

	cases := []struct {
		abs  string
		root Root
		rel  string
	}{
		{"/home/x/Documents/a/b.txt", "documents", "a/b.txt"},
		{"/home/x/Documents/Photos/summer.jpg", "pictures", "summer.jpg"},
		{"/home/x/Documents", "documents", "."},
		{"/home/x/.config/app/set.ini", "config", "app/set.ini"},
		{"/home/x/project/main.go", RootOther, "home/x/project/main.go"},
		{"/etc/hosts", RootOther, "etc/hosts"},
	}
	for _, c := range cases {
		root, rel := m.Split(c.abs)
		if root != c.root || rel != c.rel {
			t.Errorf("Split(%q) = (%s, %s), want (%s, %s)", c.abs, root, rel, c.root, c.rel)
		}
	}

	if p, ok := m.Join("documents", "a/b.txt"); !ok || p != "/home/x/Documents/a/b.txt" {
		t.Errorf("Join documents = %q %v", p, ok)
	}
	if p, ok := m.Join(RootOther, "etc/hosts"); !ok || p != filepath.Join(home, "DHS-restored", "etc/hosts") {
		t.Errorf("Join other = %q — files outside the profile go into DHS-restored, not into the original path", p)
	}
	if _, ok := m.Join("videos", "x"); ok {
		t.Error("a root unknown on the destination system cannot be resolved")
	}
}

func TestOtherKeyWindows(t *testing.T) {
	// The Windows path must become portable even when we read it from Linux.
	if runtime.GOOS != "windows" {
		t.Skip("filepath.VolumeName recognizes drive letters only on Windows")
	}
	if got := otherKey(`C:\Users\x\a.txt`); got != "C/Users/x/a.txt" {
		t.Errorf("otherKey = %q", got)
	}
}
