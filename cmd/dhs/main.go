// Command dhs — Direct Handoff Suite.
//
// Migrates a user's working environment between operating systems through a portable package on
// an external drive. No cloud, no network, no AI: a deterministic system tool.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Necta14/dhs/internal/i18n"
)

// version is filled in at build time with -ldflags "-X main.version=...".
var version = "dev"

// errUsage says the command has already printed its own message, and main only has to set the
// exit code. Returning nil there would report success for a mistyped flag, which is worse than
// useless: a script, a CI job or the GUI would carry on as if the command had run.
var errUsage = errors.New("usage")

const usage = `dhs — Direct Handoff Suite · migrate your working environment between operating systems

Usage
  dhs <command> [options]

Commands
  scan       show what would be saved, how much it takes and whether it fits the destination
  backup     create the migration package on the external drive
  verify     verify a package, block by block, without extracting anything
  list       show what is inside a package
  restore    put the files back in place on the current system, following an approved plan
  version    version and detected system

Help for a command
  dhs <command> --help
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, i18n.T(usage))
		os.Exit(2)
	}

	var err error
	switch cmd := os.Args[1]; cmd {
	case "scan":
		err = runScan(os.Args[2:])
	case "backup":
		err = runBackup(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	case "version", "--version", "-v":
		err = runVersion()
	case "help", "--help", "-h":
		fmt.Print(i18n.T(usage))
	default:
		fmt.Fprintf(os.Stderr, i18n.T("dhs: unknown command %q\n\n%s"), cmd, i18n.T(usage))
		os.Exit(2)
	}

	if err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "dhs: %v\n", err)
		os.Exit(1)
	}
}
