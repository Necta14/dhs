//go:build windows

package apps

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/system"
)

// probeInstalled reads what "Apps & features" shows — the Uninstall keys, for every user and
// both architectures — then asks winget for its identifiers, and lists what scoop and choco
// keep on disk. The registry is always there; the rest is added when present.
func probeInstalled(info system.Info) ([]Source, []appdb.Manager, []string) {
	var out []Source
	var errs []string
	out = append(out, readUninstallKeys()...)
	if _, err := exec.LookPath("winget"); err == nil {
		s, err := runWingetExport()
		out = append(out, s...)
		if err != nil {
			errs = append(errs, "winget: "+err.Error())
		}
	}
	out = append(out, readScoop(info)...)
	out = append(out, readChoco()...)
	return out, availableManagers(), errs
}

func availableManagers() []appdb.Manager {
	var out []appdb.Manager
	for _, c := range []struct {
		bin string
		m   appdb.Manager
	}{{"winget", appdb.Winget}, {"choco", appdb.Choco}, {"scoop", appdb.Scoop}} {
		if _, err := exec.LookPath(c.bin); err == nil {
			out = append(out, c.m)
		}
	}
	return out
}

// AURHelper has no meaning on Windows.
func AURHelper() string { return "" }

var uninstallKeys = []struct {
	root registry.Key
	path string
	flag uint32
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.WOW64_64KEY},
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.WOW64_32KEY},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, 0},
}

var kbUpdate = regexp.MustCompile(`^(KB|Update for|Security Update for|Hotfix for)`)

// readUninstallKeys returns one Source per visible entry, skipping what the Settings app also
// hides: system components, updates and patches.
func readUninstallKeys() []Source {
	var out []Source
	seen := map[string]bool{}
	for _, uk := range uninstallKeys {
		k, err := registry.OpenKey(uk.root, uk.path, registry.READ|uk.flag)
		if err != nil {
			continue
		}
		names, err := k.ReadSubKeyNames(-1)
		if err != nil {
			k.Close()
			continue
		}
		for _, n := range names {
			sub, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			s, ok := readUninstallEntry(sub)
			sub.Close()
			if !ok {
				continue
			}
			key := strings.ToLower(s.Name + "\x00" + s.Version)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, s)
		}
		k.Close()
	}
	return out
}

func readUninstallEntry(k registry.Key) (Source, bool) {
	name, _, err := k.GetStringValue("DisplayName")
	name = strings.TrimSpace(name)
	if err != nil || name == "" {
		return Source{}, false
	}
	if v, _, err := k.GetIntegerValue("SystemComponent"); err == nil && v == 1 {
		return Source{}, false
	}
	if p, _, err := k.GetStringValue("ParentKeyName"); err == nil && p != "" {
		return Source{}, false
	}
	if r, _, err := k.GetStringValue("ReleaseType"); err == nil && (r == "Update" || r == "Hotfix" || r == "Security Update") {
		return Source{}, false
	}
	if kbUpdate.MatchString(name) {
		return Source{}, false
	}
	ver, _, _ := k.GetStringValue("DisplayVersion")
	return Source{Manager: appdb.Registry, Package: name, Name: name, Version: strings.TrimSpace(ver)}, true
}

// wingetExport is the shape of `winget export`: sources, each with its packages.
type wingetExport struct {
	Sources []struct {
		Packages []struct {
			PackageIdentifier string `json:"PackageIdentifier"`
			Version           string `json:"Version"`
		} `json:"Packages"`
	} `json:"Sources"`
}

// runWingetExport asks winget which of the installed applications it knows. The table output of
// `winget list` truncates names and ids; the export file is JSON and complete.
func runWingetExport() ([]Source, error) {
	tmp, err := os.CreateTemp("", "dhs-winget-*.json")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "winget", "export", "-o", path, "--include-versions",
		"--accept-source-agreements", "--disable-interactivity")
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	// winget exits non-zero when some installed apps are not in any source; the file is still written.
	_ = cmd.Run()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var exp wingetExport
	if err := json.Unmarshal(b, &exp); err != nil {
		return nil, err
	}
	var out []Source
	for _, src := range exp.Sources {
		for _, p := range src.Packages {
			if p.PackageIdentifier != "" {
				out = append(out, Source{Manager: appdb.Winget, Package: p.PackageIdentifier, Version: p.Version})
			}
		}
	}
	return out, nil
}

// readScoop lists <scoop>/apps/<name>/current: the bucket comes from install.json, the version
// from manifest.json.
func readScoop(info system.Info) []Source {
	root := os.Getenv("SCOOP")
	if root == "" {
		root = filepath.Join(info.Home, "scoop")
	}
	entries, err := os.ReadDir(filepath.Join(root, "apps"))
	if err != nil {
		return nil
	}
	var out []Source
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "scoop" {
			continue
		}
		cur := filepath.Join(root, "apps", e.Name(), "current")
		s := Source{Manager: appdb.Scoop, Package: e.Name()}
		var inst struct {
			Bucket string `json:"bucket"`
		}
		if b, err := os.ReadFile(filepath.Join(cur, "install.json")); err == nil && json.Unmarshal(b, &inst) == nil && inst.Bucket != "" {
			s.Package = inst.Bucket + "/" + e.Name()
		}
		var man struct {
			Version string `json:"version"`
		}
		if b, err := os.ReadFile(filepath.Join(cur, "manifest.json")); err == nil && json.Unmarshal(b, &man) == nil {
			s.Version = man.Version
		}
		out = append(out, s)
	}
	return out
}

var nuspecVersion = regexp.MustCompile(`<version>([^<]+)</version>`)

// readChoco lists %ProgramData%\chocolatey\lib\<name>\<name>.nuspec.
func readChoco() []Source {
	root := os.Getenv("ChocolateyInstall")
	if root == "" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			return nil
		}
		root = filepath.Join(pd, "chocolatey")
	}
	entries, err := os.ReadDir(filepath.Join(root, "lib"))
	if err != nil {
		return nil
	}
	var out []Source
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s := Source{Manager: appdb.Choco, Package: e.Name()}
		if b, err := os.ReadFile(filepath.Join(root, "lib", e.Name(), e.Name()+".nuspec")); err == nil {
			if m := nuspecVersion.FindSubmatch(b); m != nil {
				s.Version = strings.TrimSpace(string(m[1]))
			}
		}
		out = append(out, s)
	}
	return out
}

// isAdmin says whether the process runs elevated.
func isAdmin() bool {
	var sid *windows.SID
	if err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid); err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	return err == nil && member
}

// elevator: Windows has no sudo to prepend; winget and choco raise UAC prompts on their own.
func elevator() string { return "" }
