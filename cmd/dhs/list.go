package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/report"
)

const listUsage = `dhs list — show what is inside a package, without extracting anything

Usage
  dhs list <package> [options]

Options
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --all                      list every file, not just the summary
  --json
  --help
`

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(listUsage)) }
	passFile := fs.String("passphrase-file", "", "")
	all := fs.Bool("all", false, "")
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
		return errors.New(i18n.T("list: give exactly one package"))
	}

	// Without a passphrase, show what can be seen without it: the manifest.
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
	fmt.Printf(i18n.T("%s%s %s (%s) · created %s · %s\n"), report.Pad(i18n.T("Source"), w), m.Source.Name, m.Source.Version, m.Source.Arch,
		m.Created.Local().Format("2006-01-02 15:04"), dim(m.Tool))
	state := green(i18n.T("complete"))
	if !r.Complete() {
		state = red(i18n.T("INCOMPLETE"))
	}
	fmt.Printf(i18n.T("%s%s files · %s → %s on disk · %d volumes · level %s · cipher %s · %s\n"), report.Pad(i18n.T("Contents"), w),
		report.Count(m.Files), report.Bytes(m.Raw), report.Bytes(m.Stored), m.Volumes, i18n.T(m.Level.String()), m.Cipher, state)
	if m.Secrets {
		fmt.Printf("%s%s\n", report.Pad("", w), warn(i18n.T("contains secrets (keys, passwords)")))
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
		fmt.Printf(i18n.T("%s%s %s files  %s\n"), report.Pad("", 2), report.Pad(string(root), 12), report.Pad(report.Count(int64(a.files)), 9), report.Bytes(a.bytes))
	}
	if dups > 0 {
		fmt.Printf("%s%s\n", report.Pad("", 2), dim(i18n.Tf("%d files are duplicates of others and take up space only once", dups)))
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
				mark += warn(i18n.T(" (secret)"))
			}
			fmt.Printf("%s %s/%s%s\n", report.Pad(report.Bytes(e.Size), 11), e.Root, e.Path, mark)
		}
	}
	return nil
}
