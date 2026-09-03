package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/passphrase"
)

// confirm asks a yes/no question. The default is "no": silence approves nothing.
// Both the English and the Romanian answers are accepted, whatever the display language.
func confirm(question string) bool {
	fmt.Fprintf(os.Stderr, i18n.T("%s [y/N] "), question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "d", "da", "y", "yes":
		return true
	}
	return false
}

// askPassphrase reads the passphrase without echo. Without a terminal, it reads a line from stdin.
func askPassphrase(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if !isTerminal(os.Stdin) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// askNewPassphrase asks for the passphrase twice and says clearly what losing it means.
func askNewPassphrase() (string, error) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, bold(i18n.T("Choose a passphrase for the package.")))
	fmt.Fprintln(os.Stderr, warn(i18n.T("If you lose it, the package is lost for good. There is no recovery, no \"I forgot my password\".")))
	fmt.Fprintln(os.Stderr, dim(i18n.T("At least 8 characters. A long sentence you remember beats a short, complicated string.")))
	for attempt := 0; attempt < 3; attempt++ {
		p1, err := askPassphrase(i18n.T("Passphrase: "))
		if err != nil {
			return "", err
		}
		if err := passphrase.Check(p1); err != nil {
			fmt.Fprintln(os.Stderr, red(err.Error()))
			continue
		}
		p2, err := askPassphrase(i18n.T("Once more:  "))
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			fmt.Fprintln(os.Stderr, red(i18n.T("They do not match.")))
			continue
		}
		return p1, nil
	}
	return "", errors.New(i18n.T("too many attempts"))
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func bold(s string) string { return paint("1", s) }
func warn(s string) string { return paint("33", s) }
