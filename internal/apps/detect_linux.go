//go:build linux

package apps

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/system"
)

// probeInstalled asks every package database present on this system what it holds. It reads
// files where the format is plain text (pacman, dpkg, apk, flatpak, snap) and runs the tool only
// where the database is binary (rpm). A manager that fails is reported and skipped; the others
// still count.
func probeInstalled(info system.Info) ([]Source, []appdb.Manager, []string) {
	var out []Source
	var errs []string
	add := func(s []Source, err error, name string) {
		out = append(out, s...)
		if err != nil {
			errs = append(errs, name+": "+err.Error())
		}
	}
	if s, err := readPacman("/var/lib/pacman/local"); s != nil || err != nil {
		add(s, err, "pacman")
	}
	if s, err := readDpkg("/var/lib/dpkg/status"); s != nil || err != nil {
		add(s, err, "dpkg")
	}
	if _, err := os.Stat("/var/lib/rpm"); err == nil {
		s, err := runRPM(rpmManager(info))
		add(s, err, "rpm")
	}
	if s, err := readApk("/lib/apk/db/installed"); s != nil || err != nil {
		add(s, err, "apk")
	}
	add(readFlatpak([]string{"/var/lib/flatpak/app", filepath.Join(info.Home, ".local/share/flatpak/app")}), nil, "flatpak")
	add(readSnap([]string{"/snap", "/var/lib/snapd/snap"}), nil, "snap")
	return out, availableManagers(info), errs
}

// availableManagers lists what can install here, native manager first, then the sandboxes, then
// an AUR helper. The native one is chosen by the distribution family, not by whichever binary
// happens to exist: Debian ships a pacman package too.
func availableManagers(info system.Info) []appdb.Manager {
	has := func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }
	var out []appdb.Manager
	switch family(info) {
	case "arch":
		if has("pacman") {
			out = append(out, appdb.Pacman)
		}
	case "debian":
		if has("apt-get") {
			out = append(out, appdb.Apt)
		}
	case "fedora":
		if has("dnf") {
			out = append(out, appdb.Dnf)
		}
	case "suse":
		if has("zypper") {
			out = append(out, appdb.Zypper)
		}
	case "alpine":
		if has("apk") {
			out = append(out, appdb.Apk)
		}
	default:
		// An unknown distribution: take the first native manager that exists.
		for _, c := range []struct {
			bin string
			m   appdb.Manager
		}{{"pacman", appdb.Pacman}, {"apt-get", appdb.Apt}, {"dnf", appdb.Dnf}, {"zypper", appdb.Zypper}, {"apk", appdb.Apk}} {
			if has(c.bin) {
				out = append(out, c.m)
				break
			}
		}
	}
	if has("flatpak") {
		out = append(out, appdb.Flatpak)
	}
	if has("snap") {
		out = append(out, appdb.Snap)
	}
	if family(info) == "arch" && (has("paru") || has("yay")) {
		out = append(out, appdb.AUR)
	}
	return out
}

// AURHelper returns the helper to use for AUR packages, or "".
func AURHelper() string {
	for _, h := range []string{"paru", "yay"} {
		if _, err := exec.LookPath(h); err == nil {
			return h
		}
	}
	return ""
}

// isAdmin says whether installing needs no elevation.
func isAdmin() bool { return os.Geteuid() == 0 }

// elevator returns the command that grants root for one command, or "" when none is available.
func elevator() string {
	if isAdmin() {
		return ""
	}
	for _, e := range []string{"sudo", "doas", "run0"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return ""
}
