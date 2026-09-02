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

	if u, err := user.Current(); err == nil {
		// Username vine ca „DOMENIU\\user"; ne interesează doar partea de user.
		i.User = u.Username
		if _, after, ok := strings.Cut(u.Username, `\`); ok {
			i.User = after
		}
		i.Home = u.HomeDir
	}
	if i.Home == "" {
		i.Home, _ = os.UserHomeDir()
	}
	if h, err := os.Hostname(); err == nil {
		i.Hostname = h
	}

	// Numele comercial și versiunea stau în registry. Dacă lipsesc, rămânem cu „Windows".
	if k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	); err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("ProductName"); err == nil && v != "" {
			i.Name = v
		}
		// DisplayVersion e „23H2"; ReleaseId e forma veche, „2009".
		if v, _, err := k.GetStringValue("DisplayVersion"); err == nil && v != "" {
			i.Version = v
		} else if v, _, err := k.GetStringValue("ReleaseId"); err == nil && v != "" {
			i.Version = v
		}
		// Windows 11 se raportează tot ca „Windows 10" în ProductName; build ≥ 22000 îl trădează.
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

// knownFolder leagă fiecare loc standard de GUID-ul lui din Windows. Nu presupunem niciodată
// „C:\Users\X\Documents": folderele pot fi redirecționate către OneDrive sau altă partiție.
var knownFolder = map[Kind]*windows.KNOWNFOLDERID{
	Documents: windows.FOLDERID_Documents,
	Pictures:  windows.FOLDERID_Pictures,
	Videos:    windows.FOLDERID_Videos,
	Music:     windows.FOLDERID_Music,
	Downloads: windows.FOLDERID_Downloads,
	Desktop:   windows.FOLDERID_Desktop,
	Config:    windows.FOLDERID_RoamingAppData,
}

// fallbackName e folosit doar dacă apelul de sistem eșuează.
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
