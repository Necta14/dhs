package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/necta/dhs/internal/pack"
	"github.com/necta/dhs/internal/report"
)

const listUsage = `dhs list — arată ce e într-un pachet, fără să extragă nimic

Utilizare
  dhs list <pachet> [opțiuni]

Opțiuni
  --parola-fisier <cale>   citește fraza de acces din fișier; altfel o cere
  --tot                    listează fiecare fișier, nu doar rezumatul
  --json
  --help
`

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, listUsage) }
	passFile := fs.String("parola-fisier", "", "")
	all := fs.Bool("tot", false, "")
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
		return errors.New("list: dă exact un pachet")
	}

	// Fără frază, arătăm ce se poate vedea fără ea: manifestul.
	m, err := pack.Peek(positional[0])
	if err != nil {
		return err
	}
	r, err := openPackage(positional[0], *passFile, *asJSON)
	if err != nil {
		if errors.Is(err, pack.ErrNeedPassphrase) && *asJSON {
			return writeJSON(struct {
				Manifest pack.Manifest `json:"manifest"`
			}{m})
		}
		return err
	}

	if *asJSON {
		return writeJSON(struct {
			Manifest pack.Manifest `json:"manifest"`
			Index    pack.Index    `json:"index"`
		}{r.Manifest, r.Index})
	}

	const w = 14
	fmt.Printf("%s%s %s (%s) · creat %s · %s\n", report.Pad("Sursă", w), m.Source.Name, m.Source.Version, m.Source.Arch,
		m.Created.Local().Format("2006-01-02 15:04"), dim(m.Tool))
	state := green("complet")
	if !r.Complete() {
		state = red("INCOMPLET")
	}
	fmt.Printf("%s%s fișiere · %s → %s pe disc · %d volume · nivel %v · cifru %s · %s\n", report.Pad("Conținut", w),
		report.Count(m.Files), report.Bytes(m.Raw), report.Bytes(m.Stored), m.Volumes, m.Level, m.Cipher, state)
	if m.Secrets {
		fmt.Printf("%s%s\n", report.Pad("", w), warn("conține secrete (chei, parole)"))
	}

	type agg struct {
		files int
		bytes int64
	}
	byRoot := map[pack.Root]*agg{}
	var dups int
	for _, e := range r.Index.Entries {
		a := byRoot[e.Root]
		if a == nil {
			a = &agg{}
			byRoot[e.Root] = a
		}
		a.files++
		a.bytes += e.Size
		if e.Dup {
			dups++
		}
	}
	roots := make([]pack.Root, 0, len(byRoot))
	for r := range byRoot {
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool { return byRoot[roots[i]].bytes > byRoot[roots[j]].bytes })
	fmt.Println()
	for _, root := range roots {
		a := byRoot[root]
		fmt.Printf("%s%s %s fișiere  %s\n", report.Pad("", 2), report.Pad(string(root), 12), report.Pad(report.Count(int64(a.files)), 9), report.Bytes(a.bytes))
	}
	if dups > 0 {
		fmt.Printf("%s%s\n", report.Pad("", 2), dim(fmt.Sprintf("%d fișiere sunt duplicate ale altora și ocupă loc o singură dată", dups)))
	}

	if *all {
		fmt.Println()
		entries := append([]*pack.Entry(nil), r.Index.Entries...)
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Root != entries[j].Root {
				return entries[i].Root < entries[j].Root
			}
			return entries[i].Path < entries[j].Path
		})
		for _, e := range entries {
			mark := ""
			if e.Dup {
				mark = dim(" (dup)")
			}
			if e.Secret {
				mark += warn(" (secret)")
			}
			fmt.Printf("%s %s/%s%s\n", report.Pad(report.Bytes(e.Size), 11), e.Root, e.Path, mark)
		}
	}
	return nil
}
