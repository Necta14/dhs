// Package report formatează cifrele pentru ochi de om, în română.
package report

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// Bytes formatează o dimensiune cu virgulă zecimală, cum se scrie în română.
func Bytes(n int64) string {
	if n < 0 {
		return "−" + Bytes(-n)
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	digits := 1
	if value >= 100 {
		digits = 0
	}
	return strings.Replace(fmt.Sprintf("%.*f %s", digits, value, units[i]), ".", ",", 1)
}

// Range formatează un interval, comprimând „3,0 – 3,0 GiB" la „3,0 GiB" când capetele coincid.
func Range(lo, hi int64) string {
	if lo == hi {
		return Bytes(lo)
	}
	return Bytes(lo) + " – " + Bytes(hi)
}

// Duration formatează un timp estimat, rotunjit cât să nu pretindă o precizie pe care n-o are.
func Duration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "sub o secundă"
	case d < time.Minute:
		return fmt.Sprintf("~%d sec", int(math.Round(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("~%d min", int(math.Round(d.Minutes())))
	default:
		h := int(d.Hours())
		m := int(math.Round(d.Minutes())) - h*60
		if m == 0 {
			return fmt.Sprintf("~%d h", h)
		}
		return fmt.Sprintf("~%d h %d min", h, m)
	}
}

// Count formatează un număr mare cu separator de mii, ca în română: 412 883.
func Count(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "−" + Count(-n)
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Pad completează un text la lățimea dată, numărând caractere, nu octeți — altfel diacriticele
// strică alinierea coloanelor.
func Pad(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
