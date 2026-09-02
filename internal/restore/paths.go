// Package restore pune fișierele dintr-un pachet la locul lor pe sistemul curent.
//
// Aici se întâlnesc două sisteme de fișiere care nu se cunosc: numele venite de pe Linux pot fi
// ilegale pe Windows (`raport: final.docx`), două fișiere care pe Linux diferă (`A.txt`, `a.txt`)
// pe Windows sunt același fișier, iar o cale absolută de pe alt sistem nu înseamnă nimic aici.
// Planul rezolvă toate astea ÎNAINTE să scriem un singur octet, și le arată utilizatorului.
package restore

import (
	"strings"
	"unicode"

	"github.com/Necta14/dhs/internal/system"
)

// windowsReserved sunt numele pe care Windows le refuză indiferent de extensie.
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// Sanitize adaptează o cale relativă (cu „/") la sistemul destinație. Întoarce calea și dacă a
// fost schimbată. Nu inventează: doar înlocuiește ce ar face sistemul să refuze fișierul, cu „_",
// ca utilizatorul să-l recunoască.
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
	// „.." nu are ce căuta într-un pachet; dacă totuși apare, devine inofensiv.
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
		// Windows taie punctele și spațiile de la final, deci „a." și „a" ar fi același fișier.
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

// FoldKey e cheia după care două nume se ciocnesc pe un sistem de fișiere care nu ține cont de
// majuscule. Pe Linux e numele însuși; pe Windows, forma cu litere mici.
func FoldKey(rel string, target system.OS) string {
	if target == system.Windows {
		return strings.ToLower(rel)
	}
	return rel
}

// WithSuffix inserează un sufix înaintea extensiei: raport.docx → raport (DHS).docx.
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
