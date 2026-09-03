// Package restore puts the files from a package back in their place on the current system.
//
// This is where two file systems that know nothing of each other meet: names that came from
// Linux may be illegal on Windows (`report: final.docx`), two files that differ on Linux
// (`A.txt`, `a.txt`) are the same file on Windows, and an absolute path from another system
// means nothing here. The plan resolves all of that BEFORE we write a single byte, and shows it
// to the user.
package restore

import (
	"strings"
	"unicode"

	"github.com/Necta14/dhs/internal/system"
)

// windowsReserved are the names Windows refuses regardless of extension.
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// Sanitize adapts a relative path (with "/") to the target system. It returns the path and
// whether it was changed. It does not invent anything: it only replaces what would make the
// system refuse the file, with "_", so the user can still recognise it.
func Sanitize(rel string, target system.OS) (string, bool) {
	parts := strings.Split(rel, "/")
	changed := false
	for i, p := range parts {
		clean := sanitizeSegment(p, target)
		if clean != p {
			changed = true
			parts[i] = clean
		}
	}
	return strings.Join(parts, "/"), changed
}

func sanitizeSegment(s string, target system.OS) string {
	if s == "" || s == "." {
		return s
	}
	// ".." has no business in a package; if it shows up anyway, it becomes harmless.
	if s == ".." {
		return "_.."
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0, r == '/':
			b.WriteRune('_')
		case target == system.Windows && (r < 32 || strings.ContainsRune(`<>:"\|?*`, r)):
			b.WriteRune('_')
		case target == system.Windows && r == unicode.ReplacementChar:
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	if target == system.Windows {
		// Windows strips trailing dots and spaces, so "a." and "a" would be the same file.
		trimmed := strings.TrimRight(out, ". ")
		if trimmed == "" {
			trimmed = "_"
		}
		out = trimmed
		base := out
		if i := strings.IndexByte(out, '.'); i > 0 {
			base = out[:i]
		}
		if windowsReserved[strings.ToLower(base)] {
			out = "_" + out
		}
	}
	return out
}

// FoldKey is the key on which two names collide on a file system that ignores case. On Linux
// it is the name itself; on Windows, the lower-case form.
func FoldKey(rel string, target system.OS) string {
	if target == system.Windows {
		return strings.ToLower(rel)
	}
	return rel
}

// WithSuffix inserts a suffix before the extension: report.docx → report (DHS).docx.
func WithSuffix(rel, suffix string) string {
	dir, name := "", rel
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		dir, name = rel[:i+1], rel[i+1:]
	}
	ext := ""
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name, ext = name[:i], name[i:]
	}
	return dir + name + suffix + ext
}
