package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/necta/dhs/internal/passphrase"
)

// confirm pune o întrebare da/nu. Implicitul e „nu": tăcerea nu aprobă nimic.
func confirm(question string) bool {
	fmt.Fprintf(os.Stderr, "%s [d/N] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "d", "da", "y", "yes":
		return true
	}
	return false
}

// askPassphrase citește fraza fără ecou. Fără terminal, citește o linie de la stdin.
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

// askNewPassphrase cere fraza de două ori și spune clar ce înseamnă să o pierzi.
func askNewPassphrase() (string, error) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, bold("Alege o frază de acces pentru pachet."))
	fmt.Fprintln(os.Stderr, warn("Dacă o pierzi, pachetul e pierdut definitiv. Nu există recuperare, nu există „am uitat parola”."))
	fmt.Fprintln(os.Stderr, dim("Cel puțin 8 caractere. O propoziție lungă pe care o ții minte bate un șir scurt și complicat."))
	for attempt := 0; attempt < 3; attempt++ {
		p1, err := askPassphrase("Fraza de acces: ")
		if err != nil {
			return "", err
		}
		if err := passphrase.Check(p1); err != nil {
			fmt.Fprintln(os.Stderr, red(err.Error()))
			continue
		}
		p2, err := askPassphrase("Încă o dată:    ")
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			fmt.Fprintln(os.Stderr, red("Nu se potrivesc."))
			continue
		}
		return p1, nil
	}
	return "", errors.New("prea multe încercări")
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func bold(s string) string { return paint("1", s) }
func warn(s string) string { return paint("33", s) }
