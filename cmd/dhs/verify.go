package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/report"
)

const verifyUsage = `dhs verify — verifică un pachet, fără să extragă nimic

Utilizare
  dhs verify <pachet> [opțiuni]

Verifică sumele fiecărui fișier din pachet, apoi decriptează fiecare volum și verifică fiecare bloc
față de suma lui de control. Dacă trece, pachetul se poate restaura integral.

Opțiuni
  --parola-fisier <cale>   citește fraza de acces din fișier; altfel o cere
  --json
  --help
`

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, verifyUsage) }
	passFile := fs.String("parola-fisier", "", "")
	asJSON := fs.Bool("json", false, "")

	var positional []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return nil
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	if len(positional) != 1 {
		return errors.New("verify: dă exact un pachet")
	}
	dir := positional[0]
	live := !*asJSON && isTerminal(os.Stderr)

	r, err := openPackage(dir, *passFile, *asJSON)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rep, err := r.Verify(ctx, verifyProgress(live))
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(struct {
			Manifest pack.Manifest      `json:"manifest"`
			Report   *pack.VerifyReport `json:"verificare"`
		}{r.Manifest, rep})
	}

	m := r.Manifest
	fmt.Printf("%s%s\n", report.Pad("Pachet", 14), dir)
	fmt.Printf("%s%s %s (%s) · creat %s · %s\n", report.Pad("Sursă", 14), m.Source.Name, m.Source.Version, m.Source.Arch,
		m.Created.Local().Format("2006-01-02 15:04"), dim(m.Tool))
	fmt.Printf("%s%s fișiere · %s → %s · %d volume · nivel %v · cifru %s\n", report.Pad("Conținut", 14),
		report.Count(m.Files), report.Bytes(m.Raw), report.Bytes(m.Stored), m.Volumes, m.Level, m.Cipher)
	fmt.Printf("%s%d fișiere de pachet, %d volume, %d blocuri, %s bruți\n", report.Pad("Verificat", 14),
		rep.Files, rep.Volumes, rep.Blocks, report.Bytes(rep.Bytes))
	if rep.OK() {
		fmt.Printf("%s%s\n", report.Pad("", 14), green("✓ Pachetul e întreg."))
		return nil
	}
	fmt.Printf("%s%s\n", report.Pad("", 14), red(fmt.Sprintf("✗ %d probleme:", len(rep.Problems))))
	for _, p := range rep.Problems {
		fmt.Printf("%s%s\n", report.Pad("", 16), p)
	}
	return errors.New("pachetul nu a trecut verificarea")
}

// openPackage deschide un pachet, cerând fraza de acces doar dacă trebuie.
func openPackage(dir, passFile string, noPrompt bool) (*pack.Reader, error) {
	m, err := pack.Peek(dir)
	if err != nil {
		return nil, err
	}
	pass := ""
	if m.Cipher != pack.CipherNone.String() {
		switch {
		case passFile != "":
			b, err := os.ReadFile(passFile)
			if err != nil {
				return nil, fmt.Errorf("nu pot citi fraza din %s: %w", passFile, err)
			}
			pass = strings.TrimRight(string(b), "\r\n")
		case noPrompt:
			return nil, errors.New("pachetul e criptat; cu --json trebuie --parola-fisier")
		default:
			pass, err = askPassphrase("Fraza de acces a pachetului: ")
			if err != nil {
				return nil, err
			}
		}
	}
	r, err := pack.Open(dir, pass)
	if errors.Is(err, pack.ErrBadPassphrase) {
		return nil, errors.New("fraza de acces e greșită")
	}
	return r, err
}
