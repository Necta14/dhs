package apps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/system"
)

func linuxEnv(vars map[string]string) Env {
	return Env{OS: appdb.Linux, Home: "/home/u", Get: func(k string) string { return vars[k] }}
}

func TestResolve(t *testing.T) {
	env := linuxEnv(nil)
	cases := []struct {
		p       string
		variant Variant
		sandbox string
		want    string
	}{
		{"~/.mozilla/firefox", Native, "", "/home/u/.mozilla/firefox"},
		{"$XDG_CONFIG_HOME/Code/User", Native, "", "/home/u/.config/Code/User"},
		{"$XDG_DATA_HOME/kate", Native, "", "/home/u/.local/share/kate"},
		{"$XDG_STATE_HOME/x", Native, "", "/home/u/.local/state/x"},
		{"$XDG_CONFIG_HOME/Code/User", Flatpak, "com.visualstudio.code", "/home/u/.var/app/com.visualstudio.code/config/Code/User"},
		{"~/.mozilla/firefox", Flatpak, "org.mozilla.firefox", "/home/u/.var/app/org.mozilla.firefox/.mozilla/firefox"},
		{"$XDG_CONFIG_HOME/Code/User", Snap, "code", "/home/u/snap/code/current/.config/Code/User"},
		{"$XDG_DATA_HOME/x", Snap, "x", "/home/u/snap/x/current/.local/share/x"},
	}
	for _, c := range cases {
		got, ok := Resolve(c.p, env, c.variant, c.sandbox)
		if !ok || got != filepath.FromSlash(c.want) {
			t.Errorf("Resolve(%q, %q) = %q, %v; want %q", c.p, c.variant, got, ok, c.want)
		}
	}
	moved := linuxEnv(map[string]string{"XDG_CONFIG_HOME": "/home/u/cfg"})
	if got, _ := Resolve("$XDG_CONFIG_HOME/x", moved, Native, ""); got != filepath.FromSlash("/home/u/cfg/x") {
		t.Errorf("XDG_CONFIG_HOME override ignored: %q", got)
	}
	for _, bad := range []string{"/etc/x", "$HOME/x", "~", "~/", "%APPDATA%/x"} {
		if _, ok := Resolve(bad, env, Native, ""); ok {
			t.Errorf("%q should not resolve on Linux", bad)
		}
	}

	win := Env{OS: appdb.Windows, Home: `C:\Users\u`, Get: func(k string) string {
		return map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`, "LOCALAPPDATA": `C:\Users\u\AppData\Local`}[k]
	}}
	wcases := map[string]string{
		"%APPDATA%/Code/User":      `C:\Users\u\AppData\Roaming\Code\User`,
		"%LOCALAPPDATA%/kate":      `C:\Users\u\AppData\Local\kate`,
		"%USERPROFILE%/.gitconfig": `C:\Users\u\.gitconfig`,
	}
	for p, want := range wcases {
		got, ok := Resolve(p, win, Native, "")
		// On a Linux test host filepath.Join uses "/" and keeps the backslashes of the fake
		// Windows home as ordinary characters; on Windows the result is all backslashes. Compare
		// with every separator folded to "/".
		if !ok || strings.ReplaceAll(got, `\`, "/") != strings.ReplaceAll(want, `\`, "/") {
			t.Errorf("Resolve(%q) = %q, %v", p, got, ok)
		}
	}
	if _, ok := Resolve("%APPDATA%/x", win, Flatpak, "x"); ok {
		t.Error("a sandbox variant on Windows should not resolve")
	}
}

func TestPacmanDesc(t *testing.T) {
	desc := "%NAME%\nfirefox\n\n%VERSION%\n142.0-1\n\n%BASE%\nfirefox\n"
	s := parsePacmanDesc([]byte(desc))
	if s.Package != "firefox" || s.Version != "142.0-1" || s.Manager != appdb.Pacman {
		t.Errorf("got %+v", s)
	}
}

func TestDpkgStatus(t *testing.T) {
	dir := t.TempDir()
	status := `Package: firefox-esr
Status: install ok installed
Version: 128.3.0esr-1
Description: Mozilla Firefox web browser
 Long description
 continues.

Package: removed-thing
Status: deinstall ok config-files
Version: 1.0

Package: git
Status: install ok installed
Version: 1:2.45.2-1
`
	p := filepath.Join(dir, "status")
	if err := os.WriteFile(p, []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readDpkg(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Package != "firefox-esr" || got[0].Version != "128.3.0esr-1" || got[1].Package != "git" {
		t.Errorf("got %+v", got)
	}
	if got[0].Manager != appdb.Apt {
		t.Errorf("dpkg sources must be reported under apt, got %s", got[0].Manager)
	}
	if s, err := readDpkg(filepath.Join(dir, "missing")); s != nil || err != nil {
		t.Errorf("a missing status file is not an error: %v %v", s, err)
	}
}

func TestApkInstalled(t *testing.T) {
	db := "C:Q1abc\nP:firefox\nV:130.0-r0\nA:x86_64\nF:usr/bin\nR:firefox\nF:usr/share/applications\nR:firefox.desktop\n\nP:git\nV:2.45.2-r0\nF:usr/bin\nR:git\n"
	got := parseApkInstalled([]byte(db))
	if len(got) != 2 || got[0].Package != "firefox" || got[0].Version != "130.0-r0" || got[1].Package != "git" {
		t.Fatalf("got %+v", got)
	}
	if !got[0].Desktop || got[1].Desktop {
		t.Errorf("desktop flags: firefox=%v git=%v", got[0].Desktop, got[1].Desktop)
	}
}

func TestDesktopEntries(t *testing.T) {
	list := "/.\n/usr\n/usr/bin\n/usr/bin/firefox\n/usr/share/applications\n/usr/share/applications/firefox.desktop\n"
	if !listHasDesktop([]byte(list)) {
		t.Error("a desktop entry in the list was not seen")
	}
	if listHasDesktop([]byte("/usr/lib/x86_64-linux-gnu/libfoo.so.1\n/usr/share/doc/libfoo/copyright\n")) {
		t.Error("a library has no desktop entry")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "info"), 0o755)
	os.WriteFile(filepath.Join(dir, "info", "firefox.list"), []byte(list), 0o644)
	os.WriteFile(filepath.Join(dir, "info", "libfoo:amd64.list"), []byte("/usr/lib/libfoo.so\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "status"), []byte("Package: firefox\nStatus: install ok installed\nVersion: 1\n\nPackage: libfoo\nStatus: install ok installed\nVersion: 2\n"), 0o644)
	got, err := readDpkg(filepath.Join(dir, "status"))
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %+v", err, got)
	}
	if !got[0].Desktop || got[1].Desktop {
		t.Errorf("dpkg desktop flags: %+v", got)
	}
	// Unknown reporting: the library is matched but never reported; the application is.
	lib := Source{Manager: appdb.Apt, Package: "libfoo"}
	app := Source{Manager: appdb.Apt, Package: "some-gui", Desktop: true}
	if lib.reportable() || !app.reportable() || !(Source{Manager: appdb.Flatpak, Package: "x.y"}).reportable() {
		t.Error("reportable is wrong")
	}
	// Windows: redistributables and runtimes are not applications; everything else is.
	rt := Source{Manager: appdb.Registry, Name: "Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.40.33810"}
	real := Source{Manager: appdb.Registry, Name: "Internal Program Ltd v3"}
	if rt.reportable() || !real.reportable() {
		t.Error("windows runtime filter is wrong")
	}
}

func TestSnapYAMLVersion(t *testing.T) {
	if v := snapYAMLVersion([]byte("name: code\nversion: '1.93.1'\nsummary: x\n")); v != "1.93.1" {
		t.Errorf("got %q", v)
	}
}

// testDB is a small database with the shapes the planner has to handle.
func testDB(t *testing.T) *appdb.DB {
	t.Helper()
	fsys := fstest.MapFS{
		"apps/firefox.json": {Data: []byte(`{"id":"firefox","name":"Mozilla Firefox","category":"browser",
		  "windows":{"winget":["Mozilla.Firefox"],"registry":["Mozilla Firefox"],"paths":{"profiles":"%APPDATA%/Mozilla/Firefox"}},
		  "linux":{"pacman":["firefox"],"apt":["firefox","firefox-esr"],"flatpak":["org.mozilla.firefox"],"paths":{"profiles":"~/.mozilla/firefox"}},
		  "config":{"portability":"identical","exclude":["cache2"]}}`)},
		"apps/vlc.json": {Data: []byte(`{"id":"vlc","name":"VLC","category":"media-player",
		  "windows":{"winget":["VideoLAN.VLC"],"registry":["VLC media player"],"paths":{"config":"%APPDATA%/vlc"}},
		  "linux":{"pacman":["vlc"],"apt":["vlc"],"paths":{"config":"$XDG_CONFIG_HOME/vlc"}},
		  "config":{"portability":"translatable"}}`)},
		"apps/notepad-plus-plus.json": {Data: []byte(`{"id":"notepad-plus-plus","name":"Notepad++","category":"editor",
		  "windows":{"winget":["Notepad++.Notepad++"],"registry":["Notepad++"],"paths":{"config":"%APPDATA%/Notepad++"}},
		  "config":{"portability":"untranslatable"},"equivalents":["kate"]}`)},
		"apps/kate.json": {Data: []byte(`{"id":"kate","name":"Kate","category":"editor",
		  "windows":{"winget":["KDE.Kate"]},
		  "linux":{"pacman":["kate"],"apt":["kate"],"flatpak":["org.kde.kate"],"paths":{"config":"$XDG_CONFIG_HOME/katerc"}},
		  "config":{"portability":"identical"}}`)},
		"apps/spotify.json": {Data: []byte(`{"id":"spotify","name":"Spotify","category":"music",
		  "windows":{"winget":["Spotify.Spotify"]},
		  "linux":{"flatpak":["com.spotify.Client"],"snap":["spotify"]},
		  "config":{"portability":"none"}}`)},
		"apps/git.json": {Data: []byte(`{"id":"git","name":"Git","category":"vcs",
		  "windows":{"winget":["Git.Git"],"registry":["Git"],"paths":{"gitconfig":"%USERPROFILE%/.gitconfig"}},
		  "linux":{"pacman":["git"],"apt":["git"],"paths":{"gitconfig":"~/.gitconfig"}},
		  "config":{"portability":"identical"}}`)},
	}
	db, err := appdb.LoadFS(fsys, "apps")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

type fakeInfo struct {
	name string
	dir  bool
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() any           { return nil }

func fakeStat(existing map[string]bool) func(string) (os.FileInfo, error) {
	return func(p string) (os.FileInfo, error) {
		p = filepath.ToSlash(p)
		dir, ok := existing[p]
		if !ok {
			return nil, os.ErrNotExist
		}
		return fakeInfo{name: filepath.Base(p), dir: dir}, nil
	}
}

func TestDetectorMatchesAndFindsConfig(t *testing.T) {
	db := testDB(t)
	d := &Detector{
		DB:   db,
		Info: system.Info{OS: system.Linux, Home: "/home/u", Distro: "arch"},
		Env:  linuxEnv(nil),
		Probe: func() ([]Source, []appdb.Manager, []string) {
			return []Source{
				{Manager: appdb.Pacman, Package: "firefox", Version: "142.0-1"},
				{Manager: appdb.Flatpak, Package: "org.mozilla.firefox"},
				{Manager: appdb.Pacman, Package: "some-lib", Version: "1"},
				{Manager: appdb.Pacman, Package: "some-gui", Version: "2", Desktop: true},
				{Manager: appdb.Pacman, Package: "vlc"},
			}, []appdb.Manager{appdb.Pacman, appdb.Flatpak}, nil
		},
		Stat: fakeStat(map[string]bool{
			"/home/u/.mozilla/firefox":                              true,
			"/home/u/.var/app/org.mozilla.firefox/.mozilla/firefox": true,
			"/home/u/.gitconfig":                                    false, // git is not installed: config only
			"/home/u/.config/vlc":                                   true,
		}),
	}
	inv, err := d.Run()
	if err != nil {
		t.Fatal(err)
	}
	// The library without a desktop entry is not reported; the application is.
	if len(inv.Unknown) != 1 || inv.Unknown[0].Package != "some-gui" {
		t.Errorf("unknown = %+v", inv.Unknown)
	}
	ff := inv.Get("firefox")
	if ff == nil || len(ff.Sources) != 2 || ff.Version != "142.0-1" {
		t.Fatalf("firefox = %+v", ff)
	}
	if len(ff.Config) != 2 || ff.Config[0].Variant != Native || ff.Config[1].Variant != Flatpak {
		t.Errorf("firefox config = %+v", ff.Config)
	}
	if ff.Config[1].Root("firefox") != "apps/firefox/profiles@flatpak" {
		t.Errorf("root = %s", ff.Config[1].Root("firefox"))
	}
	g := inv.Get("git")
	if g == nil || !g.ConfigOnly || len(g.Config) != 1 || !g.Config[0].File {
		t.Errorf("git = %+v", g)
	}
	if inv.Installed() != 2 {
		t.Errorf("installed = %d", inv.Installed())
	}
	if inv.Get("kate") != nil {
		t.Error("kate has nothing on this system and must not appear")
	}

	// The scanner filter: the browser cache is left out, the rest passes.
	filter := inv.Filter(db)
	if filter("/home/u/.mozilla/firefox/abc.default/cache2", true) {
		t.Error("cache2 should be excluded")
	}
	if !filter("/home/u/.mozilla/firefox/abc.default/prefs.js", false) {
		t.Error("prefs.js should pass")
	}
	if !filter("/home/u/Documents/x", false) {
		t.Error("paths outside every location pass")
	}

	// Manifest round trip.
	m := NewManifest(inv, db.Len(), time.Unix(0, 0))
	b, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Format != ManifestFormat || len(back.Apps) != 3 || back.Get("firefox").Config[1].Variant != Flatpak {
		t.Errorf("round trip lost data: %+v", back)
	}
}

func TestRegistryAndScoopMatching(t *testing.T) {
	db := testDB(t)
	d := &Detector{DB: db, Info: system.Info{OS: system.Windows, Home: `C:\Users\u`}, Env: Env{OS: appdb.Windows, Home: `C:\Users\u`},
		Probe: func() ([]Source, []appdb.Manager, []string) {
			return []Source{
				{Manager: appdb.Registry, Package: "Mozilla Firefox (x64 en-US)", Name: "Mozilla Firefox (x64 en-US)", Version: "142.0.1"},
				{Manager: appdb.Winget, Package: "mozilla.firefox", Version: "142.0.1"},
				{Manager: appdb.Registry, Package: "GitHub Desktop", Name: "GitHub Desktop"},
			}, []appdb.Manager{appdb.Winget}, nil
		},
		Stat: fakeStat(nil),
	}
	inv, err := d.Run()
	if err != nil {
		t.Fatal(err)
	}
	if ff := inv.Get("firefox"); ff == nil || len(ff.Sources) != 2 {
		t.Errorf("firefox = %+v", ff)
	}
	if len(inv.Unknown) != 1 || inv.Unknown[0].Name != "GitHub Desktop" {
		t.Errorf("unknown = %+v", inv.Unknown)
	}
}

func windowsManifest() *Manifest {
	return &Manifest{Format: ManifestFormat, Inventory: Inventory{
		OS: appdb.Windows, Managers: []appdb.Manager{appdb.Winget},
		Apps: []Found{
			{ID: "firefox", Name: "Mozilla Firefox", Sources: []Source{{Manager: appdb.Winget, Package: "Mozilla.Firefox"}},
				Config: []Location{{Key: "profiles", Path: `C:\Users\u\AppData\Roaming\Mozilla\Firefox`}}},
			{ID: "vlc", Name: "VLC", Sources: []Source{{Manager: appdb.Registry, Package: "VLC media player"}},
				Config: []Location{{Key: "config", Path: `C:\Users\u\AppData\Roaming\vlc`}}},
			{ID: "notepad-plus-plus", Name: "Notepad++", Sources: []Source{{Manager: appdb.Winget, Package: "Notepad++.Notepad++"}},
				Config: []Location{{Key: "config", Path: `C:\Users\u\AppData\Roaming\Notepad++`}}},
			{ID: "spotify", Name: "Spotify", Sources: []Source{{Manager: appdb.Winget, Package: "Spotify.Spotify"}}},
			{ID: "git", Name: "Git", ConfigOnly: true, Config: []Location{{Key: "gitconfig", Path: `C:\Users\u\.gitconfig`, File: true}}},
		},
		Unknown: []Source{{Manager: appdb.Registry, Package: "Internal Program Ltd v3", Name: "Internal Program Ltd v3"}},
	}}
}

func TestPlanWindowsToArch(t *testing.T) {
	db := testDB(t)
	target := Target{OS: appdb.Linux, Distro: "arch", Managers: []appdb.Manager{appdb.Pacman}, Env: linuxEnv(nil),
		Installed: map[string]*Found{"git": {ID: "git", Sources: []Source{{Manager: appdb.Pacman, Package: "git"}}}}}
	p := BuildPlan(windowsManifest(), target, db)

	wantInstall := map[string]string{"firefox": "pacman", "vlc": "pacman", "kate": "pacman"}
	if len(p.Install) != len(wantInstall) {
		t.Fatalf("install = %+v", p.Install)
	}
	for _, it := range p.Install {
		if string(it.Manager) != wantInstall[it.ID] {
			t.Errorf("%s via %s", it.ID, it.Manager)
		}
		if it.ID == "kate" && (len(it.For) != 1 || it.For[0] != "notepad-plus-plus") {
			t.Errorf("kate should stand in for notepad-plus-plus: %+v", it)
		}
	}
	if len(p.NoSource) != 1 || p.NoSource[0].ID != "spotify" || len(p.NoSource[0].Needs) != 2 {
		t.Errorf("no source = %+v", p.NoSource)
	}
	if len(p.Present) != 0 {
		t.Errorf("git was config-only on the source; nothing to report present: %+v", p.Present)
	}
	if len(p.Unknown) != 1 {
		t.Errorf("unknown = %+v", p.Unknown)
	}

	cfg := map[string]ConfigItem{}
	for _, c := range p.Configs {
		cfg[c.ID+"/"+c.Key] = c
	}
	if c := cfg["firefox/profiles"]; c.Action != Place || c.Dest != filepath.FromSlash("/home/u/.mozilla/firefox") || c.Root != "apps/firefox/profiles" {
		t.Errorf("firefox profiles: %+v", c)
	}
	if c := cfg["vlc/config"]; c.Action != Aside || !strings.Contains(c.Reason, "translat") {
		t.Errorf("vlc config should be kept aside as translatable: %+v", c)
	}
	if c := cfg["notepad-plus-plus/config"]; c.Action != Aside {
		t.Errorf("notepad++ config: %+v", c)
	}
	// git is installed on the target, so its config-only file goes in place.
	if c := cfg["git/gitconfig"]; c.Action != Place || c.Dest != filepath.FromSlash("/home/u/.gitconfig") || !c.File {
		t.Errorf("gitconfig: %+v", c)
	}
	roots := p.AppRoots()
	if len(roots) != 2 || roots["apps/git/gitconfig"] == "" {
		t.Errorf("app roots = %+v", roots)
	}

	// One pacman command, with every package, marked batch; sudo or not depends on the test host.
	var pacman *Command
	for i := range p.Commands {
		if p.Commands[i].Manager == appdb.Pacman {
			pacman = &p.Commands[i]
		}
	}
	if pacman == nil || !pacman.Batch || len(pacman.Packages) != 3 {
		t.Fatalf("commands = %+v", p.Commands)
	}
	s := pacman.String()
	if !strings.Contains(s, "pacman -Syu --needed --noconfirm") || !strings.HasSuffix(s, "firefox vlc kate") {
		t.Errorf("command = %q", s)
	}
}

func TestPlanPrefersFlatpakLocationWhenInstalledThatWay(t *testing.T) {
	db := testDB(t)
	m := &Manifest{Format: ManifestFormat, Inventory: Inventory{OS: appdb.Linux, Apps: []Found{
		{ID: "firefox", Name: "Mozilla Firefox", Sources: []Source{{Manager: appdb.Pacman, Package: "firefox"}},
			Config: []Location{
				{Key: "profiles", Path: "/home/a/.mozilla/firefox"},
				{Key: "profiles", Variant: Flatpak, Path: "/home/a/.var/app/org.mozilla.firefox/.mozilla/firefox"},
			}},
	}}}
	target := Target{OS: appdb.Linux, Distro: "fedora", Managers: []appdb.Manager{appdb.Dnf, appdb.Flatpak}, Env: linuxEnv(nil),
		Installed: map[string]*Found{"firefox": {ID: "firefox", Sources: []Source{{Manager: appdb.Flatpak, Package: "org.mozilla.firefox"}}}}}
	p := BuildPlan(m, target, db)
	if len(p.Configs) != 2 {
		t.Fatalf("configs = %+v", p.Configs)
	}
	for _, c := range p.Configs {
		switch c.Variant {
		case Flatpak:
			if c.Action != Place || c.Dest != filepath.FromSlash("/home/u/.var/app/org.mozilla.firefox/.mozilla/firefox") {
				t.Errorf("flatpak copy: %+v", c)
			}
		default:
			if c.Action != Aside {
				t.Errorf("native copy should be kept aside: %+v", c)
			}
		}
	}
	// Same OS, translatable config: goes in place all the same.
	m2 := &Manifest{Inventory: Inventory{OS: appdb.Linux, Apps: []Found{{ID: "vlc", Name: "VLC",
		Sources: []Source{{Manager: appdb.Apt, Package: "vlc"}}, Config: []Location{{Key: "config", Path: "/home/a/.config/vlc"}}}}}}
	// The test database knows VLC through pacman and apt only, so the target must offer one of them.
	p2 := BuildPlan(m2, Target{OS: appdb.Linux, Managers: []appdb.Manager{appdb.Apt}, Env: linuxEnv(nil), Installed: map[string]*Found{}}, db)
	if len(p2.Install) != 1 || p2.Install[0].Package != "vlc" {
		t.Errorf("install = %+v", p2.Install)
	}
	if len(p2.Configs) != 1 || p2.Configs[0].Action != Place {
		t.Errorf("same-OS translatable config must be placed: %+v", p2.Configs)
	}
}

func TestPlanMissingAndNotInDatabase(t *testing.T) {
	db := testDB(t)
	m := &Manifest{Inventory: Inventory{OS: appdb.Linux, Apps: []Found{
		{ID: "kate", Name: "Kate", Sources: []Source{{Manager: appdb.Pacman, Package: "kate"}}},
		{ID: "ghost", Name: "Ghost", Sources: []Source{{Manager: appdb.Pacman, Package: "ghost"}}},
	}}}
	// Windows target with only scoop: Kate has winget only → no source.
	p := BuildPlan(m, Target{OS: appdb.Windows, Managers: []appdb.Manager{appdb.Scoop}, Env: Env{OS: appdb.Windows, Home: `C:\u`}, Installed: map[string]*Found{}}, db)
	if len(p.NoSource) != 1 || p.NoSource[0].ID != "kate" {
		t.Errorf("no source = %+v", p.NoSource)
	}
	if len(p.Missing) != 1 || p.Missing[0].ID != "ghost" || p.Missing[0].Reason != "not in this database" {
		t.Errorf("missing = %+v", p.Missing)
	}
}

func TestRunnerRetriesBatchOneByOne(t *testing.T) {
	var runs []string
	r := &Runner{Exec: func(_ context.Context, c Command) (int, error) {
		runs = append(runs, strings.Join(c.Argv, " "))
		for _, p := range c.Packages {
			if p == "bad" && len(c.Packages) >= 1 {
				return 1, errors.New("exit status 1")
			}
		}
		return 0, nil
	}}
	cmds := []Command{
		{Manager: appdb.Apt, Argv: []string{"apt-get", "update"}, Prep: true},
		{Manager: appdb.Apt, Argv: []string{"apt-get", "install", "-y", "good", "bad", "fine"}, Packages: []string{"good", "bad", "fine"}, Batch: true},
	}
	res := r.Run(context.Background(), cmds)
	if len(res) != 2 || !res[0].OK || res[1].OK {
		t.Fatalf("results = %+v", res)
	}
	if len(res[1].Failed) != 1 || res[1].Failed[0] != "bad" {
		t.Errorf("failed = %v", res[1].Failed)
	}
	// update, the batch, then three singles
	if len(runs) != 5 || runs[2] != "apt-get install -y good" || runs[3] != "apt-get install -y bad" {
		t.Errorf("runs = %v", runs)
	}

	// A failed preparatory step skips the manager's remaining commands.
	r2 := &Runner{Exec: func(_ context.Context, c Command) (int, error) {
		if c.Prep {
			return 100, errors.New("exit status 100")
		}
		return 0, nil
	}}
	res2 := r2.Run(context.Background(), cmds)
	if res2[1].OK || !strings.Contains(res2[1].Error, "skipped") || len(res2[1].Failed) != 3 {
		t.Errorf("after a failed prep: %+v", res2[1])
	}
}

func TestSnapAndWingetCommandsAreOnePerPackage(t *testing.T) {
	items := []InstallItem{
		{ID: "vscode", Manager: appdb.Snap, Package: "code --classic"},
		{ID: "spotify", Manager: appdb.Snap, Package: "spotify"},
	}
	cmds := Commands(items, Target{OS: appdb.Linux, Managers: []appdb.Manager{appdb.Snap}})
	if len(cmds) != 2 {
		t.Fatalf("commands = %+v", cmds)
	}
	if s := strings.Join(cmds[0].Argv, " "); !strings.HasSuffix(s, "snap install code --classic") {
		t.Errorf("classic flag lost: %q", s)
	}
	w := Commands([]InstallItem{{ID: "firefox", Manager: appdb.Winget, Package: "Mozilla.Firefox"}}, Target{OS: appdb.Windows, Managers: []appdb.Manager{appdb.Winget}})
	if len(w) != 1 || w[0].Argv[0] != "winget" || w[0].Argv[3] != "Mozilla.Firefox" {
		t.Errorf("winget = %+v", w)
	}
}

func TestAppRootRoundTrip(t *testing.T) {
	cases := []struct {
		id, key, variant string
		root             pack.Root
	}{
		{"firefox", "profiles", "", "apps/firefox/profiles"},
		{"firefox", "profiles", "flatpak", "apps/firefox/profiles@flatpak"},
		{"notepad-plus-plus", "config", "snap", "apps/notepad-plus-plus/config@snap"},
	}
	for _, c := range cases {
		r := pack.AppRoot(c.id, c.key, c.variant)
		if r != c.root {
			t.Errorf("AppRoot = %s, want %s", r, c.root)
		}
		id, key, variant, ok := r.SplitAppRoot()
		if !ok || id != c.id || key != c.key || variant != c.variant {
			t.Errorf("SplitAppRoot(%s) = %q %q %q %v", r, id, key, variant, ok)
		}
	}
	for _, bad := range []pack.Root{"documents", "apps/", "apps/x", "apps/x/", "other"} {
		if _, _, _, ok := bad.SplitAppRoot(); ok {
			t.Errorf("%q should not be an app root", bad)
		}
	}
}
