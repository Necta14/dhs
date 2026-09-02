package main

import (
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

	"github.com/necta/dhs/internal/pack"
	"github.com/necta/dhs/internal/report"
	"github.com/necta/dhs/internal/scan"
	"github.com/necta/dhs/internal/system"
)

const backupUsage = `dhs backup — creează pachetul de migrare

Utilizare
  dhs backup --dest <cale> [cale...] [opțiuni]

Fără căi, salvează locurile standard din profil. Pachetul e un director <dest>/<nume>.dhs,
format din volume de 3,5 GiB. Înainte să scrie ceva, arată estimarea și cere confirmare.

Opțiuni
  --dest <cale>      unde se scrie pachetul (obligatoriu)
  --nume <nume>      numele directorului pachetului; implicit migrare-<data>-<ora>
  --nivel <1|2|3>    compresie: 1 compatibil, 2 echilibrat (implicit), 3 maxim
  --secrete          include chei, parole și fișiere .env — excluse implicit
  --tot              nu exclude nimic (cache-uri, jocuri, mașini virtuale)
  --fara-criptare    scrie pachetul necriptat. Cu nivelul 1 dă un pachet deschizabil fără DHS
  --parola-fisier <cale>   citește fraza de acces din fișier (pentru automatizare); altfel o cere
  --da               nu cere confirmare
  --verifica         după scriere, verifică integral pachetul (decriptează și verifică fiecare bloc)
  --json             ieșire JSON
  --help
`

type backupOutput struct {
	Package  string             `json:"pachet"`
	Manifest pack.Manifest      `json:"manifest"`
	Files    int64              `json:"fisiere"`
	Bytes    int64              `json:"octeti_brut"`
	Stored   int64              `json:"octeti_stocati"`
	Dedup    int64              `json:"octeti_dedup"`
	Skipped  []skippedFile      `json:"sarite,omitempty"`
	Duration time.Duration      `json:"durata_ns"`
	Verify   *pack.VerifyReport `json:"verificare,omitempty"`
}

type skippedFile struct {
	Path   string `json:"cale"`
	Reason string `json:"motiv"`
}

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, backupUsage) }

	dest := fs.String("dest", "", "")
	name := fs.String("nume", "", "")
	level := fs.Int("nivel", int(scan.LevelBalanced), "")
	secrets := fs.Bool("secrete", false, "")
	all := fs.Bool("tot", false, "")
	noCrypt := fs.Bool("fara-criptare", false, "")
	passFile := fs.String("parola-fisier", "", "")
	yes := fs.Bool("da", false, "")
	verify := fs.Bool("verifica", false, "")
	asJSON := fs.Bool("json", false, "")

	var explicitRoots []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return nil
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		explicitRoots = append(explicitRoots, rest[0])
		rest = rest[1:]
	}

	if *dest == "" {
		return errors.New("backup: --dest e obligatoriu")
	}
	lvl := scan.Level(*level)
	if !lvl.Valid() {
		return fmt.Errorf("nivelul %d nu există; alege 1, 2 sau 3", *level)
	}
	live := !*asJSON && isTerminal(os.Stderr)

	info, err := system.Detect()
	if err != nil {
		return fmt.Errorf("nu pot detecta sistemul: %w", err)
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
		return errors.New("backup: nimic de salvat")
	}

	// Destinația: există, e director, are loc.
	destInfo, err := os.Stat(*dest)
	if err != nil {
		return fmt.Errorf("destinația %s: %w", *dest, err)
	}
	if !destInfo.IsDir() {
		return fmt.Errorf("destinația %s nu e un director", *dest)
	}
	vol, err := system.SpaceOf(*dest)
	if err != nil {
		return fmt.Errorf("nu pot citi spațiul de pe %s: %w", *dest, err)
	}

	// 1. Inventar — același cu dhs scan, dar reținem intrările ca să le împachetăm.
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

	// 2. Arată ce urmează și cere confirmare — înainte de parolă, ca să nu o ceri degeaba.
	if !*asJSON {
		printScan(scanOutput{System: info, Inventory: inv, Estimate: est, Dest: &vol, Fits: &fits}, 0)
		fmt.Println()
	}
	if !fits {
		return fmt.Errorf("nu încape: lipsesc %s pe %s", report.Bytes(-left), *dest)
	}
	if inv.Files == 0 {
		return errors.New("backup: inventarul e gol")
	}
	if !*yes && !*asJSON {
		if !confirm(fmt.Sprintf("Scriu %s fișiere (%s) în %s. Continui?", report.Count(inv.Files), report.Bytes(inv.Bytes), *dest)) {
			return errors.New("anulat")
		}
	}

	// 3. Fraza de acces.
	pass := ""
	if !*noCrypt {
		switch {
		case *passFile != "":
			b, err := os.ReadFile(*passFile)
			if err != nil {
				return fmt.Errorf("nu pot citi fraza din %s: %w", *passFile, err)
			}
			pass = strings.TrimRight(string(b), "\r\n")
		case *asJSON:
			return errors.New("backup --json cere --parola-fisier sau --fara-criptare")
		default:
			pass, err = askNewPassphrase()
			if err != nil {
				return err
			}
		}
	} else if !*yes && !*asJSON {
		fmt.Fprintln(os.Stderr, warn("Pachetul va fi NECRIPTAT: oricine pune mâna pe disc îți citește fișierele."))
		if !confirm("Continui fără criptare?") {
			return errors.New("anulat")
		}
	}

	// 4. Scrierea.
	if *name == "" {
		*name = "migrare-" + time.Now().Format("2006-01-02-1504")
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
			fmt.Fprintf(os.Stderr, "\r\033[K  %s / %s fișiere · %s citiți · %s scriși · volumul %d",
				report.Count(p.Files), report.Count(inv.Files), report.Bytes(p.Bytes), report.Bytes(p.Stored), p.Volume)
		},
	})
	if err != nil {
		return err
	}

	rm := pack.NewRootMap(info.Home, locs)
	var skipped []skippedFile
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			w.Abort(err)
			return fmt.Errorf("întrerupt; pachetul %s e incomplet și poate fi șters", pkgDir)
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
				return fmt.Errorf("scrierea a eșuat: %w", err)
			}
			// Eroare de citire pe un singur fișier: îl sărim și mergem mai departe.
			skipped = append(skipped, skippedFile{e.Path, err.Error()})
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("închiderea pachetului a eșuat: %w", err)
	}
	if live {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	elapsed := time.Since(start)

	// 5. Recitire.
	r, err := pack.Open(pkgDir, pass)
	if err != nil {
		return fmt.Errorf("pachetul s-a scris dar nu se poate redeschide: %w", err)
	}
	out := backupOutput{Package: pkgDir, Manifest: r.Manifest, Files: r.Manifest.Files, Bytes: r.Manifest.Raw, Stored: r.Manifest.Stored, Skipped: skipped, Duration: elapsed}
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
	fmt.Printf("%s%s\n", report.Pad("Pachet", 14), pkgDir)
	fmt.Printf("%s%s fișiere · %s → %s pe disc · %d volume · %s\n", report.Pad("Scris", 14),
		report.Count(out.Files), report.Bytes(out.Bytes), report.Bytes(out.Stored), r.Manifest.Volumes, report.Duration(elapsed))
	if len(skipped) > 0 {
		fmt.Printf("%s%d fișiere n-au putut fi citite (vezi --json pentru listă)\n", report.Pad("Sărite", 14), len(skipped))
	}
	if out.Verify != nil {
		if out.Verify.OK() {
			fmt.Printf("%s%s\n", report.Pad("Verificat", 14), green("✓ toate blocurile și sumele sunt în regulă"))
		} else {
			fmt.Printf("%s%s\n", report.Pad("Verificat", 14), red(fmt.Sprintf("✗ %d probleme", len(out.Verify.Problems))))
			for _, p := range out.Verify.Problems {
				fmt.Printf("%s%s\n", report.Pad("", 16), p)
			}
			return errors.New("pachetul nu a trecut verificarea")
		}
	} else {
		fmt.Printf("%s%s\n", report.Pad("", 14), dim("dhs verify "+pkgDir+"  — verifică integral, oricând"))
	}
	if pass != "" {
		fmt.Println()
		fmt.Println(warn("Fără fraza de acces, pachetul e pierdut definitiv. N-o putem recupera."))
	}
	return nil
}

func verifyProgress(live bool) func(done, total int64) {
	if !live {
		return nil
	}
	return func(done, total int64) {
		fmt.Fprintf(os.Stderr, "\r\033[K  verific… %s / %s", report.Bytes(done), report.Bytes(total))
		if done >= total {
			fmt.Fprint(os.Stderr, "\r\033[K")
		}
	}
}
