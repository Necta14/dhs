package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/necta/dhs/internal/report"
	"github.com/necta/dhs/internal/scan"
	"github.com/necta/dhs/internal/system"
)

const scanUsage = `dhs scan — arată ce s-ar salva și cât ar ocupa, fără să scrie nimic

Utilizare
  dhs scan [cale...] [opțiuni]

Fără căi, scanează locurile standard din profil: documente, imagini, video, muzică,
descărcări, desktop și configurări.

Opțiuni
  --dest <cale>    destinația pachetului; verifică dacă încape și ce sistem de fișiere are
  --nivel <1|2|3>  nivelul de compresie: 1 compatibil, 2 echilibrat (implicit), 3 maxim
  --secrete        include chei, parole și fișiere .env (implicit sunt doar numărate)
  --tot            nu exclude nimic; arată și cache-urile, jocurile, mașinile virtuale
  --json           ieșire JSON, pentru interfețe grafice și scripturi
  --help
`

type scanOutput struct {
	System    system.Info    `json:"sistem"`
	Inventory *scan.Result   `json:"inventar"`
	Estimate  scan.Estimate  `json:"estimare"`
	Dest      *system.Volume `json:"destinatie,omitempty"`
	Fits      *bool          `json:"incape,omitempty"`
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, scanUsage) }

	dest := fs.String("dest", "", "destinația pachetului")
	level := fs.Int("nivel", int(scan.LevelBalanced), "nivelul de compresie (1–3)")
	secrets := fs.Bool("secrete", false, "include secretele")
	all := fs.Bool("tot", false, "nu exclude nimic")
	asJSON := fs.Bool("json", false, "ieșire JSON")

	// Pachetul flag se oprește la prima cale, deci „dhs scan ~/Documente --json" ar lua „--json"
	// drept cale. Parsăm în bucle, culegând căile pe măsură ce apar, ca ordinea să nu conteze.
	var explicitRoots []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return nil // flag a scris deja mesajul
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		explicitRoots = append(explicitRoots, rest[0])
		rest = rest[1:]
	}

	lvl := scan.Level(*level)
	if !lvl.Valid() {
		return fmt.Errorf("nivelul %d nu există; alege 1, 2 sau 3", *level)
	}

	info, err := system.Detect()
	if err != nil {
		return fmt.Errorf("nu pot detecta sistemul: %w", err)
	}

	roots := explicitRoots
	if len(roots) == 0 {
		var paths []string
		for _, l := range system.Locations(info) {
			if l.Exists {
				paths = append(paths, l.Path)
			}
		}
		roots = scan.HomeRoots(paths)
		if len(roots) == 0 {
			return fmt.Errorf("n-am găsit niciun loc standard în %s; dă căile explicit", info.Home)
		}
	}

	opts := scan.Options{Roots: roots, IncludeSecrets: *secrets}
	if *all {
		opts.Excluder = scan.NewExcluder(nil)
	}
	// Progresul se scrie doar către un terminal: redirecționat, ar umple fișierul cu coduri ANSI.
	live := !*asJSON && isTerminal(os.Stderr)
	if live {
		opts.OnProgress = progressWriter()
	}

	start := time.Now()
	inv, err := scan.Walk(opts)
	if err != nil {
		return err
	}
	if live {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}

	est := scan.Estimator{Workers: runtime.NumCPU()}.Estimate(inv, lvl)

	out := scanOutput{System: info, Inventory: inv, Estimate: est}
	if *dest != "" {
		v, err := system.SpaceOf(*dest)
		if err != nil {
			return fmt.Errorf("nu pot citi destinația %s: %w", *dest, err)
		}
		fits, _ := est.Fits(v.Free)
		out.Dest, out.Fits = &v, &fits
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	printScan(out, time.Since(start))
	return nil
}

func progressWriter() func(files, bytes int64) {
	return func(files, bytes int64) {
		fmt.Fprintf(os.Stderr, "\r\033[K  citesc… %s fișiere, %s", report.Count(files), report.Bytes(bytes))
	}
}

func printScan(o scanOutput, elapsed time.Duration) {
	inv, est := o.Inventory, o.Estimate
	const w = 14

	fmt.Printf("%s%s\n", report.Pad("Sistem", w), o.System)
	fmt.Printf("%s%s\n", report.Pad("Rădăcini", w), strings.Join(inv.Roots, "\n"+report.Pad("", w)))
	fmt.Println()

	fmt.Printf("%s%s în %s fișiere   %s\n",
		report.Pad("De inclus", w), report.Bytes(inv.Bytes), report.Count(inv.Files),
		dim(fmt.Sprintf("(scanat în %s)", report.Duration(elapsed))))

	// Ce s-a exclus și de ce — utilizatorul trebuie să poată contesta fiecare gigaoctet lipsă.
	if n := inv.SkippedBytes(); n > 0 {
		fmt.Printf("%s%s   %s\n", report.Pad("Excluse", w), report.Bytes(n), dim("--tot le include înapoi"))
		for i, s := range inv.Skipped {
			if i == 4 {
				fmt.Printf("%s%s\n", report.Pad("", w+2), dim(fmt.Sprintf("… și încă %d reguli", len(inv.Skipped)-4)))
				break
			}
			fmt.Printf("%s%s %s\n", report.Pad("", w+2),
				report.Pad(s.Dir, 22), dim(fmt.Sprintf("%s · %s", report.Bytes(s.Bytes), s.Reason)))
		}
	}
	if inv.Secrets.Files > 0 {
		fmt.Printf("%s%s în %s fișiere   %s\n", report.Pad("Secrete", w),
			report.Bytes(inv.Secrets.Bytes), report.Count(inv.Secrets.Files),
			dim("excluse implicit; --secrete le include"))
	}
	fmt.Println()

	// Compoziția contează: dacă totul e deja comprimat, nivelul 3 nu are ce câștiga.
	classes := make([]scan.Class, 0, len(inv.ByClass))
	for c := range inv.ByClass {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool {
		return inv.ByClass[classes[i]].Bytes > inv.ByClass[classes[j]].Bytes
	})
	fmt.Println("Compoziție")
	for _, c := range classes {
		b := inv.ByClass[c]
		pct := 0.0
		if inv.Bytes > 0 {
			pct = float64(b.Bytes) / float64(inv.Bytes) * 100
		}
		note := ""
		switch c {
		case scan.Incompressible:
			note = dim("se stochează, nu se comprimă")
		case scan.Unknown:
			note = dim("se lămurește prin test de entropie la împachetare")
		}
		fmt.Printf("%s%s %s  %s %s\n", report.Pad("", w+2), report.Pad(c.String(), 16),
			report.Pad(report.Bytes(b.Bytes), 11),
			report.Pad(fmt.Sprintf("%.0f%%", pct), 5), note)
	}
	fmt.Println()

	fmt.Printf("%s%s\n", report.Pad("Nivel", w), est.Level)
	fmt.Printf("%s%s   %s\n", report.Pad("Estimare", w),
		report.Range(est.Min, est.Max), dim(report.Duration(est.Duration)+" pe "+fmt.Sprint(runtime.NumCPU())+" nuclee"))
	if !est.Sampled {
		fmt.Printf("%s%s\n", report.Pad("", w), dim("estimare din tabel; --precis o va măsura prin eșantionare"))
	}
	if est.Volumes > 0 {
		fmt.Printf("%s%d × %s\n", report.Pad("Volume", w), est.Volumes, report.Bytes(scan.VolumeSize))
	}

	if o.Dest != nil {
		fmt.Println()
		fmt.Printf("%s%s   %s liberi din %s   %s\n", report.Pad("Destinație", w), o.Dest.Path,
			report.Bytes(o.Dest.Free), report.Bytes(o.Dest.Total), dim(string(o.Dest.FS)))
		if limit := o.Dest.FS.MaxFileSize(); limit > 0 {
			fmt.Printf("%s%s\n", report.Pad("", w),
				dim(fmt.Sprintf("%s limitează fișierele la %s — volumele de %s sunt sub limită",
					o.Dest.FS, report.Bytes(limit), report.Bytes(scan.VolumeSize))))
		}
		fmt.Println()
		if o.Fits != nil && *o.Fits {
			_, left := est.Fits(o.Dest.Free)
			fmt.Printf("%s%s\n", report.Pad("", w),
				green(fmt.Sprintf("✓ Încape, cu %s de rezervă.", report.Bytes(left))))
		} else {
			lipsa := est.Max - o.Dest.Free
			fmt.Printf("%s%s\n", report.Pad("", w),
				red(fmt.Sprintf("✗ Nu încape: lipsesc %s.", report.Bytes(lipsa))))
			fmt.Printf("%s%s\n", report.Pad("", w), dim("exclude din ce e mai mare, urcă nivelul de compresie, sau împarte pachetul pe mai multe medii"))
		}
	}

	if len(inv.Largest) > 0 {
		fmt.Println()
		fmt.Println("Cele mai mari")
		for i, e := range inv.Largest {
			if i == 5 {
				break
			}
			fmt.Printf("%s%s %s\n", report.Pad("", w+2), report.Pad(report.Bytes(e.Size), 11), dim(e.Path))
		}
	}

	if n := len(inv.Errors); n > 0 {
		fmt.Println()
		fmt.Printf("%s%s\n", report.Pad("Inaccesibile", w), dim(fmt.Sprintf("%d locuri sărite (drepturi lipsă)", n)))
	}
}

// Culorile se aplică doar la terminal; într-o conductă ies caractere curate.
var useColor = os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func dim(s string) string   { return paint("2", s) }
func green(s string) string { return paint("32", s) }
func red(s string) string   { return paint("31", s) }

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
