#!/usr/bin/env python3
"""DHS GUI — Linux prototype (GTK4 + libadwaita, via PyGObject).

A separate process that drives the `dhs` CLI through its `--json` output — decision D9:
the core stays in one place, each platform gets its native shell. This prototype exists so the
maintainer can try the flows by hand; the production Linux GUI may be rewritten in Go (gotk4).

Run:  python3 gui/linux/dhs-gui.py
Finds the binary via $DHS_BIN, then ./dhs at the repo root, then `dhs` on PATH.
"""

import json
import locale
import os
import shutil
import subprocess
import sys
import tempfile
import threading
from pathlib import Path

import gi

gi.require_version("Gtk", "4.0")
gi.require_version("Adw", "1")
from gi.repository import Adw, Gio, GLib, Gtk  # noqa: E402

# ─────────────────────────── i18n ───────────────────────────

RO = {
    "DHS — Direct Handoff Suite": "DHS — Direct Handoff Suite",
    "Scan": "Scanare", "Backup": "Backup", "Restore": "Restaurare",
    "Source": "Sursă", "Folders to include": "Dosare de inclus",
    "Default: Documents, Pictures, Videos, Music, Downloads, Desktop, config": "Implicit: Documente, Imagini, Video, Muzică, Descărcări, Desktop, config",
    "Choose…": "Alege…", "Destination drive": "Mediul destinație", "Not chosen": "Nealeasă",
    "Compression": "Compresie", "1 · Compatible": "1 · Compatibil", "2 · Balanced": "2 · Echilibrat", "3 · Maximum": "3 · Maxim",
    "Include secrets": "Include secretele", "SSH/GPG keys, browser passwords, .env files": "Chei SSH/GPG, parole de browser, fișiere .env",
    "Include everything": "Include tot", "No exclusions: caches, games, virtual machines": "Fără excluderi: cache-uri, jocuri, mașini virtuale",
    "Run scan": "Scanează", "Scanning…": "Scanez…", "Results": "Rezultate",
    "System": "Sistem", "To include": "De inclus", "Excluded": "Excluse", "Secrets": "Secrete",
    "Composition": "Compoziție", "Estimate": "Estimare", "Volumes": "Volume", "Destination": "Destinație",
    "files": "fișiere", "Fits, with {} to spare": "Încape, cu {} de rezervă", "Does not fit: {} short": "Nu încape: lipsesc {}",
    "unknown": "necunoscut", "incompressible": "incompresibil", "binary": "binar", "text": "text",
    "Package name": "Numele pachetului", "Encrypt with a passphrase": "Criptează cu frază de acces",
    "Passphrase": "Fraza de acces", "Repeat passphrase": "Încă o dată",
    "Verify after writing": "Verifică după scriere", "Decrypts and checks every block": "Decriptează și verifică fiecare bloc",
    "Create package": "Creează pachetul", "Writing…": "Scriu…",
    "Passphrases do not match": "Frazele nu se potrivesc", "Passphrase must have at least 8 characters": "Fraza trebuie să aibă cel puțin 8 caractere",
    "Choose a destination first": "Alege întâi destinația",
    "If you lose the passphrase, the package is lost for good. There is no recovery.": "Dacă pierzi fraza de acces, pachetul e pierdut definitiv. Nu există recuperare.",
    "Package": "Pachet", "Written": "Scris", "Verified": "Verificat", "all blocks and sums OK": "toate blocurile și sumele sunt în regulă",
    "Package folder": "Dosarul pachetului", "When a file already exists": "Când fișierul există deja",
    "keep-both": "păstrează ambele", "skip": "sari", "overwrite": "suprascrie",
    "Keep existing, write restored beside it with “ (DHS)”": "Păstrează existentul, scrie restauratul alături, cu „ (DHS)”",
    "Show plan": "Arată planul", "Planning…": "Calculez planul…", "Plan": "Plan",
    "Migration": "Migrare", "To write": "De scris", "Conflicts": "Conflicte", "Renamed": "Redenumite",
    "Restore now": "Restaurează acum", "Restoring…": "Restaurez…", "Restored": "Restaurat",
    "All files verified and put in place": "Toate fișierele au fost verificate și puse la loc",
    "{} files could not be restored": "{} fișiere nu s-au putut restaura",
    "Error": "Eroare", "OK": "OK", "Cancel": "Renunță", "Continue": "Continuă",
    "dhs binary not found. Set DHS_BIN or build it: go build -o dhs ./cmd/dhs": "Nu găsesc binarul dhs. Setează DHS_BIN sau construiește-l: go build -o dhs ./cmd/dhs",
    "Choose a package folder first": "Alege întâi dosarul pachetului",
    "Prototype — drives the dhs CLI over JSON": "Prototip — comandă CLI-ul dhs prin JSON",
}


def _lang() -> str:
    v = os.environ.get("DHS_LANG", "").lower()
    if v in ("ro", "en"):
        return v
    for var in ("LC_ALL", "LC_MESSAGES", "LANG"):
        val = os.environ.get(var, "")
        if val:
            return "ro" if val.lower().startswith("ro") else "en"
    return "en"


LANG = _lang()


def T(s: str) -> str:
    return RO.get(s, s) if LANG == "ro" else s


# ─────────────────────────── helpers ───────────────────────────

def find_dhs() -> str | None:
    env = os.environ.get("DHS_BIN")
    if env and Path(env).is_file():
        return env
    here = Path(__file__).resolve().parent.parent.parent / "dhs"
    if here.is_file():
        return str(here)
    return shutil.which("dhs")


def human(n: int) -> str:
    units = ["B", "KiB", "MiB", "GiB", "TiB"]
    v = float(n)
    i = 0
    while v >= 1024 and i < len(units) - 1:
        v /= 1024
        i += 1
    if i == 0:
        return f"{int(v)} B"
    return f"{v:,.1f} {units[i]}".replace(",", " ")


def count(n: int) -> str:
    return f"{n:,}".replace(",", " ")


class DhsError(Exception):
    pass


def run_dhs(args: list[str], passphrase: str | None = None) -> dict:
    """Runs the CLI with --json and returns the parsed output. Blocking; call from a thread."""
    binary = find_dhs()
    if not binary:
        raise DhsError(T("dhs binary not found. Set DHS_BIN or build it: go build -o dhs ./cmd/dhs"))
    tmp = None
    try:
        if passphrase is not None:
            # The passphrase goes through a private temp file, never through argv.
            fd, tmp = tempfile.mkstemp(prefix="dhs-", dir=os.environ.get("XDG_RUNTIME_DIR") or None)
            with os.fdopen(fd, "w") as f:
                f.write(passphrase + "\n")
            os.chmod(tmp, 0o600)
            args = [*args, "--passphrase-file", tmp]
        env = dict(os.environ, NO_COLOR="1")
        proc = subprocess.run([binary, *args, "--json"], capture_output=True, text=True, env=env)
        if proc.returncode != 0 and not proc.stdout.strip():
            raise DhsError(proc.stderr.strip() or f"dhs exited with {proc.returncode}")
        try:
            data = json.loads(proc.stdout)
        except json.JSONDecodeError as e:
            raise DhsError(f"{proc.stderr.strip()}\n{e}") from e
        if proc.returncode != 0:
            data["_error"] = proc.stderr.strip()
        return data
    finally:
        if tmp:
            try:
                os.remove(tmp)
            except OSError:
                pass


def in_thread(fn, on_done, on_error):
    def work():
        try:
            result = fn()
        except Exception as e:  # noqa: BLE001 — surfaced in the UI
            GLib.idle_add(on_error, str(e))
            return
        GLib.idle_add(on_done, result)
    threading.Thread(target=work, daemon=True).start()


def alert(parent, heading: str, body: str):
    d = Adw.AlertDialog(heading=heading, body=body)
    d.add_response("ok", T("OK"))
    d.present(parent)


def row(title: str, subtitle: str = "") -> Adw.ActionRow:
    r = Adw.ActionRow(title=title, subtitle=subtitle)
    r.set_subtitle_selectable(True)
    return r


def pick_folder(parent, callback):
    dlg = Gtk.FileDialog()

    def done(d, res):
        try:
            f = d.select_folder_finish(res)
        except GLib.Error:
            return
        if f:
            callback(f.get_path())

    dlg.select_folder(parent, None, done)


def clear(group: Adw.PreferencesGroup):
    child = group.get_first_child()
    # PreferencesGroup wraps rows in an internal list box; remove rows we added.
    for r in list(getattr(group, "_rows", [])):
        group.remove(r)
    group._rows = []


def add(group: Adw.PreferencesGroup, widget):
    group.add(widget)
    group._rows = getattr(group, "_rows", []) + [widget]


# ─────────────────────────── pages ───────────────────────────

class ScanPage(Adw.PreferencesPage):
    def __init__(self, win):
        super().__init__()
        self.win = win
        self.roots: list[str] = []
        self.dest: str | None = None

        src = Adw.PreferencesGroup(title=T("Source"))
        self.roots_row = row(T("Folders to include"), T("Default: Documents, Pictures, Videos, Music, Downloads, Desktop, config"))
        b = Gtk.Button(label=T("Choose…"), valign=Gtk.Align.CENTER)
        b.connect("clicked", lambda *_: pick_folder(win, self._add_root))
        self.roots_row.add_suffix(b)
        src.add(self.roots_row)

        self.dest_row = row(T("Destination drive"), T("Not chosen"))
        b = Gtk.Button(label=T("Choose…"), valign=Gtk.Align.CENTER)
        b.connect("clicked", lambda *_: pick_folder(win, self._set_dest))
        self.dest_row.add_suffix(b)
        src.add(self.dest_row)

        self.level = Adw.ComboRow(title=T("Compression"))
        self.level.set_model(Gtk.StringList.new([T("1 · Compatible"), T("2 · Balanced"), T("3 · Maximum")]))
        self.level.set_selected(1)
        src.add(self.level)
        self.secrets = Adw.SwitchRow(title=T("Include secrets"), subtitle=T("SSH/GPG keys, browser passwords, .env files"))
        src.add(self.secrets)
        self.everything = Adw.SwitchRow(title=T("Include everything"), subtitle=T("No exclusions: caches, games, virtual machines"))
        src.add(self.everything)

        self.run = Gtk.Button(label=T("Run scan"), halign=Gtk.Align.END)
        self.run.add_css_class("suggested-action")
        self.run.connect("clicked", self._scan)
        self.spinner = Gtk.Spinner()
        box = Gtk.Box(spacing=12, halign=Gtk.Align.END)
        box.append(self.spinner)
        box.append(self.run)
        src.set_header_suffix(box)
        self.add(src)

        self.results = Adw.PreferencesGroup(title=T("Results"))
        self.add(self.results)

    def _add_root(self, path):
        self.roots.append(path)
        self.roots_row.set_subtitle("\n".join(self.roots))

    def _set_dest(self, path):
        self.dest = path
        self.dest_row.set_subtitle(path)

    def args(self) -> list[str]:
        a = ["scan", *self.roots, "--level", str(self.level.get_selected() + 1)]
        if self.dest:
            a += ["--dest", self.dest]
        if self.secrets.get_active():
            a.append("--secrets")
        if self.everything.get_active():
            a.append("--all")
        return a

    def _scan(self, *_):
        self.run.set_sensitive(False)
        self.spinner.start()
        in_thread(lambda: run_dhs(self.args()), self._show, self._fail)

    def _fail(self, msg):
        self.spinner.stop()
        self.run.set_sensitive(True)
        alert(self.win, T("Error"), msg)

    def _show(self, d):
        self.spinner.stop()
        self.run.set_sensitive(True)
        clear(self.results)
        inv, est, sysinfo = d["inventory"], d["estimate"], d["system"]
        add(self.results, row(T("System"), f'{sysinfo["name"]} {sysinfo.get("version", "")} ({sysinfo["arch"]})'))
        add(self.results, row(T("To include"), f'{human(inv["bytes"])} · {count(inv["files"])} {T("files")}'))
        skipped = sum(s["bytes"] for s in inv.get("skipped") or [])
        if skipped:
            top = ", ".join(f'{s["dir"]} {human(s["bytes"])}' for s in (inv.get("skipped") or [])[:4])
            add(self.results, row(T("Excluded"), f"{human(skipped)} — {top}"))
        sec = inv.get("secrets") or {}
        if sec.get("files"):
            add(self.results, row(T("Secrets"), f'{human(sec["bytes"])} · {count(sec["files"])} {T("files")}'))
        comp = inv.get("by_class") or {}
        names = {0: "unknown", 1: "incompressible", 2: "binary", 3: "text"}
        parts = []
        for k, v in sorted(comp.items(), key=lambda kv: -kv[1]["bytes"]):
            name = names.get(int(k), str(k))
            pct = (v["bytes"] / inv["bytes"] * 100) if inv["bytes"] else 0
            parts.append(f'{T(name)} {human(v["bytes"])} ({pct:.0f}%)')
        add(self.results, row(T("Composition"), " · ".join(parts)))
        add(self.results, row(T("Estimate"), f'{human(est["min"])} – {human(est["max"])} · {est["duration_ns"] / 1e9:.0f} s'))
        add(self.results, row(T("Volumes"), f'{est["volumes"]} × 3.5 GiB'))
        if d.get("destination"):
            dest = d["destination"]
            free = dest["free"]
            fits = d.get("fits")
            txt = T("Fits, with {} to spare").format(human(free - est["max"])) if fits else T("Does not fit: {} short").format(human(est["max"] - free))
            r = row(T("Destination"), f'{dest["path"]} · {human(free)} free · {dest["fs"]}\n{txt}')
            r.add_css_class("success" if fits else "error")
            add(self.results, r)


class BackupPage(Adw.PreferencesPage):
    def __init__(self, win, scan: ScanPage):
        super().__init__()
        self.win = win
        self.scan = scan

        g = Adw.PreferencesGroup(title=T("Backup"), description=T("If you lose the passphrase, the package is lost for good. There is no recovery."))
        self.name = Adw.EntryRow(title=T("Package name"))
        g.add(self.name)
        self.encrypt = Adw.SwitchRow(title=T("Encrypt with a passphrase"))
        self.encrypt.set_active(True)
        g.add(self.encrypt)
        self.p1 = Adw.PasswordEntryRow(title=T("Passphrase"))
        self.p2 = Adw.PasswordEntryRow(title=T("Repeat passphrase"))
        g.add(self.p1)
        g.add(self.p2)
        self.encrypt.connect("notify::active", lambda *_: [w.set_sensitive(self.encrypt.get_active()) for w in (self.p1, self.p2)])
        self.verify = Adw.SwitchRow(title=T("Verify after writing"), subtitle=T("Decrypts and checks every block"))
        g.add(self.verify)

        self.run = Gtk.Button(label=T("Create package"), halign=Gtk.Align.END)
        self.run.add_css_class("suggested-action")
        self.run.connect("clicked", self._go)
        self.spinner = Gtk.Spinner()
        box = Gtk.Box(spacing=12, halign=Gtk.Align.END)
        box.append(self.spinner)
        box.append(self.run)
        g.set_header_suffix(box)
        self.add(g)

        self.results = Adw.PreferencesGroup(title=T("Results"))
        self.add(self.results)

    def _go(self, *_):
        if not self.scan.dest:
            alert(self.win, T("Error"), T("Choose a destination first"))
            return
        pw = None
        if self.encrypt.get_active():
            a, b = self.p1.get_text(), self.p2.get_text()
            if a != b:
                alert(self.win, T("Error"), T("Passphrases do not match"))
                return
            if len(a) < 8:
                alert(self.win, T("Error"), T("Passphrase must have at least 8 characters"))
                return
            pw = a
        args = self.scan.args()
        args[0] = "backup"
        args += ["--dest", self.scan.dest, "--yes"] if "--dest" not in args else ["--yes"]
        if self.name.get_text().strip():
            args += ["--name", self.name.get_text().strip()]
        if pw is None:
            args.append("--no-encrypt")
        if self.verify.get_active():
            args.append("--verify")
        self.run.set_sensitive(False)
        self.spinner.start()
        in_thread(lambda: run_dhs(args, pw), self._show, self._fail)

    def _fail(self, msg):
        self.spinner.stop()
        self.run.set_sensitive(True)
        alert(self.win, T("Error"), msg)

    def _show(self, d):
        self.spinner.stop()
        self.run.set_sensitive(True)
        clear(self.results)
        m = d.get("manifest", {})
        add(self.results, row(T("Package"), d.get("package", "")))
        add(self.results, row(T("Written"), f'{count(d.get("files", 0))} {T("files")} · {human(d.get("raw_bytes", 0))} → {human(d.get("stored_bytes", 0))} · {m.get("volumes", "?")} {T("Volumes").lower()} · {d.get("duration_ns", 0) / 1e9:.0f} s'))
        v = d.get("verification")
        if v is not None:
            ok = not v.get("Problems")
            r = row(T("Verified"), T("all blocks and sums OK") if ok else "\n".join(v.get("Problems") or []))
            r.add_css_class("success" if ok else "error")
            add(self.results, r)
        if d.get("_error"):
            add(self.results, row(T("Error"), d["_error"]))


class RestorePage(Adw.PreferencesPage):
    def __init__(self, win):
        super().__init__()
        self.win = win
        self.pkg: str | None = None

        g = Adw.PreferencesGroup(title=T("Restore"))
        self.pkg_row = row(T("Package folder"), T("Not chosen"))
        b = Gtk.Button(label=T("Choose…"), valign=Gtk.Align.CENTER)
        b.connect("clicked", lambda *_: pick_folder(win, self._set_pkg))
        self.pkg_row.add_suffix(b)
        g.add(self.pkg_row)
        self.pw = Adw.PasswordEntryRow(title=T("Passphrase"))
        g.add(self.pw)
        self.conflicts = Adw.ComboRow(title=T("When a file already exists"), subtitle=T("Keep existing, write restored beside it with “ (DHS)”"))
        self.conflicts.set_model(Gtk.StringList.new([T("keep-both"), T("skip"), T("overwrite")]))
        g.add(self.conflicts)

        self.plan_btn = Gtk.Button(label=T("Show plan"))
        self.plan_btn.connect("clicked", lambda *_: self._run(dry=True))
        self.go_btn = Gtk.Button(label=T("Restore now"))
        self.go_btn.add_css_class("destructive-action")
        self.go_btn.set_sensitive(False)
        self.go_btn.connect("clicked", lambda *_: self._run(dry=False))
        self.spinner = Gtk.Spinner()
        box = Gtk.Box(spacing=12, halign=Gtk.Align.END)
        for w in (self.spinner, self.plan_btn, self.go_btn):
            box.append(w)
        g.set_header_suffix(box)
        self.add(g)

        self.results = Adw.PreferencesGroup(title=T("Plan"))
        self.add(self.results)

    def _set_pkg(self, path):
        self.pkg = path
        self.pkg_row.set_subtitle(path)
        self.go_btn.set_sensitive(False)

    def _run(self, dry: bool):
        if not self.pkg:
            alert(self.win, T("Error"), T("Choose a package folder first"))
            return
        policy = ["keep-both", "skip", "overwrite"][self.conflicts.get_selected()]
        args = ["restore", self.pkg, "--conflicts", policy, "--dry-run" if dry else "--yes"]
        pw = self.pw.get_text() or None
        for w in (self.plan_btn, self.go_btn):
            w.set_sensitive(False)
        self.spinner.start()
        in_thread(lambda: run_dhs(args, pw), lambda d: self._show(d, dry), self._fail)

    def _fail(self, msg):
        self.spinner.stop()
        self.plan_btn.set_sensitive(True)
        alert(self.win, T("Error"), msg)

    def _show(self, d, dry: bool):
        self.spinner.stop()
        self.plan_btn.set_sensitive(True)
        clear(self.results)
        p = d.get("plan", {})
        tgt = d.get("target", {})
        add(self.results, row(T("Migration"), f'→ {tgt.get("name", "")} {tgt.get("version", "")} ({tgt.get("arch", "")})'))
        add(self.results, row(T("To write"), f'{count(p.get("files", 0))} {T("files")} · {human(p.get("bytes", 0))}'))
        for rs in p.get("roots") or []:
            add(self.results, row(rs.get("Root", ""), f'{count(rs.get("Files", 0))} {T("files")}, {human(rs.get("Bytes", 0))} → {rs.get("Dest", "")}'))
        if p.get("conflicts"):
            add(self.results, row(T("Conflicts"), f'{p["conflicts"]} · {T(p.get("policy", ""))}'))
        if p.get("renamed"):
            add(self.results, row(T("Renamed"), str(p["renamed"])))
        if dry:
            self.go_btn.set_sensitive(p.get("files", 0) > 0)
            return
        rep = d.get("result") or {}
        failed = d.get("failed") or []
        r = row(T("Restored"), f'{count(rep.get("Files", 0))} {T("files")} · {human(rep.get("Bytes", 0))}\n' + (T("All files verified and put in place") if not failed else T("{} files could not be restored").format(len(failed))))
        r.add_css_class("success" if not failed else "error")
        add(self.results, r)
        for f in failed[:10]:
            add(self.results, row(f.get("path", ""), f.get("error", "")))


class AppsPage(Adw.PreferencesPage):
    """dhs plan: which applications a package would install here, and where their configuration
    goes. Read-only by design — installing needs root, so the page hands the exact commands to a
    terminal instead of running them itself (D5: the user sees every command before it runs)."""

    def __init__(self, win):
        super().__init__()
        self.win = win
        self.pkg: str | None = None
        self.commands: list[str] = []

        g = Adw.PreferencesGroup(title=T("Applications"), description=T("What dhs install would do on this system. Nothing runs from here."))
        self.pkg_row = row(T("Package folder"), T("Not chosen"))
        b = Gtk.Button(label=T("Choose…"), valign=Gtk.Align.CENTER)
        b.connect("clicked", lambda *_: pick_folder(win, self._set_pkg))
        self.pkg_row.add_suffix(b)
        g.add(self.pkg_row)
        self.pw = Adw.PasswordEntryRow(title=T("Passphrase"))
        g.add(self.pw)

        self.plan_btn = Gtk.Button(label=T("Show plan"))
        self.plan_btn.connect("clicked", lambda *_: self._run())
        self.copy_btn = Gtk.Button(label=T("Copy commands"))
        self.copy_btn.set_sensitive(False)
        self.copy_btn.connect("clicked", lambda *_: self._copy())
        self.spinner = Gtk.Spinner()
        box = Gtk.Box(spacing=12, halign=Gtk.Align.END)
        for w in (self.spinner, self.plan_btn, self.copy_btn):
            box.append(w)
        g.set_header_suffix(box)
        self.add(g)

        self.install = Adw.PreferencesGroup(title=T("Install"))
        self.present = Adw.PreferencesGroup(title=T("Already here"))
        self.missing = Adw.PreferencesGroup(title=T("Not available here"))
        self.unknown = Adw.PreferencesGroup(title=T("Unknown"), description=T("Not in the database; nothing is installed for them."))
        self.configs = Adw.PreferencesGroup(title=T("Configuration"))
        self.cmds = Adw.PreferencesGroup(title=T("Commands"), description=T("Run dhs install <package> in a terminal, or paste these."))
        for grp in (self.install, self.present, self.missing, self.unknown, self.configs, self.cmds):
            self.add(grp)

    def _set_pkg(self, path):
        self.pkg = path
        self.pkg_row.set_subtitle(path)

    def _run(self):
        if not self.pkg:
            alert(self.win, T("Error"), T("Choose a package folder first"))
            return
        self.plan_btn.set_sensitive(False)
        self.spinner.start()
        in_thread(lambda: run_dhs(["plan", self.pkg], self.pw.get_text() or None), self._show, self._fail)

    def _fail(self, msg):
        self.spinner.stop()
        self.plan_btn.set_sensitive(True)
        alert(self.win, T("Error"), msg)

    def _copy(self):
        self.get_clipboard().set("\n".join(self.commands) + "\n")

    def _show(self, d):
        self.spinner.stop()
        self.plan_btn.set_sensitive(True)
        p = d.get("plan") or {}
        m = d.get("manifest") or {}
        names = {a["id"]: a.get("name", a["id"]) for a in m.get("apps") or []}
        for grp in (self.install, self.present, self.missing, self.unknown, self.configs, self.cmds):
            clear(grp)
        for it in p.get("install") or []:
            sub = f'{it.get("manager", "")}  {it.get("package", "")}'
            if it.get("for"):
                sub += "  ·  " + T("stands in for ") + ", ".join(names.get(x, x) for x in it["for"])
            add(self.install, row(it.get("name", it["id"]), sub))
        if not p.get("install"):
            add(self.install, row(T("Nothing to install"), ""))
        for it in p.get("present") or []:
            sub = it.get("via", "")
            if it.get("for"):
                sub += "  ·  " + T("stands in for ") + ", ".join(names.get(x, x) for x in it["for"])
            add(self.present, row(it.get("name", it["id"]), sub))
        for it in p.get("no_source") or []:
            needs = ", ".join(it.get("needs") or [])
            add(self.missing, row(it.get("name", it["id"]), T("needs one of: ") + needs if needs else T("no known way to install it on this platform")))
        for it in p.get("missing") or []:
            add(self.missing, row(it.get("name", it["id"]), T(it.get("reason", ""))))
        self.missing.set_visible(bool(p.get("no_source") or p.get("missing")))
        unknown = p.get("unknown") or []
        for u in unknown[:20]:
            add(self.unknown, row(u.get("name") or u.get("package", ""), f'{u.get("via", "")} {u.get("version", "")}'.strip()))
        if len(unknown) > 20:
            add(self.unknown, row(T("… and {} more").format(len(unknown) - 20), ""))
        self.unknown.set_visible(bool(unknown))
        for c in p.get("configs") or []:
            title = f'{c["id"]}/{c["key"]}' + (f'@{c["variant"]}' if c.get("variant") else "")
            if c.get("action") == "place":
                add(self.configs, row(title, "→ " + c.get("destination", "")))
            else:
                add(self.configs, row(title, T("kept aside: ") + T(c.get("reason", ""))))
        self.configs.set_visible(bool(p.get("configs")))
        self.commands = [" ".join(c.get("argv") or []) for c in p.get("commands") or []]
        for c in self.commands:
            r = Adw.ActionRow(title=GLib.markup_escape_text(c))
            r.add_css_class("monospace")
            add(self.cmds, r)
        self.cmds.set_visible(bool(self.commands))
        self.copy_btn.set_sensitive(bool(self.commands))


# ─────────────────────────── app ───────────────────────────

class Window(Adw.ApplicationWindow):
    def __init__(self, app):
        super().__init__(application=app, title=T("DHS — Direct Handoff Suite"), default_width=820, default_height=720)
        view = Adw.ToolbarView()
        header = Adw.HeaderBar()
        stack = Adw.ViewStack()
        scan = ScanPage(self)
        stack.add_titled_with_icon(scan, "scan", T("Scan"), "system-search-symbolic")
        stack.add_titled_with_icon(BackupPage(self, scan), "backup", T("Backup"), "document-save-symbolic")
        stack.add_titled_with_icon(RestorePage(self), "restore", T("Restore"), "document-revert-symbolic")
        stack.add_titled_with_icon(AppsPage(self), "apps", T("Applications"), "view-grid-symbolic")
        switcher = Adw.ViewSwitcher(stack=stack, policy=Adw.ViewSwitcherPolicy.WIDE)
        header.set_title_widget(switcher)
        view.add_top_bar(header)
        view.set_content(stack)
        self.set_content(view)
        binary = find_dhs()
        if not binary:
            GLib.idle_add(alert, self, T("Error"), T("dhs binary not found. Set DHS_BIN or build it: go build -o dhs ./cmd/dhs"))


class App(Adw.Application):
    def __init__(self):
        super().__init__(application_id="io.github.necta14.dhs", flags=Gio.ApplicationFlags.DEFAULT_FLAGS)

    def do_activate(self):
        win = self.props.active_window or Window(self)
        win.present()


if __name__ == "__main__":
    try:
        locale.setlocale(locale.LC_ALL, "")
    except locale.Error:
        pass
    sys.exit(App().run(sys.argv))
