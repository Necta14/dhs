package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/system"
)

// Source is one sighting of an application by one package manager.
type Source struct {
	Manager appdb.Manager `json:"via"`
	Package string        `json:"package"` // the identifier under that manager, as found
	Version string        `json:"version,omitempty"`
	Name    string        `json:"name,omitempty"` // display name, when the manager has one
}

// Location is one configuration location of an application that exists on this system.
type Location struct {
	Key     string  `json:"key"`
	Variant Variant `json:"variant,omitempty"`
	Path    string  `json:"path"`
	File    bool    `json:"file,omitempty"` // a single file rather than a directory
}

// Root is the package root the location's files are stored under.
func (l Location) Root(id string) pack.Root { return pack.AppRoot(id, l.Key, string(l.Variant)) }

// Found is an application recognised in the database.
type Found struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Version  string   `json:"version,omitempty"`
	Sources  []Source `json:"sources,omitempty"`
	// ConfigOnly: no package manager reports it, but its configuration is on disk — a portable
	// build, an AppImage, or a leftover. The configuration is carried; nothing is proposed for
	// installation on its account.
	ConfigOnly bool       `json:"config_only,omitempty"`
	Config     []Location `json:"config,omitempty"`
}

// Inventory is the result of detection on one system.
type Inventory struct {
	OS     appdb.OS `json:"os"`
	Distro string   `json:"distro,omitempty"`
	// Managers are the ones that can install on this system, in the order DHS prefers them.
	Managers []appdb.Manager `json:"managers"`
	Apps     []Found         `json:"apps"`
	// Unknown are installed applications the database does not know. Reported, never guessed.
	Unknown []Source `json:"unknown,omitempty"`
	// Errors are managers that could not be queried; detection went on without them.
	Errors []string `json:"errors,omitempty"`
}

// Installed counts the applications a package manager actually reported.
func (inv *Inventory) Installed() int {
	n := 0
	for _, a := range inv.Apps {
		if !a.ConfigOnly {
			n++
		}
	}
	return n
}

// Get returns the found application with this id, or nil.
func (inv *Inventory) Get(id string) *Found {
	for i := range inv.Apps {
		if inv.Apps[i].ID == id {
			return &inv.Apps[i]
		}
	}
	return nil
}

// Manifest is dhs/apps.json inside the package: the inventory of the source system, plus what is
// needed to interpret it later. It lives encrypted, in the first volume.
type Manifest struct {
	Format   int       `json:"format"`
	Created  time.Time `json:"created"`
	Database int       `json:"database_entries"`
	Inventory
}

// ManifestFormat is bumped when the structure changes incompatibly.
const ManifestFormat = 1

// ManifestPath is the entry path under pack.RootMeta.
const ManifestPath = "apps.json"

// ErrNoManifest says the package carries no application manifest: made with --no-apps, or by a
// DHS from before applications existed.
var ErrNoManifest = errors.New("the package has no application manifest")

// NewManifest wraps an inventory for writing into the package.
func NewManifest(inv *Inventory, dbEntries int, now time.Time) *Manifest {
	return &Manifest{Format: ManifestFormat, Created: now.UTC(), Database: dbEntries, Inventory: *inv}
}

// Encode serialises the manifest, indented so that a curious user can read it out of a package.
func (m *Manifest) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeManifest parses apps.json.
func DecodeManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("apps.json: %w", err)
	}
	if m.Format > ManifestFormat {
		return nil, fmt.Errorf("apps.json: format %d is newer than this DHS understands (%d)", m.Format, ManifestFormat)
	}
	return &m, nil
}

// ReadManifest extracts dhs/apps.json from a package into memory. The manifest is small and sits in
// the first volume, so this is cheap.
func ReadManifest(ctx context.Context, r *pack.Reader) (*Manifest, error) {
	var target *pack.Entry
	for _, e := range r.Index.Entries {
		if e.Root == pack.RootMeta && e.Path == ManifestPath {
			target = e
			break
		}
	}
	if target == nil {
		return nil, ErrNoManifest
	}
	sink := &memSink{}
	rep, err := r.Extract(ctx, pack.ExtractOptions{
		Filter: func(e *pack.Entry) bool { return e == target },
		Open:   func(*pack.Entry) (pack.Sink, error) { return sink, nil },
	})
	if err != nil {
		return nil, err
	}
	if len(rep.Failed) > 0 {
		return nil, fmt.Errorf("apps.json: %v", rep.Failed[0].Err)
	}
	return DecodeManifest(sink.Bytes())
}

type memSink struct{ bytes.Buffer }

func (s *memSink) Commit() error { return nil }
func (s *memSink) Abort() error  { s.Reset(); return nil }

// EnvFromSystem builds the path-resolution environment for the running system.
func EnvFromSystem(info system.Info) Env {
	return Env{OS: appdb.OS(info.OS), Home: info.Home, Get: os.Getenv}
}

// Detector runs detection. The zero value is not usable; use Detect or fill DB, Info and Env.
type Detector struct {
	DB   *appdb.DB
	Info system.Info
	Env  Env
	// Probe overrides the platform's package-manager query — tests use it. It returns the
	// installed packages, the managers available for installing, and the managers that failed.
	Probe func() (sources []Source, managers []appdb.Manager, errs []string)
	// Stat overrides os.Stat — tests use it to fake configuration directories.
	Stat func(string) (os.FileInfo, error)
}

// Detect is the one-call form: what is installed here, matched against the database, with the
// configuration locations that exist.
func Detect(db *appdb.DB, info system.Info) (*Inventory, error) {
	d := &Detector{DB: db, Info: info, Env: EnvFromSystem(info)}
	return d.Run()
}

// Run performs the detection.
func (d *Detector) Run() (*Inventory, error) {
	if d.DB == nil {
		return nil, errors.New("apps: no database")
	}
	probe := d.Probe
	if probe == nil {
		probe = func() ([]Source, []appdb.Manager, []string) { return probeInstalled(d.Info) }
	}
	stat := d.Stat
	if stat == nil {
		stat = os.Stat
	}
	os := appdb.OS(d.Info.OS)
	inv := &Inventory{OS: os, Distro: d.Info.Distro}

	sources, managers, errs := probe()
	inv.Managers = managers
	inv.Errors = errs

	// 1. Match what the managers report against the database.
	byID := map[string]*Found{}
	var order []string
	for _, s := range sources {
		app := d.match(s)
		if app == nil {
			inv.Unknown = append(inv.Unknown, s)
			continue
		}
		f := byID[app.ID]
		if f == nil {
			f = &Found{ID: app.ID, Name: app.Name, Category: app.Category}
			byID[app.ID] = f
			order = append(order, app.ID)
		}
		f.Sources = append(f.Sources, s)
		if f.Version == "" {
			f.Version = s.Version
		}
	}

	// 2. Configuration that exists on disk, for every application the database knows on this
	// platform — detected or not. A sandboxed build's files live in its own home, so those are
	// looked up for the sandboxes the application was actually seen in.
	for _, app := range d.DB.Apps() {
		pl := app.On(os)
		if pl == nil || len(pl.Paths) == 0 {
			continue
		}
		f := byID[app.ID]
		var locs []Location
		for _, key := range sortedKeys(pl.Paths) {
			p := pl.Paths[key]
			for _, c := range d.candidates(f) {
				abs, ok := Resolve(p, d.Env, c.variant, c.sandbox)
				if !ok {
					continue
				}
				st, err := stat(abs)
				if err != nil {
					continue
				}
				locs = append(locs, Location{Key: key, Variant: c.variant, Path: abs, File: !st.IsDir()})
			}
		}
		if len(locs) == 0 {
			continue
		}
		if f == nil {
			f = &Found{ID: app.ID, Name: app.Name, Category: app.Category, ConfigOnly: true}
			byID[app.ID] = f
			order = append(order, app.ID)
		}
		f.Config = locs
	}

	sort.Strings(order)
	for _, id := range order {
		inv.Apps = append(inv.Apps, *byID[id])
	}
	sort.Slice(inv.Unknown, func(i, j int) bool {
		a, b := inv.Unknown[i], inv.Unknown[j]
		if a.Manager != b.Manager {
			return a.Manager < b.Manager
		}
		return strings.ToLower(a.Package) < strings.ToLower(b.Package)
	})
	return inv, nil
}

type candidate struct {
	variant Variant
	sandbox string
}

// candidates lists where an application's configuration may live: natively, and in every
// sandbox it was seen in.
func (d *Detector) candidates(f *Found) []candidate {
	out := []candidate{{Native, ""}}
	if f == nil {
		return out
	}
	for _, s := range f.Sources {
		switch s.Manager {
		case appdb.Flatpak:
			out = append(out, candidate{Flatpak, s.Package})
		case appdb.Snap:
			name, _, _ := strings.Cut(s.Package, " ")
			out = append(out, candidate{Snap, name})
		}
	}
	return out
}

// match finds the database entry behind one sighting. Managers that share a database (rpm on
// Fedora and openSUSE) are tried under both names; scoop packages are tried with and without
// their bucket.
func (d *Detector) match(s Source) *appdb.App {
	switch s.Manager {
	case appdb.Registry:
		return d.DB.ByRegistryName(s.Name)
	case appdb.Dnf, appdb.Zypper:
		if a := d.DB.ByPackage(appdb.Dnf, s.Package); a != nil {
			return a
		}
		return d.DB.ByPackage(appdb.Zypper, s.Package)
	case appdb.Pacman:
		if a := d.DB.ByPackage(appdb.Pacman, s.Package); a != nil {
			return a
		}
		return d.DB.ByPackage(appdb.AUR, s.Package)
	case appdb.Scoop:
		if a := d.DB.ByPackage(appdb.Scoop, s.Package); a != nil {
			return a
		}
		if _, name, ok := strings.Cut(s.Package, "/"); ok {
			return d.DB.ByPackage(appdb.Scoop, name)
		}
		return nil
	}
	return d.DB.ByPackage(s.Manager, s.Package)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Filter builds the per-path filter the scanner applies while capturing configuration: inside a
// location, the entry's exclusion patterns decide; elsewhere everything passes. Locations are
// tried longest first, so a nested one wins over the one that contains it.
func (inv *Inventory) Filter(db *appdb.DB) func(path string, dir bool) bool {
	type loc struct {
		path string
		app  *appdb.App
	}
	var locs []loc
	for _, f := range inv.Apps {
		app := db.Get(f.ID)
		if app == nil {
			continue
		}
		for _, l := range f.Config {
			if !l.File {
				locs = append(locs, loc{l.Path, app})
			}
		}
	}
	sort.Slice(locs, func(i, j int) bool { return len(locs[i].path) > len(locs[j].path) })
	return func(path string, dir bool) bool {
		for _, l := range locs {
			rel, ok := Under(path, l.path)
			if !ok {
				continue
			}
			return rel == "." || !l.app.Excluded(rel)
		}
		return true
	}
}
