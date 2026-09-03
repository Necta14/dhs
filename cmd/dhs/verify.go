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

	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/report"
)

const verifyUsage = `dhs verify — verify a package, without extracting anything

Usage
  dhs verify <package> [options]

Checks the checksum of every file in the package, then decrypts every volume and checks every
block against its checksum. If it passes, the package can be restored in full.

Options
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --json
  --help
`

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(verifyUsage)) }
	passFile := fs.String("passphrase-file", "", "")
	asJSON := fs.Bool("json", false, "")

	var positional []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil // asking for help is not a failure
			}
			return errUsage // flag printed the message; exit 2 rather than pretend success
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	if len(positional) != 1 {
		return errors.New(i18n.T("verify: give exactly one package"))
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
			Report   *pack.VerifyReport `json:"verification"`
		}{r.Manifest, rep})
	}

	m := r.Manifest
	fmt.Printf("%s%s\n", report.Pad(i18n.T("Package"), 14), dir)
	fmt.Printf(i18n.T("%s%s %s (%s) · created %s · %s\n"), report.Pad(i18n.T("Source"), 14), m.Source.Name, m.Source.Version, m.Source.Arch,
		m.Created.Local().Format("2006-01-02 15:04"), dim(m.Tool))
	fmt.Printf(i18n.T("%s%s files · %s → %s · %d volumes · level %s · cipher %s\n"), report.Pad(i18n.T("Contents"), 14),
		report.Count(m.Files), report.Bytes(m.Raw), report.Bytes(m.Stored), m.Volumes, i18n.T(m.Level.String()), m.Cipher)
	fmt.Printf(i18n.T("%s%d package files, %d volumes, %d blocks, %s raw\n"), report.Pad(i18n.T("Verified"), 14),
		rep.Files, rep.Volumes, rep.Blocks, report.Bytes(rep.Bytes))
	if rep.OK() {
		fmt.Printf("%s%s\n", report.Pad("", 14), green(i18n.T("✓ The package is intact.")))
		return nil
	}
	fmt.Printf("%s%s\n", report.Pad("", 14), red(i18n.Tf("✗ %d problems:", len(rep.Problems))))
	for _, p := range rep.Problems {
		fmt.Printf("%s%s\n", report.Pad("", 16), p)
	}
	return errors.New(i18n.T("the package did not pass verification"))
}

// openPackage opens a package, asking for the passphrase only when needed.
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
				return nil, fmt.Errorf(i18n.T("cannot read the passphrase from %s: %w"), passFile, err)
			}
			pass = strings.TrimRight(string(b), "\r\n")
		case noPrompt:
			return nil, errors.New(i18n.T("the package is encrypted; with --json you need --passphrase-file"))
		default:
			pass, err = askPassphrase(i18n.T("Package passphrase: "))
			if err != nil {
				return nil, err
			}
		}
	}
	r, err := pack.Open(dir, pass)
	if errors.Is(err, pack.ErrBadPassphrase) {
		return nil, errors.New(i18n.T("wrong passphrase"))
	}
	return r, err
}
