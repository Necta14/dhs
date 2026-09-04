// Package appdb is the application database: which applications DHS knows, under which
// identifiers each platform's package managers know them, where they keep their configuration,
// and what stands in for them on a platform where they do not exist.
//
// The database is a set of JSON files, one per application, in apps/. They are embedded in the
// binary, so DHS answers every question offline; contributions arrive as pull requests. The data
// is licensed CC-BY-4.0 (see LICENSE in this directory), separately from the code.
//
// Nothing here guesses. An application is recognised only through an exact identifier from this
// database; whatever is not in it is reported as unknown (Rule #2).
package appdb

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed apps/*.json
var files embed.FS

// OS names a platform section of an entry. The values match internal/system, but appdb stays a
// plain data package with no dependency on the rest of DHS.
type OS string

const (
	Windows OS = "windows"
	Linux   OS = "linux"
)

// Manager is a way of installing, and of detecting, an application.
type Manager string

const (
	Winget   Manager = "winget"
	Choco    Manager = "choco"
	Scoop    Manager = "scoop"
	Registry Manager = "registry" // Windows "Apps & features": detection only, nothing installs through it
	Pacman   Manager = "pacman"
	AUR      Manager = "aur" // through paru or yay; pacman lists what they install
	Apt      Manager = "apt"
	Dnf      Manager = "dnf"
	Zypper   Manager = "zypper"
	Apk      Manager = "apk"
	Flatpak  Manager = "flatpak"
	Snap     Manager = "snap"
)

// Managers lists every manager, in a fixed order, for reports and validation.
var Managers = []Manager{Winget, Choco, Scoop, Registry, Pacman, AUR, Apt, Dnf, Zypper, Apk, Flatpak, Snap}

// OSOf says which platform a manager belongs to.
func (m Manager) OS() OS {
	switch m {
	case Winget, Choco, Scoop, Registry:
		return Windows
	}
	return Linux
}

// Installs says whether the manager can install anything. The Windows registry only detects.
func (m Manager) Installs() bool { return m != Registry }

// Portability says how far an application's configuration travels between operating systems.
type Portability string

const (
	// Identical: the same files, in the same layout, on every platform. Restored in place.
	Identical Portability = "identical"
	// Translatable: the same settings in a different form. v1 does not translate; the files are
	// kept aside under DHS-restored and reported.
	Translatable Portability = "translatable"
	// Untranslatable: bound to the platform. Kept aside on a different OS, restored on the same one.
	Untranslatable Portability = "untranslatable"
	// None: nothing worth carrying. The entry has no paths.
	None Portability = "none"
)

// Platform is the section of an entry for one operating system.
type Platform struct {
	// Windows
	Winget   []string `json:"winget,omitempty"`
	Choco    []string `json:"choco,omitempty"`
	Scoop    []string `json:"scoop,omitempty"` // "bucket/name"; the main bucket may be omitted
	Registry []string `json:"registry,omitempty"`

	// Linux
	Pacman  []string `json:"pacman,omitempty"`
	AUR     []string `json:"aur,omitempty"`
	Apt     []string `json:"apt,omitempty"`
	Dnf     []string `json:"dnf,omitempty"`
	Zypper  []string `json:"zypper,omitempty"`
	Apk     []string `json:"apk,omitempty"`
	Flatpak []string `json:"flatpak,omitempty"`
	Snap    []string `json:"snap,omitempty"` // "name" or "name --classic"

	// Paths are the configuration locations, keyed by a name that is the same on every platform,
	// so that "profile" on Windows and "profile" on Linux are known to be the same thing.
	Paths map[string]string `json:"paths,omitempty"`
}

// IDs returns the identifiers under which the platform knows the application for one manager.
func (p *Platform) IDs(m Manager) []string {
	if p == nil {
		return nil
	}
	switch m {
	case Winget:
		return p.Winget
	case Choco:
		return p.Choco
	case Scoop:
		return p.Scoop
	case Registry:
		return p.Registry
	case Pacman:
		return p.Pacman
	case AUR:
		return p.AUR
	case Apt:
		return p.Apt
	case Dnf:
		return p.Dnf
	case Zypper:
		return p.Zypper
	case Apk:
		return p.Apk
	case Flatpak:
		return p.Flatpak
	case Snap:
		return p.Snap
	}
	return nil
}

// Installable says whether at least one manager on this platform can install the application.
func (p *Platform) Installable() bool {
	if p == nil {
		return false
	}
	for _, m := range Managers {
		if m.Installs() && len(p.IDs(m)) > 0 {
			return true
		}
	}
	return false
}

// Config is the policy for the application's configuration, shared by every platform.
type Config struct {
	Portability Portability `json:"portability"`
	// Exclude lists what inside the configuration locations is not worth carrying: caches, logs,
	// lock files. A pattern without "/" matches any path segment; one with "/" matches the whole
	// relative path. Matching is case-insensitive.
	Exclude []string `json:"exclude,omitempty"`
	Notes   string   `json:"notes,omitempty"`
}

// App is one entry of the database.
type App struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Summary  string `json:"summary,omitempty"`
	Category string `json:"category"`
	Homepage string `json:"homepage,omitempty"`
	License  string `json:"license,omitempty"`

	Windows *Platform `json:"windows,omitempty"`
	Linux   *Platform `json:"linux,omitempty"`

	Config Config `json:"config"`
	// Equivalents are the ids of other entries that stand in for this one on a platform where
	// it does not exist, in order of preference. Curated by people, never guessed.
	Equivalents []string `json:"equivalents,omitempty"`
}

// On returns the platform section for an operating system, or nil.
func (a *App) On(os OS) *Platform {
	switch os {
	case Windows:
		return a.Windows
	case Linux:
		return a.Linux
	}
	return nil
}

// Categories is the closed list an entry may use. Closed on purpose: a free-text category
// fragments the report into "editor", "Editor" and "text editor".
var Categories = []string{
	"browser", "email", "chat", "video-call", "feed-reader",
	"office", "pdf", "notes", "ebook", "education", "science", "finance", "productivity",
	"editor", "ide", "terminal", "shell", "devtool", "vcs", "database", "virtualization", "container",
	"media-player", "music", "video-editor", "audio-editor", "graphics", "photo", "3d", "cad",
	"screenshot", "screen-recorder", "streaming",
	"gaming", "launcher", "emulator",
	"password-manager", "security", "vpn", "network", "remote-desktop",
	"file-manager", "file-sync", "cloud-storage", "backup", "archive", "download", "torrent",
	"system", "utility", "fonts", "accessibility", "other",
}

var categorySet = func() map[string]bool {
	m := make(map[string]bool, len(Categories))
	for _, c := range Categories {
		m[c] = true
	}
	return m
}()

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// allowedPrefixes are the only ways a configuration path may start. Anything else — an absolute
// path, a drive letter, another variable — is a mistake, because it would not resolve on the
// user's machine.
var allowedPrefixes = map[OS][]string{
	Linux:   {"~/", "$XDG_CONFIG_HOME/", "$XDG_DATA_HOME/", "$XDG_STATE_HOME/"},
	Windows: {"%APPDATA%/", "%LOCALAPPDATA%/", "%USERPROFILE%/"},
}

// Validate checks one entry and returns every problem found, as messages for the contributor.
func Validate(a *App) []string {
	var p []string
	bad := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if !idRe.MatchString(a.ID) {
		bad("id %q: lowercase letters, digits and hyphens only, starting with a letter or digit", a.ID)
	}
	if strings.TrimSpace(a.Name) == "" {
		bad("name is missing")
	}
	if !categorySet[a.Category] {
		bad("category %q is not in the list: %s", a.Category, strings.Join(Categories, ", "))
	}
	if a.Homepage != "" && !strings.HasPrefix(a.Homepage, "https://") && !strings.HasPrefix(a.Homepage, "http://") {
		bad("homepage %q is not a URL", a.Homepage)
	}
	if a.Windows == nil && a.Linux == nil {
		bad("neither a windows nor a linux section")
	}
	switch a.Config.Portability {
	case Identical, Translatable, Untranslatable, None:
	default:
		bad("config.portability %q: identical, translatable, untranslatable or none", a.Config.Portability)
	}

	paths := 0
	for _, os := range []OS{Windows, Linux} {
		pl := a.On(os)
		if pl == nil {
			continue
		}
		for _, m := range Managers {
			ids := pl.IDs(m)
			if len(ids) > 0 && m.OS() != os {
				bad("%s: %s belongs to the %s section", os, m, m.OS())
			}
			for _, id := range ids {
				if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
					bad("%s.%s: empty identifier or stray spaces in %q", os, m, id)
				}
				if m != Snap && m != Registry && strings.ContainsAny(id, " \t") {
					bad("%s.%s: %q contains spaces", os, m, id)
				}
			}
		}
		if !pl.Installable() && len(pl.Registry) == 0 && len(pl.Paths) == 0 {
			bad("%s: the section is empty", os)
		}
		for key, pth := range pl.Paths {
			paths++
			if !idRe.MatchString(key) {
				bad("%s.paths: key %q: lowercase letters, digits and hyphens only", os, key)
			}
			ok := false
			for _, pre := range allowedPrefixes[os] {
				if strings.HasPrefix(pth, pre) && len(pth) > len(pre) {
					ok = true
				}
			}
			if !ok {
				bad("%s.paths.%s: %q must start with one of %s", os, key, pth, strings.Join(allowedPrefixes[os], " "))
			}
			if strings.Contains(pth, `\`) || strings.Contains(pth, "//") || strings.HasSuffix(pth, "/") || strings.Contains(pth, "/../") || strings.Contains(pth, "/./") {
				bad("%s.paths.%s: %q: forward slashes, no trailing slash, no . or ..", os, key, pth)
			}
		}
	}
	if a.Config.Portability == None && paths > 0 {
		bad("config.portability is none but there are paths")
	}
	if a.Config.Portability != None && paths == 0 {
		bad("config.portability is %s but there are no paths; use none", a.Config.Portability)
	}
	if a.Config.Portability == Identical && a.Windows != nil && a.Linux != nil && len(a.Windows.Paths) > 0 && len(a.Linux.Paths) > 0 {
		shared := 0
		for k := range a.Windows.Paths {
			if _, ok := a.Linux.Paths[k]; ok {
				shared++
			}
		}
		if shared == 0 {
			bad("config.portability is identical but windows.paths and linux.paths share no key")
		}
	}
	for _, x := range a.Config.Exclude {
		if x == "" || strings.HasPrefix(x, "/") || strings.HasSuffix(x, "/") {
			bad("config.exclude: %q: a relative pattern, without leading or trailing slash", x)
		}
		if _, err := path.Match(x, ""); err != nil {
			bad("config.exclude: %q: %v", x, err)
		}
	}
	seen := map[string]bool{}
	for _, e := range a.Equivalents {
		if e == a.ID {
			bad("equivalents: an entry cannot be its own equivalent")
		}
		if seen[e] {
			bad("equivalents: %q listed twice", e)
		}
		seen[e] = true
	}
	return p
}

// DB is the loaded database, indexed for the questions detection and planning ask.
type DB struct {
	apps  []*App
	byID  map[string]*App
	byPkg map[pkgKey]*App
	// byRegistry holds the lower-cased Apps & features names, longest first.
	byRegistry []registryName
}

type pkgKey struct {
	m  Manager
	id string // lower-cased
}

type registryName struct {
	name string
	app  *App
}

var (
	once     sync.Once
	embedded *DB
	embedErr error
)

// Load returns the database embedded in the binary. It is parsed once per process.
func Load() (*DB, error) {
	once.Do(func() { embedded, embedErr = LoadFS(files, "apps") })
	return embedded, embedErr
}

// LoadFS reads every *.json under dir and validates the set. Tests and tools use it on a
// directory of their own. Entries that do not validate are reported together, not one at a time.
func LoadFS(fsys fs.FS, dir string) (*DB, error) {
	names, err := fs.Glob(fsys, path.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	db := &DB{byID: make(map[string]*App, len(names)), byPkg: make(map[pkgKey]*App, len(names)*4)}
	var problems []string
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var a App
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&a); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		base := strings.TrimSuffix(path.Base(name), ".json")
		if a.ID != base {
			problems = append(problems, fmt.Sprintf("%s: id %q does not match the file name", name, a.ID))
		}
		for _, p := range Validate(&a) {
			problems = append(problems, fmt.Sprintf("%s: %s", name, p))
		}
		if _, dup := db.byID[a.ID]; dup {
			problems = append(problems, fmt.Sprintf("%s: id %q appears twice", name, a.ID))
			continue
		}
		app := a
		db.byID[app.ID] = &app
		db.apps = append(db.apps, &app)
	}
	// Cross-entry checks: equivalents must exist, identifiers must not be claimed twice.
	for _, a := range db.apps {
		for _, e := range a.Equivalents {
			if db.byID[e] == nil {
				problems = append(problems, fmt.Sprintf("%s: equivalent %q is not in the database", a.ID, e))
			}
		}
		for _, os := range []OS{Windows, Linux} {
			pl := a.On(os)
			if pl == nil {
				continue
			}
			for _, m := range Managers {
				if m == Registry {
					continue
				}
				for _, id := range pl.IDs(m) {
					k := pkgKey{m, strings.ToLower(snapName(m, id))}
					if other, taken := db.byPkg[k]; taken && other != a {
						problems = append(problems, fmt.Sprintf("%s: %s %q is already claimed by %s", a.ID, m, id, other.ID))
						continue
					}
					db.byPkg[k] = a
				}
			}
			for _, n := range pl.Registry {
				db.byRegistry = append(db.byRegistry, registryName{strings.ToLower(n), a})
			}
		}
	}
	sort.Slice(db.byRegistry, func(i, j int) bool { return len(db.byRegistry[i].name) > len(db.byRegistry[j].name) })
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return db, nil
}

// ValidationError carries every problem found while loading, so a contributor fixes them in one go.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	if len(e.Problems) == 1 {
		return "appdb: " + e.Problems[0]
	}
	return fmt.Sprintf("appdb: %d problems:\n  %s", len(e.Problems), strings.Join(e.Problems, "\n  "))
}

// snapName strips the install flags a snap identifier may carry ("code --classic" → "code").
func snapName(m Manager, id string) string {
	if m != Snap {
		return id
	}
	name, _, _ := strings.Cut(id, " ")
	return name
}

// Len is the number of entries.
func (d *DB) Len() int { return len(d.apps) }

// Apps returns every entry, sorted by id.
func (d *DB) Apps() []*App { return d.apps }

// Get returns the entry with this id, or nil.
func (d *DB) Get(id string) *App { return d.byID[id] }

// ByPackage finds the application a manager knows under this identifier. Case does not matter:
// winget and flatpak ids are compared case-insensitively by their own tools.
func (d *DB) ByPackage(m Manager, id string) *App {
	return d.byPkg[pkgKey{m, strings.ToLower(snapName(m, id))}]
}

// ByRegistryName finds the application behind a Windows "Apps & features" display name. The
// database holds the name without version or architecture suffixes; the installed one may carry
// them: "Mozilla Firefox (x64 en-US)" is matched by "Mozilla Firefox". A name is matched whole, or
// as a prefix followed by a space or a parenthesis, never in the middle of a word — so "Git" does
// not claim "GitHub Desktop".
func (d *DB) ByRegistryName(display string) *App {
	name := strings.ToLower(strings.TrimSpace(display))
	for _, r := range d.byRegistry {
		if name == r.name || strings.HasPrefix(name, r.name+" ") || strings.HasPrefix(name, r.name+"(") {
			return r.app
		}
	}
	return nil
}

// Excluded says whether a path relative to a configuration location is left out by the entry's
// exclusion patterns. Segments and whole paths are compared in lower case.
func (a *App) Excluded(rel string) bool {
	if len(a.Config.Exclude) == 0 {
		return false
	}
	rel = strings.ToLower(strings.Trim(strings.ReplaceAll(rel, `\`, "/"), "/"))
	segs := strings.Split(rel, "/")
	for _, pat := range a.Config.Exclude {
		pat = strings.ToLower(pat)
		if strings.Contains(pat, "/") {
			if ok, _ := path.Match(pat, rel); ok {
				return true
			}
			continue
		}
		for _, s := range segs {
			if ok, _ := path.Match(pat, s); ok {
				return true
			}
		}
	}
	return false
}
