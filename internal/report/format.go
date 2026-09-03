// Package report formats numbers for human eyes.
//
// The decimal comma and the space as thousands separator are deliberate and stay: they are
// neutral enough across locales, and the CLI layer will own localisation later.
package report

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// Bytes formats a size with a decimal comma.
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

// Range formats an interval, collapsing "3,0 – 3,0 GiB" to "3,0 GiB" when the ends coincide.
func Range(lo, hi int64) string {
	if lo == hi {
		return Bytes(lo)
	}
	return Bytes(lo) + " – " + Bytes(hi)
}

// Duration formats an estimated time, rounded so it does not claim a precision it does not have.
func Duration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "under a second"
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

// Count formats a large number with a space as thousands separator: 412 883.
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

// Pad pads a text to the given width, counting characters, not bytes — otherwise non-ASCII
// letters break column alignment.
func Pad(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
