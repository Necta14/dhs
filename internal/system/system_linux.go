//go:build linux

package system

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

func detect() (Info, error) {
	i := Info{OS: Linux, Arch: Arch(), Name: "Linux"}

	// $HOME takes precedence over /etc/passwd: that is how every system tool behaves, and it is
	// how DHS can be run against a synthetic profile in tests without touching the real one.
	i.Home, _ = os.UserHomeDir()
	if u, err := user.Current(); err == nil {
		i.User = u.Username
		if i.Home == "" {
			i.Home = u.HomeDir
		}
	}
	if h, err := os.Hostname(); err == nil {
		i.Hostname = h
	}

	// /etc/os-release is the freedesktop standard; every modern distribution has it.
	if rel, err := parseOSRelease("/etc/os-release"); err == nil {
		if n := rel["NAME"]; n != "" {
			i.Name = n
		}
		i.Distro = rel["ID"]
		if like := strings.Fields(rel["ID_LIKE"]); len(like) > 0 {
			i.Like = like
		}
		// Arch and its derivatives have no VERSION_ID; VERSION is missing too. It stays empty, which is fine.
		if v := rel["VERSION_ID"]; v != "" {
			i.Version = v
		} else if v := rel["VERSION"]; v != "" {
			i.Version = v
		}
	}
	return i, nil
}

// parseOSRelease reads the key=value format of os-release, with optional quotes.
func parseOSRelease(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]string, 16)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return out, sc.Err()
}

// xdgFallback is the default name of each location, used when user-dirs.dirs is missing.
var xdgFallback = map[Kind]string{
	Documents: "Documents",
	Pictures:  "Pictures",
	Videos:    "Videos",
	Music:     "Music",
	Downloads: "Downloads",
	Desktop:   "Desktop",
}

// xdgVar is the name of the variable in ~/.config/user-dirs.dirs for each location.
var xdgVar = map[Kind]string{
	Documents: "XDG_DOCUMENTS_DIR",
	Pictures:  "XDG_PICTURES_DIR",
	Videos:    "XDG_VIDEOS_DIR",
	Music:     "XDG_MUSIC_DIR",
	Downloads: "XDG_DOWNLOAD_DIR",
	Desktop:   "XDG_DESKTOP_DIR",
}

func locations(i Info) []Location {
	dirs := readUserDirs(filepath.Join(configHome(i.Home), "user-dirs.dirs"), i.Home)

	out := make([]Location, 0, len(KindOrder))
	for _, k := range KindOrder {
		var path string
		switch {
		case k == Config:
			path = configHome(i.Home)
		case dirs[k] != "":
			path = dirs[k]
		default:
			path = filepath.Join(i.Home, xdgFallback[k])
		}
		st, err := os.Stat(path)
		out = append(out, Location{Kind: k, Path: path, Exists: err == nil && st.IsDir()})
	}
	return out
}

func configHome(home string) string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(home, ".config")
}

// readUserDirs reads ~/.config/user-dirs.dirs, where the locations are localised
// ("Documente", "Imagini" on a Romanian system). Without it we would miss half the profile.
func readUserDirs(path, home string) map[Kind]string {
	out := make(map[Kind]string, len(xdgVar))
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	byVar := make(map[string]Kind, len(xdgVar))
	for k, v := range xdgVar {
		byVar[v] = k
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		kind, known := byVar[strings.TrimSpace(key)]
		if !known {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		// The format uses "$HOME/Documents"; we resolve it ourselves, not the shell.
		if after, found := strings.CutPrefix(value, "$HOME/"); found {
			value = filepath.Join(home, after)
		} else if value == "$HOME" {
			value = home
		}
		if value != "" {
			out[kind] = value
		}
	}
	return out
}
