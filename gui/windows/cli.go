//go:build windows

// Everything this interface knows about DHS it learns by running dhs.exe and reading its --json
// output. No packing, no crypto, no filesystem rules here: decision D9 keeps all of that in the
// core, and the interface is a shell over it.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// createNoWindow keeps a console from flashing up every time we call the binary.
const createNoWindow = 0x08000000

// findBinary looks for dhs.exe next to this executable first, so the pair can be carried on a
// stick, then falls back to $DHS_BIN and the PATH.
func findBinary() (string, error) {
	if p := os.Getenv("DHS_BIN"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), "dhs.exe")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	if p, err := exec.LookPath("dhs.exe"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("dhs"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("dhs.exe not found. Put it next to this program, or set DHS_BIN.")
}

// run calls the binary and returns stdout. stderr comes back inside the error, because that is
// where the core writes the message a user needs to read.
func run(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
}

// ---- the shapes the CLI emits ------------------------------------------------------------------

type sysInfo struct {
	OS       string `json:"os"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Arch     string `json:"arch"`
	User     string `json:"user"`
	Home     string `json:"home"`
	Hostname string `json:"hostname"`
}

func (s sysInfo) String() string {
	n := s.Name
	if s.Version != "" {
		n += " " + s.Version
	}
	return fmt.Sprintf("%s (%s)", n, s.Arch)
}

type bucket struct {
	Files int64 `json:"files"`
	Bytes int64 `json:"bytes"`
}

type skipped struct {
	Reason string `json:"reason"`
	Files  int64  `json:"files"`
	Bytes  int64  `json:"bytes"`
}

type inventory struct {
	Roots   []string          `json:"roots"`
	Files   int64             `json:"files"`
	Bytes   int64             `json:"bytes"`
	ByClass map[string]bucket `json:"by_class"`
	Skipped []skipped         `json:"skipped"`
	Secrets bucket            `json:"secrets"`
	Errors  []string          `json:"errors"`
}

type estimate struct {
	Level    int   `json:"level"`
	Raw      int64 `json:"raw"`
	Min      int64 `json:"min"`
	Max      int64 `json:"max"`
	Volumes  int   `json:"volumes"`
	Duration int64 `json:"duration_ns"`
	Sampled  bool  `json:"sampled"`
}

type volume struct {
	Path  string `json:"path"`
	FS    string `json:"fs"`
	Total int64  `json:"total"`
	Free  int64  `json:"free"`
}

type scanOut struct {
	System    sysInfo    `json:"system"`
	Inventory *inventory `json:"inventory"`
	Estimate  estimate   `json:"estimate"`
	Dest      *volume    `json:"destination"`
	Fits      *bool      `json:"fits"`
}

type manifest struct {
	Format   int    `json:"format"`
	Created  string `json:"created"`
	Cipher   string `json:"cipher"`
	Level    int    `json:"level"`
	Volumes  int    `json:"volumes"`
	Files    int64  `json:"files"`
	Raw      int64  `json:"raw_bytes"`
	Stored   int64  `json:"stored_bytes"`
	Complete bool   `json:"complete"`
	Tool     string `json:"tool"`
	Source   struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Arch    string `json:"arch"`
	} `json:"source"`
}

type backupOut struct {
	Package  string   `json:"package"`
	Manifest manifest `json:"manifest"`
	Files    int64    `json:"files"`
	Bytes    int64    `json:"raw_bytes"`
	Stored   int64    `json:"stored_bytes"`
	Dedup    int64    `json:"dedup_bytes"`
	Duration int64    `json:"duration_ns"`
}

type rootSummary struct {
	Root  string `json:"Root"`
	Dest  string `json:"Dest"`
	Files int    `json:"Files"`
	Bytes int64  `json:"Bytes"`
}

type restoreItem struct {
	Path   string `json:"path"`
	Dest   string `json:"destination"`
	Action string `json:"action"`
	Note   string `json:"note"`
	Bytes  int64  `json:"bytes"`
}

type restoreOut struct {
	Package string  `json:"package"`
	Target  sysInfo `json:"target"`
	Plan    struct {
		Files     int           `json:"files"`
		Bytes     int64         `json:"bytes"`
		Conflicts int           `json:"conflicts"`
		Renamed   int           `json:"renamed"`
		Skipped   int           `json:"skipped"`
		Policy    string        `json:"policy"`
		Roots     []rootSummary `json:"roots"`
		Unknown   []string      `json:"unknown_roots"`
		Items     []restoreItem `json:"items"`
	} `json:"plan"`
	Result *struct {
		Files int   `json:"files"`
		Bytes int64 `json:"bytes"`
	} `json:"result"`
	Failed []struct {
		Dest string `json:"path"`
		Err  string `json:"error"`
	} `json:"failed"`
	Duration int64 `json:"duration_ns"`
}

type listOut struct {
	Manifest manifest `json:"manifest"`
	Index    struct {
		Entries []struct {
			Root string `json:"root"`
			Path string `json:"path"`
			Size int64  `json:"size"`
			Dup  bool   `json:"dup"`
		} `json:"entries"`
	} `json:"index"`
}

// ---- presentation -----------------------------------------------------------------------------

func human(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, unit := range units {
		v /= u
		if v < u {
			if v < 10 {
				return fmt.Sprintf("%.1f %s", v, unit)
			}
			return fmt.Sprintf("%.0f %s", v, unit)
		}
	}
	return fmt.Sprintf("%.0f PiB", v)
}

func humanDur(ns int64) string {
	d := time.Duration(ns)
	switch {
	case d < time.Second:
		return "under a second"
	case d < time.Minute:
		return fmt.Sprintf("%.0f sec", d.Seconds())
	default:
		return fmt.Sprintf("%.0f min", d.Minutes())
	}
}

func levelName(l int) string {
	switch l {
	case 1:
		return "1 - compatible"
	case 3:
		return "3 - maximum"
	default:
		return "2 - balanced"
	}
}

func formatScan(o scanOut) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }

	p("System        %s", o.System)
	if o.Inventory != nil {
		p("Home          %s", o.System.Home)
		p("")
		p("Places")
		for _, r := range o.Inventory.Roots {
			p("                %s", r)
		}
		p("")
		p("To include    %s in %d files", human(o.Inventory.Bytes), o.Inventory.Files)
		if o.Inventory.Secrets.Files > 0 {
			p("Secrets       %s in %d files   excluded unless you tick the box",
				human(o.Inventory.Secrets.Bytes), o.Inventory.Secrets.Files)
		}
		if len(o.Inventory.Skipped) > 0 {
			var total int64
			for _, s := range o.Inventory.Skipped {
				total += s.Bytes
			}
			p("Excluded      %s   by the built-in rules", human(total))
			for i, s := range o.Inventory.Skipped {
				if i >= 4 {
					break
				}
				p("                %-22s %s", s.Reason, human(s.Bytes))
			}
		}
		if len(o.Inventory.ByClass) > 0 {
			p("")
			p("Composition")
			for _, k := range []string{"text", "binary", "incompressible", "unknown"} {
				if c, ok := o.Inventory.ByClass[k]; ok && c.Bytes > 0 {
					pct := 0.0
					if o.Inventory.Bytes > 0 {
						pct = float64(c.Bytes) * 100 / float64(o.Inventory.Bytes)
					}
					p("                %-16s %-12s %.0f%%", k, human(c.Bytes), pct)
				}
			}
		}
	}
	p("")
	p("Level         %s", levelName(o.Estimate.Level))
	if o.Estimate.Min != o.Estimate.Max {
		p("Estimate      %s - %s   %s", human(o.Estimate.Min), human(o.Estimate.Max), humanDur(o.Estimate.Duration))
	} else {
		p("Estimate      %s   %s", human(o.Estimate.Max), humanDur(o.Estimate.Duration))
	}
	if !o.Estimate.Sampled {
		p("              from the ratio table; a precise figure needs sampling")
	}
	if o.Estimate.Volumes > 0 {
		p("Volumes       %d x 3.5 GiB", o.Estimate.Volumes)
	}
	if o.Dest != nil {
		p("")
		p("Destination   %s   %s free of %s   %s", o.Dest.Path, human(o.Dest.Free), human(o.Dest.Total), o.Dest.FS)
		if o.Fits != nil {
			if *o.Fits {
				p("              It fits, with %s to spare.", human(o.Dest.Free-o.Estimate.Max))
			} else {
				p("              IT DOES NOT FIT. %s short in the worst case.", human(o.Estimate.Max-o.Dest.Free))
			}
		}
	}
	if o.Inventory != nil && len(o.Inventory.Errors) > 0 {
		p("")
		p("Could not read %d places", len(o.Inventory.Errors))
	}
	return b.String()
}

func formatBackup(o backupOut) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }
	p("Package       %s", o.Package)
	p("Written       %d files - %s raw - %s on disk", o.Files, human(o.Bytes), human(o.Stored))
	if o.Dedup > 0 {
		p("Deduplicated  %s saved by files that repeat", human(o.Dedup))
	}
	p("Volumes       %d", o.Manifest.Volumes)
	p("Encryption    %s", o.Manifest.Cipher)
	p("Took          %s", humanDur(o.Duration))
	p("")
	p("Without the passphrase the package is lost for good. Nobody can recover it.")
	return b.String()
}

func formatPlan(o restoreOut, done bool) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }
	p("Package       %s", o.Package)
	p("Migration     %s -> %s", o.Manifestish(), o.Target)
	p("")
	p("Where it goes")
	for _, r := range o.Plan.Roots {
		p("                %-12s %d files, %-12s %s", r.Root, r.Files, human(r.Bytes), r.Dest)
	}
	if len(o.Plan.Unknown) > 0 {
		p("                unknown places go under the DHS-restored folder in your profile")
	}
	p("")
	p("To write      %d files - %s", o.Plan.Files, human(o.Plan.Bytes))
	if o.Plan.Conflicts > 0 {
		switch o.Plan.Policy {
		case "skip":
			p("Conflicts     %d already exist and will be left alone", o.Plan.Conflicts)
		case "overwrite":
			p("Conflicts     %d already exist AND WILL BE REPLACED", o.Plan.Conflicts)
		default:
			p("Conflicts     %d already exist; the existing file stays and the restored one gets a \" (DHS)\" suffix", o.Plan.Conflicts)
		}
	}
	if done && o.Result != nil {
		p("")
		p("Restored      %d files - %s - %s", o.Result.Files, human(o.Result.Bytes), humanDur(o.Duration))
		if len(o.Failed) == 0 {
			p("              Every file was verified against its checksum before being kept.")
		}
	}
	if len(o.Failed) > 0 {
		p("")
		p("Failed        %d files could not be restored intact - nothing was written for them:", len(o.Failed))
		for i, f := range o.Failed {
			if i >= 8 {
				p("                ... and %d more", len(o.Failed)-i)
				break
			}
			p("                %s: %s", filepath.Base(f.Dest), f.Err)
		}
	}
	if !done {
		p("")
		p("Nothing has been written yet.")
	}
	return b.String()
}

// Manifestish keeps the migration line readable when the package predates a field.
func (o restoreOut) Manifestish() string {
	if o.Package == "" {
		return "a package"
	}
	return filepath.Base(o.Package)
}

func formatList(o listOut) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }
	p("Made on       %s %s (%s)", o.Manifest.Source.Name, o.Manifest.Source.Version, o.Manifest.Source.Arch)
	p("Created       %s   by %s", o.Manifest.Created, o.Manifest.Tool)
	p("Contents      %d files - %s raw - %s on disk", o.Manifest.Files, human(o.Manifest.Raw), human(o.Manifest.Stored))
	p("Package       %d volumes - level %s - %s%s", o.Manifest.Volumes, levelName(o.Manifest.Level), o.Manifest.Cipher,
		map[bool]string{true: " - complete", false: " - INCOMPLETE"}[o.Manifest.Complete])
	if n := len(o.Index.Entries); n > 0 {
		p("")
		dups := 0
		for _, e := range o.Index.Entries {
			if e.Dup {
				dups++
			}
		}
		if dups > 0 {
			p("%d of the %d files are duplicates and take up space only once", dups, n)
			p("")
		}
		for i, e := range o.Index.Entries {
			if i >= 300 {
				p("... and %d more", n-i)
				break
			}
			tag := ""
			if e.Dup {
				tag = "  (dup)"
			}
			p("%-10s  %s/%s%s", human(e.Size), e.Root, e.Path, tag)
		}
	}
	return b.String()
}

func parse[T any](out []byte) (T, error) {
	var v T
	err := json.Unmarshal(out, &v)
	return v, err
}
