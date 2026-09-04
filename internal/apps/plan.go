package apps

import (
	"sort"
	"strings"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/system"
)

// Target is the system a package is being restored on, as far as applications are concerned.
type Target struct {
	OS       appdb.OS
	Distro   string
	Managers []appdb.Manager // available here, in order of preference
	// Installed is what detection found here, by id. Nil entries mean absent.
	Installed map[string]*Found
	Env       Env
	AURHelper string
}

// TargetFrom builds the target from the running system and its detection result.
func TargetFrom(info system.Info, inv *Inventory) Target {
	t := Target{OS: appdb.OS(info.OS), Distro: info.Distro, Managers: inv.Managers, Env: EnvFromSystem(info), AURHelper: AURHelper()}
	t.Installed = make(map[string]*Found, len(inv.Apps))
	for i := range inv.Apps {
		if !inv.Apps[i].ConfigOnly {
			t.Installed[inv.Apps[i].ID] = &inv.Apps[i]
		}
	}
	return t
}

// InstallItem is one application the plan installs.
type InstallItem struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Manager appdb.Manager `json:"manager"`
	Package string        `json:"package"`
	// For lists the source applications this one stands in for, when it is an equivalent rather
	// than the same application.
	For []string `json:"for,omitempty"`
}

// PresentItem is an application already installed on the target, so nothing is done for it.
type PresentItem struct {
	ID   string        `json:"id"`
	Name string        `json:"name"`
	Via  appdb.Manager `json:"via"`
	For  []string      `json:"for,omitempty"`
}

// NoSourceItem is an application that exists on this platform but that none of the available
// managers can install: the database only knows it through Flatpak, say, and Flatpak is missing.
type NoSourceItem struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Needs []appdb.Manager `json:"needs,omitempty"`
}

// MissingItem is an application with no version for this platform and no installable equivalent.
type MissingItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Equivalents []string `json:"equivalents,omitempty"` // listed in the database, none usable here
	Reason      string   `json:"reason"`
}

// ConfigAction says what happens to one configuration location.
type ConfigAction string

const (
	// Place: restored into the location the target platform uses for the same key.
	Place ConfigAction = "place"
	// Aside: kept under ~/DHS-restored/apps/<id>/<key>, with the reason.
	Aside ConfigAction = "aside"
)

// ConfigItem is the decision for one configuration location in the package.
type ConfigItem struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Key     string       `json:"key"`
	Variant Variant      `json:"variant,omitempty"`
	Root    pack.Root    `json:"root"`
	Dest    string       `json:"destination,omitempty"` // set when placed
	Action  ConfigAction `json:"action"`
	Reason  string       `json:"reason,omitempty"`
	File    bool         `json:"file,omitempty"`
}

// Plan is what DHS proposes for the applications of a package on this system. Building it touches
// nothing; it is the same whether the user then installs or not.
type Plan struct {
	SourceOS appdb.OS       `json:"source_os"`
	TargetOS appdb.OS       `json:"target_os"`
	Install  []InstallItem  `json:"install"`
	Present  []PresentItem  `json:"present"`
	NoSource []NoSourceItem `json:"no_source,omitempty"`
	Missing  []MissingItem  `json:"missing,omitempty"`
	Unknown  []Source       `json:"unknown,omitempty"`
	Configs  []ConfigItem   `json:"configs"`
	Commands []Command      `json:"commands"`
}

// Placed counts the configuration locations restored in place.
func (p *Plan) Placed() int {
	n := 0
	for _, c := range p.Configs {
		if c.Action == Place {
			n++
		}
	}
	return n
}

// BuildPlan decides, for every application in the manifest, what to do here.
func BuildPlan(m *Manifest, t Target, db *appdb.DB) *Plan {
	p := &Plan{SourceOS: m.OS, TargetOS: t.OS, Unknown: m.Unknown}
	// planned remembers what the plan installs or finds present, so two source applications
	// with the same equivalent (Notepad++ and Sublime Text, both → Kate) install it once.
	planned := map[string]*InstallItem{}
	present := map[string]*PresentItem{}

	pick := func(app *appdb.App) (appdb.Manager, string, []appdb.Manager) {
		pl := app.On(t.OS)
		var needs []appdb.Manager
		if pl == nil {
			return "", "", nil
		}
		for _, mgr := range t.Managers {
			if ids := pl.IDs(mgr); len(ids) > 0 && mgr.Installs() {
				return mgr, ids[0], nil
			}
		}
		for _, mgr := range appdb.Managers {
			if mgr.OS() == t.OS && mgr.Installs() && len(pl.IDs(mgr)) > 0 {
				needs = append(needs, mgr)
			}
		}
		return "", "", needs
	}

	for _, a := range m.Apps {
		if a.ConfigOnly {
			continue // the configuration travels; nothing is installed for a leftover
		}
		app := db.Get(a.ID)
		if app == nil {
			p.Missing = append(p.Missing, MissingItem{ID: a.ID, Name: a.Name, Reason: "not in this database"})
			continue
		}
		if f := t.Installed[a.ID]; f != nil {
			p.Present = append(p.Present, PresentItem{ID: a.ID, Name: a.Name, Via: firstManager(f)})
			continue
		}
		if app.On(t.OS) != nil {
			mgr, pkg, needs := pick(app)
			if mgr == "" {
				p.NoSource = append(p.NoSource, NoSourceItem{ID: a.ID, Name: a.Name, Needs: needs})
				continue
			}
			it := InstallItem{ID: a.ID, Name: app.Name, Manager: mgr, Package: pkg}
			p.Install = append(p.Install, it)
			planned[a.ID] = &p.Install[len(p.Install)-1]
			continue
		}
		// No version on this platform: the equivalents, in the database's order of preference.
		replaced := false
		for _, eid := range app.Equivalents {
			eq := db.Get(eid)
			if eq == nil || eq.On(t.OS) == nil {
				continue
			}
			if f := t.Installed[eid]; f != nil {
				if pi := present[eid]; pi != nil {
					pi.For = append(pi.For, a.ID)
				} else {
					p.Present = append(p.Present, PresentItem{ID: eid, Name: eq.Name, Via: firstManager(f), For: []string{a.ID}})
					present[eid] = &p.Present[len(p.Present)-1]
				}
				replaced = true
				break
			}
			if it := planned[eid]; it != nil {
				it.For = append(it.For, a.ID)
				replaced = true
				break
			}
			mgr, pkg, _ := pick(eq)
			if mgr == "" {
				continue
			}
			p.Install = append(p.Install, InstallItem{ID: eid, Name: eq.Name, Manager: mgr, Package: pkg, For: []string{a.ID}})
			planned[eid] = &p.Install[len(p.Install)-1]
			replaced = true
			break
		}
		if !replaced {
			reason := "no version for this platform and no equivalent listed"
			if len(app.Equivalents) > 0 {
				reason = "no version for this platform; none of the listed equivalents can be installed here"
			}
			p.Missing = append(p.Missing, MissingItem{ID: a.ID, Name: a.Name, Equivalents: app.Equivalents, Reason: reason})
		}
	}

	// Configuration: where each location goes, and why not when it does not.
	for _, a := range m.Apps {
		app := db.Get(a.ID)
		byKey := map[string][]Location{}
		var keys []string
		for _, l := range a.Config {
			if _, ok := byKey[l.Key]; !ok {
				keys = append(keys, l.Key)
			}
			byKey[l.Key] = append(byKey[l.Key], l)
		}
		sort.Strings(keys)
		for _, key := range keys {
			locs := byKey[key]
			chosen, dest, reason := placeConfig(app, a, key, locs, m.OS, t, planned)
			for i, l := range locs {
				it := ConfigItem{ID: a.ID, Name: a.Name, Key: key, Variant: l.Variant, Root: l.Root(a.ID), File: l.File}
				switch {
				case chosen == i:
					it.Action, it.Dest = Place, dest
				case chosen >= 0:
					it.Action, it.Reason = Aside, "another copy of this configuration is placed instead"
				default:
					it.Action, it.Reason = Aside, reason
				}
				p.Configs = append(p.Configs, it)
			}
		}
	}

	p.Commands = Commands(p.Install, t)
	return p
}

// placeConfig decides for one key of one application: which of the copies in the package (native,
// flatpak, snap) goes in place, where, or why none does. chosen is -1 when all are kept aside.
func placeConfig(app *appdb.App, a Found, key string, locs []Location, sourceOS appdb.OS, t Target, planned map[string]*InstallItem) (chosen int, dest, reason string) {
	if app == nil {
		return -1, "", "not in this database"
	}
	pl := app.On(t.OS)
	if pl == nil {
		return -1, "", "the application has no version for this platform"
	}
	path := pl.Paths[key]
	if path == "" {
		return -1, "", "no location for this configuration on this platform"
	}
	if sourceOS != t.OS {
		switch app.Config.Portability {
		case appdb.Identical:
		case appdb.Translatable:
			return -1, "", "the configuration needs translating between platforms; DHS does not translate"
		case appdb.Untranslatable:
			return -1, "", "the configuration is bound to the platform it came from"
		default:
			return -1, "", "the configuration does not travel between platforms"
		}
	}
	// How is the application present here, or how will it be?
	variant, sandbox := Native, ""
	switch {
	case t.Installed[a.ID] != nil:
		variant, sandbox = installedVariant(t.Installed[a.ID])
	case planned[a.ID] != nil:
		switch planned[a.ID].Manager {
		case appdb.Flatpak:
			variant, sandbox = Flatpak, planned[a.ID].Package
		case appdb.Snap:
			variant = Snap
			sandbox, _, _ = strings.Cut(planned[a.ID].Package, " ")
		}
	default:
		return -1, "", "the application is not installed here and is not being installed"
	}
	// Prefer the copy that came from the same kind of build; otherwise the native one; otherwise
	// the first. The files are the same either way — only the location differs.
	chosen = -1
	for i, l := range locs {
		if l.Variant == variant {
			chosen = i
			break
		}
	}
	if chosen < 0 {
		for i, l := range locs {
			if l.Variant == Native {
				chosen = i
				break
			}
		}
	}
	if chosen < 0 {
		chosen = 0
	}
	abs, ok := Resolve(path, t.Env, variant, sandbox)
	if !ok {
		return -1, "", "the location cannot be resolved on this system"
	}
	return chosen, abs, ""
}

// installedVariant says whether the application on the target is a native, Flatpak or Snap build.
// Native wins when several are present: it is what the desktop launches by default.
func installedVariant(f *Found) (Variant, string) {
	variant, sandbox := Variant("?"), ""
	for _, s := range f.Sources {
		switch s.Manager {
		case appdb.Flatpak:
			if variant == "?" {
				variant, sandbox = Flatpak, s.Package
			}
		case appdb.Snap:
			if variant == "?" {
				variant = Snap
				sandbox, _, _ = strings.Cut(s.Package, " ")
			}
		case appdb.Registry:
			// says nothing about the build
		default:
			return Native, ""
		}
	}
	if variant == "?" {
		return Native, ""
	}
	return variant, sandbox
}

func firstManager(f *Found) appdb.Manager {
	for _, s := range f.Sources {
		if s.Manager != appdb.Registry {
			return s.Manager
		}
	}
	if len(f.Sources) > 0 {
		return f.Sources[0].Manager
	}
	return ""
}

// AppRoots returns, for every configuration location the plan places, its package root and
// destination — what the restore needs to register before building its own plan.
func (p *Plan) AppRoots() map[pack.Root]string {
	out := make(map[pack.Root]string, len(p.Configs))
	for _, c := range p.Configs {
		if c.Action == Place {
			out[c.Root] = c.Dest
		}
	}
	return out
}
