package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Necta14/dhs/appdb"
	"github.com/Necta14/dhs/internal/apps"
	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/report"
	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

const backupUsage = `dhs backup — create the migration package

Usage
  dhs backup --dest <path> [path...] [options]

Without paths, saves the standard places in the profile. The package is a directory
<dest>/<name>.dhs, made of 3.5 GiB volumes. Before writing anything, it shows the estimate and
asks for confirmation.

Options
  --dest <path>       where the package is written (required)
  --name <name>       name of the package directory; default migration-<date>-<time>
  --level <1|2|3>     compression: 1 compatible, 2 balanced (default), 3 maximum
  --secrets           include keys, passwords and .env files — excluded by default
  --all               exclude nothing (caches, games, virtual machines)
  --no-apps           do not detect applications and do not carry their configuration
  --no-encrypt        write the package unencrypted. With level 1 it gives a package that opens without DHS
  --passphrase-file <path>   read the passphrase from a file (for automation); otherwise it is asked
  --yes               do not ask for confirmation
  --verify            after writing, verify the whole package (decrypt and check every block)
  --json              JSON output
  --help
`

type backupOutput struct {
	Package  string             `json:"package"`
	Manifest pack.Manifest      `json:"manifest"`
	Files    int64              `json:"files"`
	Bytes    int64              `json:"raw_bytes"`
	Stored   int64              `json:"stored_bytes"`
	Dedup    int64              `json:"dedup_bytes"`
	Skipped  []skippedFile      `json:"skipped,omitempty"`
	Apps     *apps.Inventory    `json:"apps,omitempty"`
	Duration time.Duration      `json:"duration_ns"`
	Verify   *pack.VerifyReport `json:"verification,omitempty"`
}

type skippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.T(backupUsage)) }

	dest := fs.String("dest", "", "")
	name := fs.String("name", "", "")
	level := fs.Int("level", int(scan.LevelBalanced), "")
	secrets := fs.Bool("secrets", false, "")
	all := fs.Bool("all", false, "")
	noApps := fs.Bool("no-apps", false, "")
	noCrypt := fs.Bool("no-encrypt", false, "")
	passFile := fs.String("passphrase-file", "", "")
	yes := fs.Bool("yes", false, "")
	verify := fs.Bool("verify", false, "")
	asJSON := fs.Bool("json", false, "")

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

	if *dest == "" {
		return errors.New(i18n.T("backup: --dest is required"))
	}
	lvl := scan.Level(*level)
	if !lvl.Valid() {
		return fmt.Errorf(i18n.T("level %d does not exist; choose 1, 2 or 3"), *level)
	}
	live := !*asJSON && isTerminal(os.Stderr)

	info, err := system.Detect()
	if err != nil {
		return fmt.Errorf(i18n.T("cannot detect the system: %w"), err)
	}
	locs := system.Locations(info)
	roots := explicitRoots
	if len(roots) == 0 {
		var paths []string
		for _, l := range locs {
			if l.Exists {
				paths = append(paths, l.Path)
			}
		}
		roots = scan.HomeRoots(paths)
	}
	if len(roots) == 0 {
		return errors.New(i18n.T("backup: nothing to save"))
	}

	// The destination: exists, is a directory, has room.
	destInfo, err := os.Stat(*dest)
	if err != nil {
		return fmt.Errorf(i18n.T("destination %s: %w"), *dest, err)
	}
	if !destInfo.IsDir() {
		return fmt.Errorf(i18n.T("destination %s is not a directory"), *dest)
	}
	vol, err := system.SpaceOf(*dest)
	if err != nil {
		return fmt.Errorf(i18n.T("cannot read the free space on %s: %w"), *dest, err)
	}

	// 0. Applications: what is installed, and where their configuration lives. The locations
	// become roots of their own ("apps/<id>/<key>"), so that the files can be placed on another
	// system by what they are, not by where they happened to be.
	rm := pack.NewRootMap(info.Home, locs)
	var appInv *apps.Inventory
	var db *appdb.DB
	if !*noApps {
		db, err = appdb.Load()
		if err != nil {
			return err
		}
		appInv, err = apps.Detect(db, info)
		if err != nil {
			return fmt.Errorf(i18n.T("cannot detect the applications: %w"), err)
		}
		var extra []string
		for _, a := range appInv.Apps {
			for _, l := range a.Config {
				rm.Add(l.Root(a.ID), l.Path)
				if !insideAny(l.Path, roots) {
					extra = append(extra, l.Path)
				}
			}
		}
		roots = append(roots, scan.HomeRoots(extra)...)
		roots = append(roots, filesOnly(extra)...)
	}

	// 1. Inventory — the same as dhs scan, but we keep the entries in order to pack them.
	var entries []scan.Entry
	sizes := make(map[int64]int)
	opts := scan.Options{
		Roots:          roots,
		IncludeSecrets: *secrets,
		OnEntry: func(e scan.Entry) {
			entries = append(entries, e)
			sizes[e.Size]++
		},
	}
	if appInv != nil {
		opts.Filter = appInv.Filter(db)
	}
	if *all {
		opts.Excluder = scan.NewExcluder(nil)
	}
	if live {
		opts.OnProgress = progressWriter()
	}
	inv, err := scan.Walk(opts)
	if err != nil {
		return err
	}
	if live {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	est := scan.Estimator{Workers: runtime.NumCPU()}.Estimate(inv, lvl)
	fits, left := est.Fits(vol.Free)

	// 2. Show what comes next and ask for confirmation — before the passphrase, so it is not asked in vain.
	if !*asJSON {
		printScan(scanOutput{System: info, Inventory: inv, Estimate: est, Dest: &vol, Fits: &fits, Apps: appInv}, 0)
		fmt.Println()
	}
	if !fits {
		return fmt.Errorf(i18n.T("does not fit: %s missing on %s"), report.Bytes(-left), *dest)
	}
	if inv.Files == 0 {
		return errors.New(i18n.T("backup: the inventory is empty"))
	}
	if !*yes && !*asJSON {
		if !confirm(i18n.Tf("Writing %s files (%s) to %s. Continue?", report.Count(inv.Files), report.Bytes(inv.Bytes), *dest)) {
			return errors.New(i18n.T("cancelled"))
		}
	}

	// 3. The passphrase.
	pass := ""
	if !*noCrypt {
		switch {
		case *passFile != "":
			b, err := os.ReadFile(*passFile)
			if err != nil {
				return fmt.Errorf(i18n.T("cannot read the passphrase from %s: %w"), *passFile, err)
			}
			pass = strings.TrimRight(string(b), "\r\n")
		case *asJSON:
			return errors.New(i18n.T("backup --json requires --passphrase-file or --no-encrypt"))
		default:
			pass, err = askNewPassphrase()
			if err != nil {
				return err
			}
		}
	} else if !*yes && !*asJSON {
		fmt.Fprintln(os.Stderr, warn(i18n.T("The package will be UNENCRYPTED: anyone who gets hold of the drive can read your files.")))
		if !confirm(i18n.T("Continue without encryption?")) {
			return errors.New(i18n.T("cancelled"))
		}
	}

	// 4. Writing.
	if *name == "" {
		*name = "migration-" + time.Now().Format("2006-01-02-1504")
	}
	pkgDir := filepath.Join(*dest, strings.TrimSuffix(*name, ".dhs")+".dhs")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	var lastLine time.Time
	w, err := pack.NewWriter(pack.Options{
		Dir:        pkgDir,
		Level:      lvl,
		Passphrase: pass,
		Source:     info,
		Tool:       "dhs " + version,
		OnProgress: func(p pack.Progress) {
			if !live || time.Since(lastLine) < 200*time.Millisecond {
				return
			}
			lastLine = time.Now()
			fmt.Fprintf(os.Stderr, i18n.T("\r\033[K  %s / %s files · %s read · %s written · volume %d"),
				report.Count(p.Files), report.Count(inv.Files), report.Bytes(p.Bytes), report.Bytes(p.Stored), p.Volume)
		},
	})
	if err != nil {
		return err
	}

	var skipped []skippedFile
	if appInv != nil {
		// The application manifest goes in first, so it sits in the first volume.
		mb, err := apps.NewManifest(appInv, db.Len(), time.Now()).Encode()
		if err != nil {
			return err
		}
		err = w.Add(ctx, pack.File{
			Root: pack.RootMeta, Path: apps.ManifestPath, Orig: "", Size: int64(len(mb)), Mode: 0o644, ModTime: time.Now(),
			Class: scan.Text, Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(mb)), nil },
		})
		if err != nil {
			w.Abort(err)
			return fmt.Errorf(i18n.T("writing failed: %w"), err)
		}
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			w.Abort(err)
			return fmt.Errorf(i18n.T("interrupted; the package %s is incomplete and can be deleted"), pkgDir)
		}
		root, rel := rm.Split(e.Path)
		st, err := os.Lstat(e.Path)
		if err != nil {
			skipped = append(skipped, skippedFile{e.Path, err.Error()})
			continue
		}
		path := e.Path
		err = w.Add(ctx, pack.File{
			Root: root, Path: rel, Orig: e.Path,
			Size: st.Size(), Mode: uint32(st.Mode().Perm()), ModTime: st.ModTime(),
			Class: e.Class, Secret: e.Secret != "",
			MaybeDup: sizes[e.Size] > 1,
			Open:     func() (io.ReadCloser, error) { return os.Open(path) },
		})
		if err != nil {
			if w.Err() != nil {
				w.Abort(err)
				return fmt.Errorf(i18n.T("writing failed: %w"), err)
			}
			// Read error on a single file: skip it and move on.
			skipped = append(skipped, skippedFile{e.Path, err.Error()})
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf(i18n.T("closing the package failed: %w"), err)
	}
	if live {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	elapsed := time.Since(start)

	// 5. Re-read.
	r, err := pack.Open(pkgDir, pass)
	if err != nil {
		return fmt.Errorf(i18n.T("the package was written but cannot be reopened: %w"), err)
	}
	out := backupOutput{Package: pkgDir, Manifest: r.Manifest, Files: r.Manifest.Files, Bytes: r.Manifest.Raw, Stored: r.Manifest.Stored, Skipped: skipped, Apps: appInv, Duration: elapsed}
	if *verify {
		rep, err := r.Verify(ctx, verifyProgress(live))
		if err != nil {
			return err
		}
		out.Verify = rep
	}

	if *asJSON {
		return writeJSON(out)
	}
	fmt.Printf("%s%s\n", report.Pad(i18n.T("Package"), 14), pkgDir)
	fmt.Printf(i18n.T("%s%s files · %s → %s on disk · %d volumes · %s\n"), report.Pad(i18n.T("Written"), 14),
		report.Count(out.Files), report.Bytes(out.Bytes), report.Bytes(out.Stored), r.Manifest.Volumes, report.Duration(elapsed))
	if len(skipped) > 0 {
		fmt.Printf(i18n.T("%s%d files could not be read (see --json for the list)\n"), report.Pad(i18n.T("Skipped"), 14), len(skipped))
	}
	if appInv != nil {
		fmt.Printf(i18n.T("%s%d recognised · %d unknown · dhs plan %s shows what would be installed elsewhere\n"),
			report.Pad(i18n.T("Applications"), 14), len(appInv.Apps), len(appInv.Unknown), pkgDir)
	}
	if out.Verify != nil {
		if out.Verify.OK() {
			fmt.Printf("%s%s\n", report.Pad(i18n.T("Verified"), 14), green(i18n.T("✓ all blocks and checksums are fine")))
		} else {
			fmt.Printf("%s%s\n", report.Pad(i18n.T("Verified"), 14), red(i18n.Tf("✗ %d problems", len(out.Verify.Problems))))
			for _, p := range out.Verify.Problems {
				fmt.Printf("%s%s\n", report.Pad("", 16), p)
			}
			return errors.New(i18n.T("the package did not pass verification"))
		}
	} else {
		fmt.Printf("%s%s\n", report.Pad("", 14), dim("dhs verify "+pkgDir+i18n.T("  — verifies the whole package, any time")))
	}
	if pass != "" {
		fmt.Println()
		fmt.Println(warn(i18n.T("Without the passphrase the package is lost for good. We cannot recover it.")))
	}
	return nil
}

func verifyProgress(live bool) func(done, total int64) {
	if !live {
		return nil
	}
	return func(done, total int64) {
		fmt.Fprintf(os.Stderr, i18n.T("\r\033[K  verifying… %s / %s"), report.Bytes(done), report.Bytes(total))
		if done >= total {
			fmt.Fprint(os.Stderr, "\r\033[K")
		}
	}
}

// insideAny says whether path lies under one of the roots already being scanned.
func insideAny(path string, roots []string) bool {
	for _, r := range roots {
		if _, ok := apps.Under(path, r); ok {
			return true
		}
	}
	return false
}

// filesOnly keeps the paths that are regular files: HomeRoots drops them, and a configuration
// location may well be a single file (~/.gitconfig).
func filesOnly(paths []string) []string {
	var out []string
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}
