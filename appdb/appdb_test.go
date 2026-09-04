package appdb

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestEmbeddedDatabaseValidates is the gate: a database that does not validate does not build.
func TestEmbeddedDatabaseValidates(t *testing.T) {
	db, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if db.Len() < 6 {
		t.Fatalf("only %d entries; the model entries alone are more", db.Len())
	}
	for _, a := range db.Apps() {
		if got := db.Get(a.ID); got != a {
			t.Errorf("Get(%q) returned a different entry", a.ID)
		}
	}
}

func TestQueries(t *testing.T) {
	db, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ff := db.Get("firefox")
	if ff == nil {
		t.Fatal("firefox is missing")
	}
	cases := []struct {
		m    Manager
		id   string
		want string
	}{
		{Winget, "mozilla.firefox", "firefox"}, // case-insensitive
		{Apt, "firefox-esr", "firefox"},        // a secondary name is recognised
		{Snap, "code", "vscode"},               // "code --classic" is known as "code"
		{Pacman, "does-not-exist", ""},
	}
	for _, c := range cases {
		got := db.ByPackage(c.m, c.id)
		name := ""
		if got != nil {
			name = got.ID
		}
		if name != c.want {
			t.Errorf("ByPackage(%s, %q) = %q, want %q", c.m, c.id, name, c.want)
		}
	}
	reg := []struct{ display, want string }{
		{"Mozilla Firefox (x64 en-US)", "firefox"},
		{"mozilla firefox", "firefox"},
		{"Git", "git"},
		{"Gitter Widget 2.0", ""}, // a prefix inside a word does not count
		{"Microsoft Visual Studio Code (User)", "vscode"},
		{"Notepad++ (64-bit x64)", "notepad-plus-plus"},
	}
	for _, c := range reg {
		got := db.ByRegistryName(c.display)
		name := ""
		if got != nil {
			name = got.ID
		}
		if name != c.want {
			t.Errorf("ByRegistryName(%q) = %q, want %q", c.display, name, c.want)
		}
	}
}

func TestExcluded(t *testing.T) {
	a := &App{Config: Config{Exclude: []string{"cache2", "*.lock", "storage/default/*/cache"}}}
	yes := []string{"cache2/entries/x", "profile.default/cache2", "parent.lock", "Storage/Default/moz-extension/cache", "storage/default/a/cache"}
	no := []string{"prefs.js", "storage/default/a/idb/x", "cache2x/y", "lockfile"}
	for _, p := range yes {
		if !a.Excluded(p) {
			t.Errorf("%q should be excluded", p)
		}
	}
	for _, p := range no {
		if a.Excluded(p) {
			t.Errorf("%q should not be excluded", p)
		}
	}
}

func TestValidationCatchesMistakes(t *testing.T) {
	good := `{"id":"good","name":"Good","category":"utility","linux":{"pacman":["good"]},"config":{"portability":"none"}}`
	cases := map[string]string{
		"unknown field":   `{"id":"x","name":"X","category":"utility","colour":"red","linux":{"pacman":["x"]},"config":{"portability":"none"}}`,
		"wrong file name": `{"id":"y","name":"Y","category":"utility","linux":{"pacman":["y"]},"config":{"portability":"none"}}`,
		"bad category":    `{"id":"x","name":"X","category":"Editor","linux":{"pacman":["x"]},"config":{"portability":"none"}}`,
		"absolute path":   `{"id":"x","name":"X","category":"utility","linux":{"pacman":["x"],"paths":{"c":"/etc/x"}},"config":{"portability":"identical"}}`,
		"backslash":       `{"id":"x","name":"X","category":"utility","windows":{"winget":["A.X"],"paths":{"c":"%APPDATA%\\X"}},"config":{"portability":"identical"}}`,
		"none with paths": `{"id":"x","name":"X","category":"utility","linux":{"pacman":["x"],"paths":{"c":"~/.x"}},"config":{"portability":"none"}}`,
		"manager on OS":   `{"id":"x","name":"X","category":"utility","linux":{"winget":["A.X"]},"config":{"portability":"none"}}`,
		"dangling equiv":  `{"id":"x","name":"X","category":"utility","linux":{"pacman":["x"]},"config":{"portability":"none"},"equivalents":["nope"]}`,
		"claimed twice":   `{"id":"x","name":"X","category":"utility","linux":{"pacman":["good"]},"config":{"portability":"none"}}`,
		"no shared key":   `{"id":"x","name":"X","category":"utility","windows":{"winget":["A.X"],"paths":{"a":"%APPDATA%/X"}},"linux":{"pacman":["x"],"paths":{"b":"~/.x"}},"config":{"portability":"identical"}}`,
	}
	for name, body := range cases {
		fsys := fstest.MapFS{
			"apps/good.json": {Data: []byte(good)},
			"apps/x.json":    {Data: []byte(body)},
		}
		if _, err := LoadFS(fsys, "apps"); err == nil {
			t.Errorf("%s: expected a validation error", name)
		} else if !strings.Contains(err.Error(), "appdb") {
			t.Errorf("%s: unexpected error %v", name, err)
		}
	}
	if _, err := LoadFS(fstest.MapFS{"apps/good.json": {Data: []byte(good)}}, "apps"); err != nil {
		t.Errorf("the good entry alone should load: %v", err)
	}
}
