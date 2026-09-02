// Comanda dhs — Direct Handoff Suite.
//
// Migrează mediul de lucru al unui utilizator între sisteme de operare, printr-un pachet portabil
// pe mediu extern. Fără cloud, fără rețea, fără AI: o unealtă deterministă de sistem.
package main

import (
	"fmt"
	"os"
)

// version se completează la build cu -ldflags "-X main.version=...".
var version = "dev"

const usage = `dhs — Direct Handoff Suite · migrarea mediului de lucru între sisteme de operare

Utilizare
  dhs <comandă> [opțiuni]

Comenzi
  scan       arată ce s-ar salva, cât ar ocupa și dacă încape pe destinație
  backup     creează pachetul de migrare pe mediul extern
  verify     verifică un pachet, bloc cu bloc, fără să extragă nimic
  version    versiunea și sistemul detectat

Ajutor pentru o comandă
  dhs <comandă> --help
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
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
	case "version", "--version", "-v":
		err = runVersion()
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "dhs: comandă necunoscută %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "dhs: %v\n", err)
		os.Exit(1)
	}
}
