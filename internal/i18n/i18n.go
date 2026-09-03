// Package i18n translates the CLI's user-facing text.
//
// English is the primary language: every English string is also the catalog key, so the code
// reads naturally and a missing translation simply falls back to English. The only other
// language today is Romanian, in ro.go.
//
// Boundaries: library packages (scan, pack, restore, system, ...) never import i18n — they
// return English values and errors, and only cmd/dhs decides how to show them. This keeps the
// core deterministic and testable regardless of the user's locale.
//
// The language is picked once, at start-up: DHS_LANG ("ro" or "en") wins; otherwise the first
// non-empty of LC_ALL, LC_MESSAGES, LANG is inspected and a value starting with "ro" selects
// Romanian. Anything else is English.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// lang is computed once at init; it never changes while the process runs.
var lang = detect()

// Lang returns the active language: "ro" or "en".
func Lang() string { return lang }

// T returns the translation of s for the active language. Unknown keys, and every key when the
// language is English, come back unchanged.
func T(s string) string {
	if lang == "ro" {
		if t, ok := ro[s]; ok {
			return t
		}
	}
	return s
}

// Tf translates the format string, then formats it. Format verbs are identical between a key and
// its translation, so the arguments line up in both languages.
func Tf(format string, a ...any) string { return fmt.Sprintf(T(format), a...) }

// detect reads the environment and decides the language. DHS_LANG is an explicit override; the
// POSIX locale variables are consulted in the usual precedence order.
func detect() string {
	switch os.Getenv("DHS_LANG") {
	case "ro":
		return "ro"
	case "en":
		return "en"
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(name)
		if v == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(v), "ro") {
			return "ro"
		}
		return "en"
	}
	return "en"
}
