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
	"time"

	"github.com/necta/dhs/internal/pack"
	"github.com/necta/dhs/internal/report"
	"github.com/necta/dhs/internal/restore"
	"github.com/necta/dhs/internal/system"
)

const restoreUsage = `dhs restore — pune fișierele dintr-un pachet la locul lor pe sistemul curent

Utilizare
  dhs restore <pachet> [opțiuni]

Întâi arată planul: ce merge unde, ce există deja, ce nume au trebuit adaptate. Nu scrie nimic
fără confirmare. Fiecare fișier se scrie într-un temporar, se verifică față de suma din pachet și
abia apoi e pus la locul lui. Un fișier care nu trece nu ajunge niciodată la destinație.

Opțiuni
  --dry-run                doar planul, fără confirmare și fără scriere
  --doar <rad,rad>         restaurează doar aceste locuri: documente, imagini, video, muzica,
                           descarcari, desktop, config, alt
  --conflicte <politica>   ce facem când fișierul există deja:
                             alaturi     păstrează existentul, scrie restauratul cu sufix „ (DHS)” (implicit)
                             sari        nu scrie restauratul
                             suprascrie  înlocuiește existentul — doar dacă știi ce faci
  --parola-fisier <cale>   citește fraza de acces din fișier; altfel o cere
  --da                     nu cere confirmare
  --json
  --help
`

type restoreOutput struct {
	Package  string              `json:"pachet"`
	Target   system.Info         `json:"destinatie"`
	Plan     restorePlanJSON     `json:"plan"`
	Report   *pack.ExtractReport `json:"rezultat,omitempty"`
	Failed   []restoreFailure    `json:"esuate,omitempty"`
	Duration time.Duration       `json:"durata_ns,omitempty"`
}

type restorePlanJSON struct {
	Files     int                   `json:"fisiere"`
	Bytes     int64                 `json:"octeti"`
	Conflicts int                   `json:"conflicte"`
	Renamed   int                   `json:"redenumite"`
	Skipped   int                   `json:"sarite"`
	Policy    restore.Conflict      `json:"politica"`
	Roots     []restore.RootSummary `json:"locuri"`
	Unknown   []pack.Root           `json:"locuri_necunoscute,omitempty"`
	Items     []restoreItemJSON     `json:"intrari"`
}

type restoreItemJSON struct {
	Path   string         `json:"cale"`
	Dest   string         `json:"destinatie"`
	Action restore.Action `json:"actiune"`
	Note   string         `json:"nota,omitempty"`
	Bytes  int64          `json:"octeti"`
}

type restoreFailure struct {
	Dest string `json:"destinatie"`
	Err  string `json:"eroare"`
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, restoreUsage) }
	dryRun := fs.Bool("dry-run", false, "")
	only := fs.String("doar", "", "")
	conflictFlag := fs.String("conflicte", "", "")
	passFile := fs.String("parola-fisier", "", "")
	yes := fs.Bool("da", false, "")
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
		return errors.New("restore: dă exact un pachet")
	}
	dir := positional[0]
	live := !*asJSON && isTerminal(os.Stderr)

	policy, err := restore.ParseConflict(*conflictFlag)
	if err != nil {
		return err
	}
	var filter func(*pack.Entry) bool
	if *only != "" {
		want := map[pack.Root]bool{}
		for _, r := range strings.Split(*only, ",") {
			if r = strings.TrimSpace(r); r != "" {
				want[pack.Root(r)] = true
			}
		}
		filter = func(e *pack.Entry) bool { return want[e.Root] }
	}

	r, err := openPackage(dir, *passFile, *asJSON)
	if err != nil {
		return err
	}
	if !r.Complete() {
		fmt.Fprintln(os.Stderr, warn("Pachetul e marcat INCOMPLET: scrierea lui a fost întreruptă. Se restaurează doar ce e întreg."))
	}

	info, err := system.Detect()
	if err != nil {
		return fmt.Errorf("nu pot detecta sistemul: %w", err)
	}
	rm := pack.NewRootMap(info.Home, system.Locations(info))

	plan, err := restore.Build(restore.Options{Reader: r, Roots: rm, Target: info.OS, Filter: filter, Conflict: policy})
	if err != nil {
		return err
	}

	out := restoreOutput{Package: dir, Target: info, Plan: planJSON(plan)}
	if !*asJSON {
		printPlan(r, info, plan)
	}
	if plan.Files == 0 {
		if *asJSON {
			return writeJSON(out)
		}
		fmt.Println(warn("Nimic de scris."))
		return nil
	}
	if *dryRun {
		if *asJSON {
			return writeJSON(out)
		}
		fmt.Println(dim("--dry-run: nu s-a scris nimic."))
		return nil
	}

	// Loc pe disc: planul spune cât scriem; verificăm pe cel mai mare loc destinație.
	for _, rs := range plan.Roots {
		if v, err := system.SpaceOf(rs.Dest); err == nil && v.Free < rs.Bytes {
			return fmt.Errorf("nu e loc pe %s: trebuie %s, sunt %s liberi", rs.Dest, report.Bytes(rs.Bytes), report.Bytes(v.Free))
		}
	}

	if policy == restore.Overwrite && plan.Conflicts > 0 && !*yes {
		fmt.Fprintln(os.Stderr, warn(fmt.Sprintf("%d fișiere existente vor fi SUPRASCRISE. Nu se pot recupera.", plan.Conflicts)))
	}
	if !*yes && !*asJSON {
		if !confirm(fmt.Sprintf("Scriu %s fișiere (%s). Continui?", report.Count(int64(plan.Files)), report.Bytes(plan.Bytes))) {
			return errors.New("anulat")
		}
	}
	if *asJSON && !*yes {
		return errors.New("restore --json cere --da sau --dry-run")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	var lastLine time.Time
	rep, err := plan.Execute(ctx, restore.ExecOptions{
		OnProgress: func(done, total int64) {
			if !live || time.Since(lastLine) < 200*time.Millisecond {
				return
			}
			lastLine = time.Now()
			fmt.Fprintf(os.Stderr, "\r\033[K  restaurez… %s / %s", report.Bytes(done), report.Bytes(total))
		},
	})
	if live {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if err != nil {
		return fmt.Errorf("restaurarea s-a oprit: %w", err)
	}
	out.Report = rep
	out.Duration = time.Since(start)
	for _, f := range rep.Failed {
		out.Failed = append(out.Failed, restoreFailure{Dest: f.Entry.Path, Err: f.Err.Error()})
	}
	if *asJSON {
		return writeJSON(out)
	}

	fmt.Printf("%s%s fișiere · %s · %s\n", report.Pad("Restaurat", 14),
		report.Count(int64(rep.Files)), report.Bytes(rep.Bytes), report.Duration(out.Duration))
	if len(rep.Failed) > 0 {
		fmt.Printf("%s%s\n", report.Pad("Eșuate", 14), red(fmt.Sprintf("%d fișiere nu s-au putut restaura întregi — nu s-a scris nimic pentru ele:", len(rep.Failed))))
		for i, f := range rep.Failed {
			if i == 10 {
				fmt.Printf("%s%s\n", report.Pad("", 16), dim(fmt.Sprintf("… și încă %d (vezi --json)", len(rep.Failed)-10)))
				break
			}
			fmt.Printf("%s%s  %s\n", report.Pad("", 16), f.Entry.Path, dim(f.Err.Error()))
		}
		return errors.New("restaurare incompletă")
	}
	fmt.Printf("%s%s\n", report.Pad("", 14), green("✓ Toate fișierele au fost verificate și puse la loc."))
	return nil
}

func planJSON(p *restore.Plan) restorePlanJSON {
	out := restorePlanJSON{
		Files: p.Files, Bytes: p.Bytes, Conflicts: p.Conflicts, Renamed: p.Renamed, Skipped: p.Skipped,
		Policy: p.Policy, Roots: p.Roots, Unknown: p.Unknown,
	}
	for _, it := range p.Items {
		out.Items = append(out.Items, restoreItemJSON{
			Path: string(it.Entry.Root) + "/" + it.Entry.Path, Dest: it.Dest, Action: it.Action, Note: it.Note, Bytes: it.Entry.Size,
		})
	}
	return out
}

func printPlan(r *pack.Reader, info system.Info, p *restore.Plan) {
	const w = 14
	m := r.Manifest
	fmt.Printf("%s%s %s → %s\n", report.Pad("Migrare", w), m.Source.Name, m.Source.Version, info)
	fmt.Printf("%s%s fișiere · %s · creat %s\n", report.Pad("Pachet", w),
		report.Count(m.Files), report.Bytes(m.Raw), m.Created.Local().Format("2006-01-02 15:04"))
	fmt.Println()
	fmt.Println("Unde merge")
	for _, rs := range p.Roots {
		fmt.Printf("%s%s %s  %s\n", report.Pad("", w+2), report.Pad(string(rs.Root), 12),
			report.Pad(fmt.Sprintf("%s fișiere, %s", report.Count(int64(rs.Files)), report.Bytes(rs.Bytes)), 26), dim(rs.Dest))
	}
	if len(p.Unknown) > 0 {
		names := make([]string, len(p.Unknown))
		for i, u := range p.Unknown {
			names[i] = string(u)
		}
		fmt.Printf("%s%s\n", report.Pad("", w+2), dim("locuri care nu există aici, mutate sub DHS-restaurat: "+strings.Join(names, ", ")))
	}
	fmt.Println()
	fmt.Printf("%s%s fișiere · %s\n", report.Pad("De scris", w), report.Count(int64(p.Files)), report.Bytes(p.Bytes))
	if p.Conflicts > 0 {
		what := map[restore.Conflict]string{
			restore.KeepBoth:  "existentul rămâne, restauratul primește sufixul „ (DHS)”",
			restore.Skip:      "restauratul nu se scrie",
			restore.Overwrite: "existentul e ÎNLOCUIT",
		}[p.Policy]
		fmt.Printf("%s%d există deja → %s\n", report.Pad("Conflicte", w), p.Conflicts, what)
	}
	if p.Renamed > 0 {
		fmt.Printf("%s%d nume adaptate acestui sistem (caractere interzise, majuscule, nume rezervate)\n", report.Pad("Redenumite", w), p.Renamed)
	}
	shown := 0
	for _, it := range p.Items {
		if it.Renamed || it.Exists {
			if shown == 0 {
				fmt.Println()
			}
			if shown == 8 {
				fmt.Printf("%s%s\n", report.Pad("", w+2), dim("… restul în --json"))
				break
			}
			fmt.Printf("%s%s %s  %s\n", report.Pad("", w+2), report.Pad(string(it.Action), 14), it.Entry.Path, dim("→ "+it.Dest))
			shown++
		}
	}
	fmt.Println()
}
