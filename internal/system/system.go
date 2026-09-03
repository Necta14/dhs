// Package system detects the operating system and the standard locations in the user's profile.
//
// Code specific to one operating system lives exclusively in the files suffixed _linux.go and
// _windows.go. Thanks to build tags, the Linux binary contains not a single byte of the Windows
// code, and vice versa — that keeps the product small, as Rule #4 demands.
package system

import (
	"fmt"
	"runtime"
)

// OS is the operating system we are running on, or the one a package came from.
type OS string

const (
	Linux   OS = "linux"
	Windows OS = "windows"
)

// Info describes the current system, as much as the manifest and the restore plan need.
type Info struct {
	OS       OS     `json:"os"`
	Name     string `json:"name"`    // "Arch Linux", "Windows 11 Pro"
	Version  string `json:"version"` // "rolling", "23H2"
	Arch     string `json:"arch"`    // amd64, arm64
	User     string `json:"user"`
	Home     string `json:"home"`
	Hostname string `json:"hostname"`
}

func (i Info) String() string {
	v := i.Name
	if i.Version != "" {
		v += " " + i.Version
	}
	return fmt.Sprintf("%s (%s)", v, i.Arch)
}

// Kind names the type of a standard location in the profile.
type Kind string

const (
	Documents Kind = "documents"
	Pictures  Kind = "pictures"
	Videos    Kind = "videos"
	Music     Kind = "music"
	Downloads Kind = "downloads"
	Desktop   Kind = "desktop"
	Config    Kind = "config"
)

// KindOrder is the order in which locations are shown to the user — most likely wanted first.
var KindOrder = []Kind{Documents, Pictures, Videos, Music, Downloads, Desktop, Config}

// Location is a standard location in the profile, resolved to a concrete path.
type Location struct {
	Kind Kind   `json:"kind"`
	Path string `json:"path"`
	// Exists says whether the directory actually exists on disk. A fresh profile does not have them all.
	Exists bool `json:"exists"`
}

// Detect finds out what we are running on. The implementation is per operating system.
func Detect() (Info, error) { return detect() }

// Locations returns the standard locations in the user's profile, in KindOrder.
// Missing locations are included, with Exists false, so the interface can grey them out.
func Locations(i Info) []Location { return locations(i) }

// Arch normalises the architecture to the form used in the manifest.
func Arch() string { return runtime.GOARCH }
