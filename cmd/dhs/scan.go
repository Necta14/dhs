package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/apps"
	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/report"
	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

const scanUsage = `dhs scan — show what would be saved and how much it takes, without writing anything

Usage
  dhs scan [path...] [options]

Without paths, scans the standard places in the profile: documents, pictures, videos, music,
downloads, desktop and configuration.

Options
  --dest <path>    package destination; checks whether it fits and which filesystem it has
  --level <1|2|3>  compression level: 1 compatible, 2 balanced (default), 3 maximum
  --secrets        include keys, passwords and .env files (by default they are only counted)
  --all            exclude nothing; also show caches, games, virtual machines
  --no-apps        do not detect applications
  --json           JSON output, for graphical interfaces and scripts
  --help
`

type scanOutput struct {
	System    system.Info     `json:"system"`
	Inventory *scan.Result    `json:"inventory"`
	Estimate  scan.Estimate   `json:"estimate"`
	Dest      *system.Volume  `json:"destination,omitempty"`
	Fits      *bool           `json:"fits,omitempty"`
	Apps      *apps.Inventory `json:"apps,omitempty"`
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(scanUsage)) }

	dest := fs.String("dest", "", "package destination")
	level := fs.Int("level", int(scan.LevelBalanced), "compression level (1-3)")
	secrets := fs.Bool("secrets", false, "include secrets")
	all := fs.Bool("all", false, "exclude nothing")
	noApps := fs.Bool("no-apps", false, "do not detect applications")
	asJSON := fs.Bool("json", false, "JSON output")

	// The flag package stops at the first path, so "dhs scan ~/Documents --json" would take
	// "--json" as a path. Parse in a loop, collecting paths as they appear, so order does not matter.
	var explicitRoots []string
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
		explicitRoots = append(explicitRoots, rest[0])
		rest = rest[1:]
	}

	lvl := scan.Level(*level)
	if !lvl.Valid() {
		return fmt.Errorf(i18n.T("level %d does not exist; choose 1, 2 or 3"), *level)
	}

	info, err := system.Detect()
	if err != nil {
		return fmt.Errorf(i18n.T("cannot detect the system: %w"), err)
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
			return fmt.Errorf(i18n.T("no standard place found in %s; give the paths explicitly"), info.Home)
		}
	}

	opts := scan.Options{Roots: roots, IncludeSecrets: *secrets}
	if *all {
		opts.Excluder = scan.NewExcluder(nil)
	}
	// Progress is written only to a terminal: redirected, it would fill the file with ANSI codes.
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
	if !*noApps {
		db, err := appdb.Load()
		if err != nil {
			return err
		}
		out.Apps, err = apps.Detect(db, info)
		if err != nil {
			return fmt.Errorf(i18n.T("cannot detect the applications: %w"), err)
		}
	}
	if *dest != "" {
		v, err := system.SpaceOf(*dest)
		if err != nil {
			return fmt.Errorf(i18n.T("cannot read the destination %s: %w"), *dest, err)
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
		fmt.Fprintf(os.Stderr, i18n.T("\r\033[K  reading… %s files, %s"), report.Count(files), report.Bytes(bytes))
	}
}

func printScan(o scanOutput, elapsed time.Duration) {
	inv, est := o.Inventory, o.Estimate
	const w = 14

	fmt.Printf("%s%s\n", report.Pad(i18n.T("System"), w), o.System)
	fmt.Printf("%s%s\n", report.Pad(i18n.T("Roots"), w), strings.Join(inv.Roots, "\n"+report.Pad("", w)))
	fmt.Println()

	fmt.Printf(i18n.T("%s%s in %s files   %s\n"),
		report.Pad(i18n.T("To include"), w), report.Bytes(inv.Bytes), report.Count(inv.Files),
		dim(i18n.Tf("(scanned in %s)", report.Duration(elapsed))))

	// What was excluded and why — the user must be able to contest every missing gigabyte.
	if n := inv.SkippedBytes(); n > 0 {
		fmt.Printf("%s%s   %s\n", report.Pad(i18n.T("Excluded"), w), report.Bytes(n), dim(i18n.T("--all brings them back")))
		for i, s := range inv.Skipped {
			if i == 4 {
				fmt.Printf("%s%s\n", report.Pad("", w+2), dim(i18n.Tf("… and %d more rules", len(inv.Skipped)-4)))
				break
			}
			fmt.Printf("%s%s %s\n", report.Pad("", w+2),
				report.Pad(s.Dir, 22), dim(fmt.Sprintf("%s · %s", report.Bytes(s.Bytes), i18n.T(s.Reason))))
		}
	}
	if inv.Secrets.Files > 0 {
		fmt.Printf(i18n.T("%s%s in %s files   %s\n"), report.Pad(i18n.T("Secrets"), w),
			report.Bytes(inv.Secrets.Bytes), report.Count(inv.Secrets.Files),
			dim(i18n.T("excluded by default; --secrets includes them")))
	}
	fmt.Println()

	// Composition matters: if everything is already compressed, level 3 has nothing to gain.
	classes := make([]scan.Class, 0, len(inv.ByClass))
	for c := range inv.ByClass {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool {
		return inv.ByClass[classes[i]].Bytes > inv.ByClass[classes[j]].Bytes
	})
	fmt.Println(i18n.T("Composition"))
	for _, c := range classes {
		b := inv.ByClass[c]
		pct := 0.0
		if inv.Bytes > 0 {
			pct = float64(b.Bytes) / float64(inv.Bytes) * 100
		}
		note := ""
		switch c {
		case scan.Incompressible:
			note = dim(i18n.T("stored, not compressed"))
		case scan.Unknown:
			note = dim(i18n.T("settled by an entropy test while packing"))
		}
		fmt.Printf("%s%s %s  %s %s\n", report.Pad("", w+2), report.Pad(i18n.T(c.String()), 16),
			report.Pad(report.Bytes(b.Bytes), 11),
			report.Pad(fmt.Sprintf("%.0f%%", pct), 5), note)
	}
	fmt.Println()

	fmt.Printf("%s%s\n", report.Pad(i18n.T("Level"), w), i18n.T(est.Level.String()))
	fmt.Printf("%s%s   %s\n", report.Pad(i18n.T("Estimate"), w),
		report.Range(est.Min, est.Max), dim(i18n.Tf("%s on %d cores", report.Duration(est.Duration), runtime.NumCPU())))
	if !est.Sampled {
		fmt.Printf("%s%s\n", report.Pad("", w), dim(i18n.T("estimate from table; --precise would measure it by sampling")))
	}
	if est.Volumes > 0 {
		fmt.Printf("%s%d × %s\n", report.Pad(i18n.T("Volumes"), w), est.Volumes, report.Bytes(scan.VolumeSize))
	}

	if o.Dest != nil {
		fmt.Println()
		fmt.Printf(i18n.T("%s%s   %s free of %s   %s\n"), report.Pad(i18n.T("Destination"), w), o.Dest.Path,
			report.Bytes(o.Dest.Free), report.Bytes(o.Dest.Total), dim(i18n.T(string(o.Dest.FS))))
		if limit := o.Dest.FS.MaxFileSize(); limit > 0 {
			fmt.Printf("%s%s\n", report.Pad("", w),
				dim(i18n.Tf("%s limits files to %s — the %s volumes are under the limit",
					o.Dest.FS, report.Bytes(limit), report.Bytes(scan.VolumeSize))))
		}
		fmt.Println()
		if o.Fits != nil && *o.Fits {
			_, left := est.Fits(o.Dest.Free)
			fmt.Printf("%s%s\n", report.Pad("", w),
				green(i18n.Tf("✓ Fits, with %s to spare.", report.Bytes(left))))
		} else {
			missing := est.Max - o.Dest.Free
			fmt.Printf("%s%s\n", report.Pad("", w),
				red(i18n.Tf("✗ Does not fit: %s missing.", report.Bytes(missing))))
			fmt.Printf("%s%s\n", report.Pad("", w), dim(i18n.T("exclude some of the largest items, raise the compression level, or split the package across several drives")))
		}
	}

	if len(inv.Largest) > 0 {
		fmt.Println()
		fmt.Println(i18n.T("Largest"))
		for i, e := range inv.Largest {
			if i == 5 {
				break
			}
			fmt.Printf("%s%s %s\n", report.Pad("", w+2), report.Pad(report.Bytes(e.Size), 11), dim(e.Path))
		}
	}

	if n := len(inv.Errors); n > 0 {
		fmt.Println()
		fmt.Printf("%s%s\n", report.Pad(i18n.T("Inaccessible"), w), dim(i18n.Tf("%d places skipped (missing permissions)", n)))
	}
	if o.Apps != nil {
		fmt.Println()
		printAppsSummary(o.Apps, w)
	}
}

// Colors apply only on a terminal; in a pipe, plain characters come out.
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
