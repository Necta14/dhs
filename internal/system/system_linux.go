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

	if u, err := user.Current(); err == nil {
		i.User = u.Username
		i.Home = u.HomeDir
	}
	if i.Home == "" {
		i.Home, _ = os.UserHomeDir()
	}
	if h, err := os.Hostname(); err == nil {
		i.Hostname = h
	}

	// /etc/os-release e standardul freedesktop; îl are orice distribuție modernă.
	if rel, err := parseOSRelease("/etc/os-release"); err == nil {
		if n := rel["NAME"]; n != "" {
			i.Name = n
		}
		// Arch și derivatele n-au VERSION_ID; VERSION lipsește și el. Rămâne gol, e în regulă.
		if v := rel["VERSION_ID"]; v != "" {
			i.Version = v
		} else if v := rel["VERSION"]; v != "" {
			i.Version = v
		}
	}
	return i, nil
}

// parseOSRelease citește formatul cheie=valoare din os-release, cu ghilimele opționale.
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

// xdgFallback e numele implicit al fiecărui loc, când user-dirs.dirs lipsește.
var xdgFallback = map[Kind]string{
	Documents: "Documents",
	Pictures:  "Pictures",
	Videos:    "Videos",
	Music:     "Music",
	Downloads: "Downloads",
	Desktop:   "Desktop",
}

// xdgVar e numele variabilei din ~/.config/user-dirs.dirs pentru fiecare loc.
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

// readUserDirs citește ~/.config/user-dirs.dirs, unde locurile sunt localizate
// („Documente", „Imagini" pe un sistem în română). Fără el am rata jumătate din profil.
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
		// Formatul folosește „$HOME/Documents"; îl rezolvăm noi, nu shell-ul.
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
