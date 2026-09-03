//go:build windows

package system

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func detect() (Info, error) {
	i := Info{OS: Windows, Arch: Arch(), Name: "Windows"}

	// %USERPROFILE% takes precedence, for the same reason as $HOME on Linux.
	i.Home, _ = os.UserHomeDir()
	if u, err := user.Current(); err == nil {
		// Username comes as "DOMAIN\\user"; we only care about the user part.
		i.User = u.Username
		if _, after, ok := strings.Cut(u.Username, `\`); ok {
			i.User = after
		}
		if i.Home == "" {
			i.Home = u.HomeDir
		}
	}
	if h, err := os.Hostname(); err == nil {
		i.Hostname = h
	}

	// The marketing name and the version live in the registry. If they are missing, we keep "Windows".
	if k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	); err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("ProductName"); err == nil && v != "" {
			i.Name = v
		}
		// DisplayVersion is "23H2"; ReleaseId is the old form, "2009".
		if v, _, err := k.GetStringValue("DisplayVersion"); err == nil && v != "" {
			i.Version = v
		} else if v, _, err := k.GetStringValue("ReleaseId"); err == nil && v != "" {
			i.Version = v
		}
		// Windows 11 still reports itself as "Windows 10" in ProductName; build >= 22000 gives it away.
		if b, _, err := k.GetStringValue("CurrentBuildNumber"); err == nil {
			if buildAtLeast(b, 22000) && strings.Contains(i.Name, "Windows 10") {
				i.Name = strings.Replace(i.Name, "Windows 10", "Windows 11", 1)
			}
		}
	}
	return i, nil
}

func buildAtLeast(build string, min int) bool {
	n := 0
	for _, r := range build {
		if r < '0' || r > '9' {
			return false
		}
		n = n*10 + int(r-'0')
	}
	return n >= min
}

// knownFolder ties each standard location to its Windows GUID. We never assume
// "C:\Users\X\Documents": the folders may be redirected to OneDrive or to another partition.
var knownFolder = map[Kind]*windows.KNOWNFOLDERID{
	Documents: windows.FOLDERID_Documents,
	Pictures:  windows.FOLDERID_Pictures,
	Videos:    windows.FOLDERID_Videos,
	Music:     windows.FOLDERID_Music,
	Downloads: windows.FOLDERID_Downloads,
	Desktop:   windows.FOLDERID_Desktop,
	Config:    windows.FOLDERID_RoamingAppData,
}

// fallbackName is used only if the system call fails.
var fallbackName = map[Kind]string{
	Documents: "Documents",
	Pictures:  "Pictures",
	Videos:    "Videos",
	Music:     "Music",
	Downloads: "Downloads",
	Desktop:   "Desktop",
	Config:    `AppData\Roaming`,
}

func locations(i Info) []Location {
	out := make([]Location, 0, len(KindOrder))
	for _, k := range KindOrder {
		path := ""
		if id, ok := knownFolder[k]; ok {
			if p, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT); err == nil {
				path = p
			}
		}
		if path == "" {
			path = filepath.Join(i.Home, fallbackName[k])
		}
		st, err := os.Stat(path)
		out = append(out, Location{Kind: k, Path: path, Exists: err == nil && st.IsDir()})
	}
	return out
}
