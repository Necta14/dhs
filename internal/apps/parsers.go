package apps

// The parsers for the package databases that are plain text. They carry no build tag: the formats
// are the same wherever the file is read, and the tests run on every platform.

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/system"
)

// family maps os-release ID and ID_LIKE to the package-manager family.
func family(info system.Info) string {
	ids := append([]string{info.Distro}, info.Like...)
	for _, id := range ids {
		switch strings.ToLower(id) {
		case "arch", "archlinux", "manjaro", "endeavouros", "cachyos", "garuda", "artix":
			return "arch"
		case "debian", "ubuntu", "linuxmint", "pop", "elementary", "zorin", "kali", "raspbian", "neon", "mx":
			return "debian"
		case "fedora", "rhel", "centos", "rocky", "almalinux", "nobara", "bazzite", "silverblue":
			return "fedora"
		case "opensuse", "opensuse-tumbleweed", "opensuse-leap", "sles", "suse":
			return "suse"
		case "alpine", "postmarketos":
			return "alpine"
		}
	}
	return ""
}

func rpmManager(info system.Info) appdb.Manager {
	if family(info) == "suse" {
		return appdb.Zypper
	}
	return appdb.Dnf
}

// readPacman reads /var/lib/pacman/local/<pkg>/desc: %NAME% and %VERSION% blocks.
func readPacman(dir string) ([]Source, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Source
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name(), "desc"))
		if err != nil {
			continue
		}
		s := parsePacmanDesc(b)
		if s.Package != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func parsePacmanDesc(b []byte) Source {
	s := Source{Manager: appdb.Pacman}
	sc := bufio.NewScanner(bytes.NewReader(b))
	section := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			section = ""
		case strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%"):
			section = line
		case section == "%NAME%" && s.Package == "":
			s.Package = line
		case section == "%VERSION%" && s.Version == "":
			s.Version = line
		}
	}
	return s
}

// readDpkg parses /var/lib/dpkg/status: stanzas separated by blank lines, one package each.
func readDpkg(path string) ([]Source, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return parseDpkgStatus(f), nil
}

func parseDpkgStatus(r *os.File) []Source {
	var out []Source
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var cur Source
	installed := false
	flush := func() {
		if cur.Package != "" && installed {
			cur.Manager = appdb.Apt
			out = append(out, cur)
		}
		cur, installed = Source{}, false
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "Package":
			cur.Package = val
		case "Version":
			cur.Version = val
		case "Status":
			installed = strings.HasSuffix(val, " installed")
		}
	}
	flush()
	return out
}

// runRPM queries the rpm database, which is binary (SQLite or BerkeleyDB) and not worth parsing.
func runRPM(m appdb.Manager) ([]Source, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rpm", "-qa", "--qf", "%{NAME}\t%{VERSION}\n")
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, line := range strings.Split(string(b), "\n") {
		name, ver, _ := strings.Cut(line, "\t")
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, Source{Manager: m, Package: name, Version: strings.TrimSpace(ver)})
		}
	}
	return out, nil
}

// readApk parses /lib/apk/db/installed: stanzas of single-letter fields, P: name, V: version.
func readApk(path string) ([]Source, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseApkInstalled(b), nil
}

func parseApkInstalled(b []byte) []Source {
	var out []Source
	var cur Source
	flush := func() {
		if cur.Package != "" {
			cur.Manager = appdb.Apk
			out = append(out, cur)
		}
		cur = Source{}
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		if len(line) < 2 || line[1] != ':' {
			continue
		}
		switch line[0] {
		case 'P':
			cur.Package = line[2:]
		case 'V':
			cur.Version = line[2:]
		}
	}
	flush()
	return out
}

// readFlatpak lists the application ids deployed system-wide and per user: one directory each.
func readFlatpak(dirs []string) []Source {
	var out []Source
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			id := e.Name()
			if !e.IsDir() || seen[id] || !strings.Contains(id, ".") {
				continue
			}
			seen[id] = true
			out = append(out, Source{Manager: appdb.Flatpak, Package: id})
		}
	}
	return out
}

// snapRuntimes are snaps that are not applications: bases, the daemon, themes, runtimes.
func snapRuntime(name string) bool {
	switch name {
	case "bin", "README", "snapd", "bare", "core", "core18", "core20", "core22", "core24", "core26":
		return true
	}
	for _, p := range []string{"gtk-common-themes", "gnome-3-", "gnome-4", "gnome-42", "gnome-46", "kde-frameworks", "kf5-", "kf6-", "mesa-", "cups", "ffmpeg-2"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// readSnap lists mounted snaps by directory name and reads the version from snap.yaml.
func readSnap(dirs []string) []Source {
	var out []Source
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if snapRuntime(name) || strings.HasPrefix(name, ".") {
				continue
			}
			if st, err := os.Stat(filepath.Join(dir, name)); err != nil || !st.IsDir() {
				continue
			}
			s := Source{Manager: appdb.Snap, Package: name}
			if b, err := os.ReadFile(filepath.Join(dir, name, "current", "meta", "snap.yaml")); err == nil {
				s.Version = snapYAMLVersion(b)
			}
			out = append(out, s)
		}
		if len(out) > 0 {
			break // /snap and /var/lib/snapd/snap are the same thing on different distributions
		}
	}
	return out
}

func snapYAMLVersion(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "version:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
