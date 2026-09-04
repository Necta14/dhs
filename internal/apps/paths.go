// Package apps detects the applications installed on a system, matches them against the database,
// captures their configuration into the package and, on the other side, plans what to install and
// where each configuration goes. Nothing here guesses: an application is known only through an
// identifier from appdb, and everything else is reported as unknown.
package apps

import (
	"path/filepath"
	"strings"

	"github.com/Necta14/dhs/appdb"
)

// Env is what path resolution needs to know about the user. Tests build it by hand; the
// commands fill it from the environment through EnvFromSystem.
type Env struct {
	OS   appdb.OS
	Home string
	// Get returns an environment variable, or "" — os.Getenv in production.
	Get func(string) string
}

func (e Env) get(name string) string {
	if e.Get == nil {
		return ""
	}
	return e.Get(name)
}

// Variant names the sandbox a configuration location belongs to. The native build keeps its
// files where the database says; a Flatpak or Snap build keeps them inside its own home.
type Variant string

const (
	Native  Variant = ""
	Flatpak Variant = "flatpak"
	Snap    Variant = "snap"
)

// Resolve turns a database path ("$XDG_CONFIG_HOME/Code/User", "%APPDATA%/Code/User") into an
// absolute path on this system. sandbox is the Flatpak application id or the snap name when the
// location belongs to a sandboxed build, and "" otherwise. ok is false for a path the database
// should not have accepted.
func Resolve(p string, env Env, variant Variant, sandbox string) (string, bool) {
	prefix, rest, found := strings.Cut(p, "/")
	if !found || rest == "" {
		return "", false
	}
	rest = filepath.FromSlash(rest)
	home := env.Home
	if home == "" {
		return "", false
	}

	switch env.OS {
	case appdb.Linux:
		var base string
		switch variant {
		case Flatpak:
			// A Flatpak sees ~/.var/app/<id> as its XDG base: config/, data/, .local/state/.
			app := filepath.Join(home, ".var", "app", sandbox)
			switch prefix {
			case "~":
				base = app
			case "$XDG_CONFIG_HOME":
				base = filepath.Join(app, "config")
			case "$XDG_DATA_HOME":
				base = filepath.Join(app, "data")
			case "$XDG_STATE_HOME":
				base = filepath.Join(app, ".local", "state")
			default:
				return "", false
			}
		case Snap:
			// A snap's home is ~/snap/<name>/current, with the usual dot directories inside.
			app := filepath.Join(home, "snap", sandbox, "current")
			switch prefix {
			case "~":
				base = app
			case "$XDG_CONFIG_HOME":
				base = filepath.Join(app, ".config")
			case "$XDG_DATA_HOME":
				base = filepath.Join(app, ".local", "share")
			case "$XDG_STATE_HOME":
				base = filepath.Join(app, ".local", "state")
			default:
				return "", false
			}
		default:
			switch prefix {
			case "~":
				base = home
			case "$XDG_CONFIG_HOME":
				base = envOr(env, "XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			case "$XDG_DATA_HOME":
				base = envOr(env, "XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
			case "$XDG_STATE_HOME":
				base = envOr(env, "XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
			default:
				return "", false
			}
		}
		return filepath.Join(base, rest), true

	case appdb.Windows:
		if variant != Native {
			return "", false
		}
		var base string
		switch prefix {
		case "%USERPROFILE%":
			base = home
		case "%APPDATA%":
			base = envOr(env, "APPDATA", filepath.Join(home, "AppData", "Roaming"))
		case "%LOCALAPPDATA%":
			base = envOr(env, "LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
		default:
			return "", false
		}
		return filepath.Join(base, rest), true
	}
	return "", false
}

func envOr(env Env, name, fallback string) string {
	if v := env.get(name); v != "" {
		return v
	}
	return fallback
}

// Under reports whether abs lies under dir (or is dir), and the relative path with "/".
func Under(abs, dir string) (string, bool) {
	abs, dir = filepath.Clean(abs), filepath.Clean(dir)
	if abs == dir {
		return ".", true
	}
	if strings.HasPrefix(abs, dir+string(filepath.Separator)) {
		return filepath.ToSlash(abs[len(dir)+1:]), true
	}
	return "", false
}
