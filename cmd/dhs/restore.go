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

	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/report"
	"github.com/Necta14/dhs/internal/restore"
	"github.com/Necta14/dhs/internal/system"
)

const restoreUsage = `dhs restore — put the files from a package back in place on the current system

Usage
  dhs restore <package> [options]

First shows the plan: what goes where, what already exists, which names had to be adapted. Writes
nothing without confirmation. Every file is written to a temporary, checked against the checksum
from the package, and only then put in its place. A file that fails never reaches the destination.

Options
  --dry-run                  only the plan, without confirmation and without writing
  --only <root,root>         restore only these places: documents, pictures, videos, music,
                             downloads, desktop, config, other
  --conflicts <policy>       what to do when the file already exists:
                               keep-both   keep the existing one, write the restored one with a " (DHS)" suffix (default)
                               skip        do not write the restored one
                               overwrite   replace the existing one — only if you know what you are doing
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --yes                      do not ask for confirmation
  --json
  --help
`

type restoreOutput struct {
	Package  string              `json:"package"`
	Target   system.Info         `json:"target"`
	Plan     restorePlanJSON     `json:"plan"`
	Report   *pack.ExtractReport `json:"result,omitempty"`
	Failed   []restoreFailure    `json:"failed,omitempty"`
	Duration time.Duration       `json:"duration_ns,omitempty"`
}

type restorePlanJSON struct {
	Files     int                   `json:"files"`
	Bytes     int64                 `json:"bytes"`
	Conflicts int                   `json:"conflicts"`
	Renamed   int                   `json:"renamed"`
	Skipped   int                   `json:"skipped"`
	Policy    restore.Conflict      `json:"policy"`
	Roots     []restore.RootSummary `json:"roots"`
	Unknown   []pack.Root           `json:"unknown_roots,omitempty"`
	Items     []restoreItemJSON     `json:"items"`
}

type restoreItemJSON struct {
	Path   string         `json:"path"`
	Dest   string         `json:"destination"`
	Action restore.Action `json:"action"`
	Note   string         `json:"note,omitempty"`
	Bytes  int64          `json:"bytes"`
}

type restoreFailure struct {
	Dest string `json:"path"`
	Err  string `json:"error"`
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(restoreUsage)) }
	dryRun := fs.Bool("dry-run", false, "")
	only := fs.String("only", "", "")
	conflictFlag := fs.String("conflicts", "", "")
	passFile := fs.String("passphrase-file", "", "")
	yes := fs.Bool("yes", false, "")
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
		return errors.New(i18n.T("restore: give exactly one package"))
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
		fmt.Fprintln(os.Stderr, warn(i18n.T("The package is marked INCOMPLETE: its writing was interrupted. Only what is intact will be restored.")))
	}

	info, err := system.Detect()
	if err != nil {
		return fmt.Errorf(i18n.T("cannot detect the system: %w"), err)
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
		fmt.Println(warn(i18n.T("Nothing to write.")))
		return nil
	}
	if *dryRun {
		if *asJSON {
			return writeJSON(out)
		}
		fmt.Println(dim(i18n.T("--dry-run: nothing was written.")))
		return nil
	}

	// Disk space: the plan says how much we write; check it on every destination root.
	for _, rs := range plan.Roots {
		if v, err := system.SpaceOf(rs.Dest); err == nil && v.Free < rs.Bytes {
			return fmt.Errorf(i18n.T("no room on %s: %s needed, %s free"), rs.Dest, report.Bytes(rs.Bytes), report.Bytes(v.Free))
		}
	}

	if policy == restore.Overwrite && plan.Conflicts > 0 && !*yes {
		fmt.Fprintln(os.Stderr, warn(i18n.Tf("%d existing files will be OVERWRITTEN. They cannot be recovered.", plan.Conflicts)))
	}
	if !*yes && !*asJSON {
		if !confirm(i18n.Tf("Writing %s files (%s). Continue?", report.Count(int64(plan.Files)), report.Bytes(plan.Bytes))) {
			return errors.New(i18n.T("cancelled"))
		}
	}
	if *asJSON && !*yes {
		return errors.New(i18n.T("restore --json requires --yes or --dry-run"))
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
			fmt.Fprintf(os.Stderr, i18n.T("\r\033[K  restoring… %s / %s"), report.Bytes(done), report.Bytes(total))
		},
	})
	if live {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if err != nil {
		return fmt.Errorf(i18n.T("the restore stopped: %w"), err)
	}
	out.Report = rep
	out.Duration = time.Since(start)
	for _, f := range rep.Failed {
		out.Failed = append(out.Failed, restoreFailure{Dest: f.Entry.Path, Err: f.Err.Error()})
	}
	if *asJSON {
		return writeJSON(out)
	}

	fmt.Printf(i18n.T("%s%s files · %s · %s\n"), report.Pad(i18n.T("Restored"), 14),
		report.Count(int64(rep.Files)), report.Bytes(rep.Bytes), report.Duration(out.Duration))
	if len(rep.Failed) > 0 {
		fmt.Printf("%s%s\n", report.Pad(i18n.T("Failed"), 14), red(i18n.Tf("%d files could not be restored intact — nothing was written for them:", len(rep.Failed))))
		for i, f := range rep.Failed {
			if i == 10 {
				fmt.Printf("%s%s\n", report.Pad("", 16), dim(i18n.Tf("… and %d more (see --json)", len(rep.Failed)-10)))
				break
			}
			fmt.Printf("%s%s  %s\n", report.Pad("", 16), f.Entry.Path, dim(f.Err.Error()))
		}
		return errors.New(i18n.T("incomplete restore"))
	}
	fmt.Printf("%s%s\n", report.Pad("", 14), green(i18n.T("✓ All files were verified and put back in place.")))
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
	fmt.Printf("%s%s %s → %s\n", report.Pad(i18n.T("Migration"), w), m.Source.Name, m.Source.Version, info)
	fmt.Printf(i18n.T("%s%s files · %s · created %s\n"), report.Pad(i18n.T("Package"), w),
		report.Count(m.Files), report.Bytes(m.Raw), m.Created.Local().Format("2006-01-02 15:04"))
	fmt.Println()
	fmt.Println(i18n.T("Where it goes"))
	for _, rs := range p.Roots {
		fmt.Printf("%s%s %s  %s\n", report.Pad("", w+2), report.Pad(string(rs.Root), 12),
			report.Pad(i18n.Tf("%s files, %s", report.Count(int64(rs.Files)), report.Bytes(rs.Bytes)), 26), dim(rs.Dest))
	}
	if len(p.Unknown) > 0 {
		names := make([]string, len(p.Unknown))
		for i, u := range p.Unknown {
			names[i] = string(u)
		}
		fmt.Printf("%s%s\n", report.Pad("", w+2), dim(i18n.T("places that do not exist here, moved under DHS-restored: ")+strings.Join(names, ", ")))
	}
	fmt.Println()
	fmt.Printf(i18n.T("%s%s files · %s\n"), report.Pad(i18n.T("To write"), w), report.Count(int64(p.Files)), report.Bytes(p.Bytes))
	if p.Conflicts > 0 {
		what := map[restore.Conflict]string{
			restore.KeepBoth:  i18n.T("the existing one stays, the restored one gets the \" (DHS)\" suffix"),
			restore.Skip:      i18n.T("the restored one is not written"),
			restore.Overwrite: i18n.T("the existing one is REPLACED"),
		}[p.Policy]
		fmt.Printf(i18n.T("%s%d already exist → %s\n"), report.Pad(i18n.T("Conflicts"), w), p.Conflicts, what)
	}
	if p.Renamed > 0 {
		fmt.Printf(i18n.T("%s%d names adapted to this system (forbidden characters, case, reserved names)\n"), report.Pad(i18n.T("Renamed"), w), p.Renamed)
	}
	shown := 0
	for _, it := range p.Items {
		if it.Renamed || it.Exists {
			if shown == 0 {
				fmt.Println()
			}
			if shown == 8 {
				fmt.Printf("%s%s\n", report.Pad("", w+2), dim(i18n.T("… the rest in --json")))
				break
			}
			fmt.Printf("%s%s %s  %s\n", report.Pad("", w+2), report.Pad(i18n.T(string(it.Action)), 14), it.Entry.Path, dim("→ "+it.Dest))
			shown++
		}
	}
	fmt.Println()
}
