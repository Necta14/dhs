package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/apps"
	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/report"
	"github.com/Necta14/dhs/internal/system"
)

const planUsage = `dhs plan — what would be installed and where each configuration would go, touching nothing

Usage
  dhs plan <package> [options]

Reads the application manifest of the package, looks at what this system has and can install, and
shows: the applications to install and through which manager, the ones already here, the ones with
no way to install them here, the ones with no version for this platform (with their equivalents),
the ones the database does not know, and where every configuration goes. Then the exact commands.
Nothing runs. The same plan is what dhs install executes after your approval.

Options
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --json
  --help
`

const installUsage = `dhs install — install the applications from a package, following an approved plan

Usage
  dhs install <package> [options]

Shows the plan (the same as dhs plan), including every command, and asks once. Then runs the
commands in order, output visible, and reports what did not install. Installing needs the
system's package manager: sudo on Linux, an administrator prompt on Windows. Files and
configuration are restored by dhs restore, not here.

Options
  --skip <id,id>             leave these applications out (ids as shown by dhs plan)
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --dry-run                  show the plan and stop; the same as dhs plan
  --yes                      do not ask for confirmation
  --json
  --help
`

// appsOutput is the JSON of dhs plan and dhs install.
type appsOutput struct {
	Package  string          `json:"package"`
	Source   pack.SourceInfo `json:"source"`
	Target   system.Info     `json:"target"`
	Manifest *apps.Manifest  `json:"manifest"`
	Plan     *apps.Plan      `json:"plan"`
	Results  []apps.Result   `json:"results,omitempty"`
}

func runPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(planUsage)) }
	passFile := fs.String("passphrase-file", "", "")
	asJSON := fs.Bool("json", false, "")
	dir, err := onePositional(fs, args, "plan")
	if err != nil {
		return err
	}
	out, _, err := loadAppsPlan(dir, *passFile, *asJSON, nil)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out)
	}
	printAppPlan(out, true)
	return nil
}

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(installUsage)) }
	skip := fs.String("skip", "", "")
	passFile := fs.String("passphrase-file", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	yes := fs.Bool("yes", false, "")
	asJSON := fs.Bool("json", false, "")
	dir, err := onePositional(fs, args, "install")
	if err != nil {
		return err
	}
	skipSet := map[string]bool{}
	for _, id := range strings.Split(*skip, ",") {
		if id = strings.TrimSpace(id); id != "" {
			skipSet[id] = true
		}
	}
	out, target, err := loadAppsPlan(dir, *passFile, *asJSON, skipSet)
	if err != nil {
		return err
	}
	plan := out.Plan
	if !*asJSON {
		printAppPlan(out, true)
	}
	if len(plan.Commands) == 0 {
		if *asJSON {
			return writeJSON(out)
		}
		fmt.Println(dim(i18n.T("Nothing to install.")))
		return nil
	}
	if *dryRun {
		if *asJSON {
			return writeJSON(out)
		}
		fmt.Println(dim(i18n.T("--dry-run: nothing was run.")))
		return nil
	}
	if apps.NeedsElevation(plan.Commands) {
		return errors.New(i18n.T("installing needs root and neither sudo nor doas is available; run dhs install as root"))
	}
	if !*yes && !*asJSON {
		if target.OS == appdb.Windows && !apps.IsAdmin() {
			fmt.Fprintln(os.Stderr, warn(i18n.T("Not running as administrator: expect a permission prompt for every installation.")))
		}
		if !confirm(i18n.Tf("Run these %d commands and install %d applications?", len(plan.Commands), len(plan.Install))) {
			return errors.New(i18n.T("cancelled"))
		}
	}
	if *asJSON && !*yes {
		return errors.New(i18n.T("install --json requires --yes or --dry-run"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := &apps.Runner{Stdout: os.Stderr, Stderr: os.Stderr, Stdin: os.Stdin}
	if !*asJSON {
		runner.OnStart = func(c apps.Command) {
			fmt.Fprintf(os.Stderr, "\n%s %s\n", bold("$"), c.String())
		}
	}
	out.Results = runner.Run(ctx, plan.Commands)
	if *asJSON {
		return writeJSON(out)
	}

	var failed []string
	for _, r := range out.Results {
		failed = append(failed, r.Failed...)
	}
	fmt.Println()
	if len(failed) == 0 {
		fmt.Printf("%s%s\n", report.Pad(i18n.T("Installed"), 14), green(i18n.Tf("✓ %d applications", len(plan.Install))))
		return nil
	}
	fmt.Printf("%s%s\n", report.Pad(i18n.T("Failed"), 14), red(i18n.Tf("%d packages did not install:", len(failed))))
	for _, f := range failed {
		fmt.Printf("%s%s\n", report.Pad("", 16), f)
	}
	return errors.New(i18n.T("some applications did not install"))
}

// onePositional parses the flags of a command that takes exactly one package.
func onePositional(fs *flag.FlagSet, args []string, name string) (string, error) {
	var positional []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return "", nil
			}
			return "", errUsage
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	if len(positional) != 1 {
		return "", fmt.Errorf(i18n.T("%s: give exactly one package"), name)
	}
	return positional[0], nil
}

// loadAppsPlan opens the package, reads its manifest, detects this system and builds the plan.
// skip leaves applications out of the install list (their configuration decisions follow).
func loadAppsPlan(dir, passFile string, noPrompt bool, skip map[string]bool) (*appsOutput, apps.Target, error) {
	if dir == "" {
		return nil, apps.Target{}, nil
	}
	r, err := openPackage(dir, passFile, noPrompt)
	if err != nil {
		return nil, apps.Target{}, err
	}
	ctx := context.Background()
	m, err := apps.ReadManifest(ctx, r)
	if err != nil {
		if errors.Is(err, apps.ErrNoManifest) {
			return nil, apps.Target{}, errors.New(i18n.T("this package has no application manifest: it was made with --no-apps, or by a DHS from before applications"))
		}
		return nil, apps.Target{}, err
	}
	db, err := appdb.Load()
	if err != nil {
		return nil, apps.Target{}, err
	}
	info, err := system.Detect()
	if err != nil {
		return nil, apps.Target{}, fmt.Errorf(i18n.T("cannot detect the system: %w"), err)
	}
	here, err := apps.Detect(db, info)
	if err != nil {
		return nil, apps.Target{}, err
	}
	target := apps.TargetFrom(info, here)
	if len(skip) > 0 {
		kept := m.Apps[:0:0]
		for _, a := range m.Apps {
			if !skip[a.ID] {
				kept = append(kept, a)
			}
		}
		m.Apps = kept
	}
	plan := apps.BuildPlan(m, target, db)
	return &appsOutput{Package: dir, Source: r.Manifest.Source, Target: info, Manifest: m, Plan: plan}, target, nil
}

// printAppPlan shows the plan to a person. withCommands adds the exact commands at the end.
func printAppPlan(o *appsOutput, withCommands bool) {
	const w = 14
	p, m := o.Plan, o.Manifest
	fmt.Printf("%s%s %s → %s\n", report.Pad(i18n.T("Migration"), w), o.Source.Name, o.Source.Version, o.Target)
	fmt.Printf(i18n.T("%s%d in the package · %d recognised · %d unknown\n"), report.Pad(i18n.T("Applications"), w),
		len(m.Apps)+len(m.Unknown), len(m.Apps), len(m.Unknown))

	if len(p.Install) > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.Tf("Install (%d)", len(p.Install))))
		for _, it := range p.Install {
			extra := ""
			if len(it.For) > 0 {
				extra = dim(i18n.T("  stands in for ") + strings.Join(names(m, it.For), ", "))
			}
			fmt.Printf("  %s %s %s%s\n", report.Pad(it.Name, 30), report.Pad(string(it.Manager), 8), it.Package, extra)
		}
	}
	if len(p.Present) > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.Tf("Already here (%d)", len(p.Present))))
		for _, it := range p.Present {
			extra := ""
			if len(it.For) > 0 {
				extra = dim(i18n.T("  stands in for ") + strings.Join(names(m, it.For), ", "))
			}
			fmt.Printf("  %s %s%s\n", report.Pad(it.Name, 30), dim(string(it.Via)), extra)
		}
	}
	if len(p.NoSource) > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.Tf("No way to install here (%d)", len(p.NoSource))))
		for _, it := range p.NoSource {
			needs := make([]string, len(it.Needs))
			for i, n := range it.Needs {
				needs[i] = string(n)
			}
			hint := i18n.T("no known way to install it on this platform")
			if len(needs) > 0 {
				hint = i18n.T("needs one of: ") + strings.Join(needs, ", ")
			}
			fmt.Printf("  %s %s\n", report.Pad(it.Name, 30), dim(hint))
		}
	}
	if len(p.Missing) > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.Tf("Not available on this platform (%d)", len(p.Missing))))
		for _, it := range p.Missing {
			fmt.Printf("  %s %s\n", report.Pad(it.Name, 30), dim(i18n.T(it.Reason)))
		}
	}
	if len(p.Unknown) > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.Tf("Unknown (%d)", len(p.Unknown))) + dim(i18n.T("  — not in the database; nothing is installed for them")))
		shown := 0
		for _, u := range p.Unknown {
			if shown == 12 {
				fmt.Printf("  %s\n", dim(i18n.Tf("… and %d more (see --json)", len(p.Unknown)-12)))
				break
			}
			name := u.Name
			if name == "" {
				name = u.Package
			}
			ver := ""
			if u.Version != "" {
				ver = " " + u.Version
			}
			fmt.Printf("  %s %s\n", report.Pad(name+ver, 40), dim(string(u.Manager)))
			shown++
		}
	}

	placed, aside := 0, 0
	for _, c := range p.Configs {
		if c.Action == apps.Place {
			placed++
		} else {
			aside++
		}
	}
	if placed+aside > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.T("Configuration")) + dim(i18n.Tf("  %d in place · %d kept aside under DHS-restored/apps", placed, aside)))
		for _, c := range p.Configs {
			where := dim("→ " + c.Dest)
			if c.Action == apps.Aside {
				where = dim(i18n.T("aside: ") + i18n.T(c.Reason))
			}
			fmt.Printf("  %s %s\n", report.Pad(c.ID+"/"+c.Key+variantSuffix(c.Variant), 34), where)
		}
	}

	if withCommands && len(p.Commands) > 0 {
		fmt.Println()
		fmt.Println(bold(i18n.T("Commands")))
		for _, c := range p.Commands {
			fmt.Printf("  %s\n", c.String())
		}
	}
	fmt.Println()
}

func variantSuffix(v apps.Variant) string {
	if v == "" {
		return ""
	}
	return "@" + string(v)
}

// names maps ids to display names, using the manifest.
func names(m *apps.Manifest, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if f := m.Get(id); f != nil {
			out = append(out, f.Name)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// printAppsSummary is the one-paragraph form used by scan and backup.
func printAppsSummary(inv *apps.Inventory, w int) {
	if inv == nil {
		return
	}
	configs := 0
	for _, a := range inv.Apps {
		configs += len(a.Config)
	}
	fmt.Printf(i18n.T("%s%d recognised (%d installed, %d by configuration only) · %d configuration locations · %d unknown\n"),
		report.Pad(i18n.T("Applications"), w), len(inv.Apps), inv.Installed(), len(inv.Apps)-inv.Installed(), configs, len(inv.Unknown))
	if len(inv.Apps) > 0 {
		byCat := map[string][]string{}
		for _, a := range inv.Apps {
			byCat[a.Category] = append(byCat[a.Category], a.Name)
		}
		cats := make([]string, 0, len(byCat))
		for c := range byCat {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, c := range cats {
			list := strings.Join(byCat[c], ", ")
			if len(list) > 90 {
				list = list[:87] + "…"
			}
			fmt.Printf("%s%s %s\n", report.Pad("", w+2), report.Pad(c, 16), dim(list))
		}
	}
	if len(inv.Errors) > 0 {
		fmt.Printf("%s%s\n", report.Pad("", w), warn(i18n.T("could not query: ")+strings.Join(inv.Errors, "; ")))
	}
}
