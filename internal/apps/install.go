package apps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Necta14/dhs/appdb"
)

// Command is one command the plan runs, exactly as shown to the user before approval (D5).
type Command struct {
	Manager  appdb.Manager `json:"manager"`
	Argv     []string      `json:"argv"`
	Env      []string      `json:"env,omitempty"`
	Packages []string      `json:"packages,omitempty"` // what it installs; empty for a preparatory step
	// Prep marks a step that installs nothing itself: refreshing the index, adding Flathub.
	Prep bool `json:"prep,omitempty"`
	// Batch says the command takes several packages at once; on failure it is retried one by one.
	Batch bool `json:"batch,omitempty"`
}

// String renders the command the way a user would type it.
func (c Command) String() string {
	parts := make([]string, 0, len(c.Env)+len(c.Argv))
	parts = append(parts, c.Env...)
	for _, a := range c.Argv {
		if strings.ContainsAny(a, " \t\"'") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// Commands turns the install list into commands, grouped by manager in the target's order of
// preference. Batch managers get one command for all their packages; winget and snap take one
// package per command.
func Commands(items []InstallItem, t Target) []Command {
	byMgr := map[appdb.Manager][]string{}
	for _, it := range items {
		byMgr[it.Manager] = append(byMgr[it.Manager], it.Package)
	}
	order := append([]appdb.Manager(nil), t.Managers...)
	for _, m := range appdb.Managers { // managers the target did not list still get their commands
		if _, ok := byMgr[m]; ok && !containsManager(order, m) {
			order = append(order, m)
		}
	}
	sudo := elevator()
	root := func(argv ...string) []string {
		if sudo == "" || isAdmin() {
			return argv
		}
		return append([]string{sudo}, argv...)
	}

	var out []Command
	for _, m := range order {
		pkgs := byMgr[m]
		if len(pkgs) == 0 {
			continue
		}
		switch m {
		case appdb.Pacman:
			// -Syu, not -Sy: Arch does not support partial upgrades, and a package built against a
			// newer library than the installed one would break on start.
			out = append(out, Command{Manager: m, Argv: root(append([]string{"pacman", "-Syu", "--needed", "--noconfirm"}, pkgs...)...), Packages: pkgs, Batch: true})
		case appdb.AUR:
			helper := t.AURHelper
			if helper == "" {
				helper = "paru"
			}
			out = append(out, Command{Manager: m, Argv: append([]string{helper, "-S", "--needed", "--noconfirm"}, pkgs...), Packages: pkgs, Batch: true})
		case appdb.Apt:
			env := []string{"DEBIAN_FRONTEND=noninteractive"}
			out = append(out, Command{Manager: m, Argv: root("apt-get", "update"), Env: env, Prep: true})
			out = append(out, Command{Manager: m, Argv: root(append([]string{"apt-get", "install", "-y"}, pkgs...)...), Env: env, Packages: pkgs, Batch: true})
		case appdb.Dnf:
			out = append(out, Command{Manager: m, Argv: root(append([]string{"dnf", "install", "-y"}, pkgs...)...), Packages: pkgs, Batch: true})
		case appdb.Zypper:
			out = append(out, Command{Manager: m, Argv: root(append([]string{"zypper", "--non-interactive", "install"}, pkgs...)...), Packages: pkgs, Batch: true})
		case appdb.Apk:
			out = append(out, Command{Manager: m, Argv: root("apk", "update"), Prep: true})
			out = append(out, Command{Manager: m, Argv: root(append([]string{"apk", "add"}, pkgs...)...), Packages: pkgs, Batch: true})
		case appdb.Flatpak:
			out = append(out, Command{Manager: m, Argv: []string{"flatpak", "remote-add", "--if-not-exists", "flathub", "https://dl.flathub.org/repo/flathub.flatpakrepo"}, Prep: true})
			out = append(out, Command{Manager: m, Argv: append([]string{"flatpak", "install", "-y", "--noninteractive", "flathub"}, pkgs...), Packages: pkgs, Batch: true})
		case appdb.Snap:
			for _, p := range pkgs {
				name, flags, _ := strings.Cut(p, " ")
				argv := []string{"snap", "install", name}
				if flags != "" {
					argv = append(argv, strings.Fields(flags)...)
				}
				out = append(out, Command{Manager: m, Argv: root(argv...), Packages: []string{p}})
			}
		case appdb.Winget:
			for _, p := range pkgs {
				out = append(out, Command{Manager: m, Argv: []string{"winget", "install", "--id", p, "--exact", "--source", "winget",
					"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity", "--silent"}, Packages: []string{p}})
			}
		case appdb.Choco:
			out = append(out, Command{Manager: m, Argv: append([]string{"choco", "install", "-y"}, pkgs...), Packages: pkgs, Batch: true})
		case appdb.Scoop:
			out = append(out, Command{Manager: m, Argv: append([]string{"scoop", "install"}, pkgs...), Packages: pkgs, Batch: true})
		}
	}
	return out
}

func containsManager(list []appdb.Manager, m appdb.Manager) bool {
	for _, x := range list {
		if x == m {
			return true
		}
	}
	return false
}

// NeedsElevation says whether the commands include one that wants root and no way to get it was
// found — the user must rerun as root.
func NeedsElevation(cmds []Command) bool {
	if isAdmin() || elevator() != "" {
		return false
	}
	for _, c := range cmds {
		switch c.Manager {
		case appdb.Pacman, appdb.Apt, appdb.Dnf, appdb.Zypper, appdb.Apk, appdb.Snap:
			return true
		}
	}
	return false
}

// IsAdmin reports whether the process already has administrative rights.
func IsAdmin() bool { return isAdmin() }

// Result is the outcome of one command.
type Result struct {
	Command Command  `json:"command"`
	OK      bool     `json:"ok"`
	Exit    int      `json:"exit_code"`
	Error   string   `json:"error,omitempty"`
	Failed  []string `json:"failed_packages,omitempty"` // after the one-by-one retry
}

// Runner executes approved commands, with their output going straight to the user.
type Runner struct {
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	// OnStart is called before every command, so the interface can show what runs.
	OnStart func(Command)
	// Exec overrides process execution — tests use it. It returns the exit code.
	Exec func(ctx context.Context, c Command) (int, error)
}

// Run executes the commands in order. A failed preparatory step stops the manager's later
// commands; a failed batch is retried one package at a time, so the report names exactly what
// did not install. Nothing here is ever run without the user having seen the plan (D5).
func (r *Runner) Run(ctx context.Context, cmds []Command) []Result {
	exec := r.Exec
	if exec == nil {
		exec = r.execProcess
	}
	var results []Result
	skipManager := map[appdb.Manager]bool{}
	for _, c := range cmds {
		if skipManager[c.Manager] {
			results = append(results, Result{Command: c, Exit: -1, Error: "skipped: an earlier step for this manager failed", Failed: c.Packages})
			continue
		}
		if err := ctx.Err(); err != nil {
			results = append(results, Result{Command: c, Exit: -1, Error: err.Error(), Failed: c.Packages})
			continue
		}
		if r.OnStart != nil {
			r.OnStart(c)
		}
		code, err := exec(ctx, c)
		res := Result{Command: c, OK: err == nil && code == 0, Exit: code}
		if err != nil {
			res.Error = err.Error()
		}
		if !res.OK {
			if c.Prep {
				skipManager[c.Manager] = true
			} else if c.Batch && len(c.Packages) > 1 {
				res.Failed = r.retryOneByOne(ctx, c, exec)
				res.OK = len(res.Failed) == 0
			} else {
				res.Failed = c.Packages
			}
		}
		results = append(results, res)
	}
	return results
}

// retryOneByOne re-runs a batch command with one package at a time and returns the ones that fail.
func (r *Runner) retryOneByOne(ctx context.Context, c Command, exec func(context.Context, Command) (int, error)) []string {
	// The package names are the trailing arguments; everything before them is the fixed part.
	fixed := c.Argv[:len(c.Argv)-len(c.Packages)]
	var failed []string
	for _, p := range c.Packages {
		single := Command{Manager: c.Manager, Env: c.Env, Packages: []string{p}}
		single.Argv = append(append([]string(nil), fixed...), p)
		if r.OnStart != nil {
			r.OnStart(single)
		}
		if code, err := exec(ctx, single); err != nil || code != 0 {
			failed = append(failed, p)
		}
	}
	return failed
}

func (r *Runner) execProcess(ctx context.Context, c Command) (int, error) {
	if len(c.Argv) == 0 {
		return -1, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = r.Stdout, r.Stderr, r.Stdin
	if len(c.Env) > 0 {
		cmd.Env = append(cmd.Environ(), c.Env...)
	}
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), fmt.Errorf("exit status %d", ee.ExitCode())
	}
	return -1, err
}
