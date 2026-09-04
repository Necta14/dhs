#!/usr/bin/env python3
"""Validates appdb/apps/*.json with the same rules as appdb_test.go.

Usage: python3 appdb/tools/validate.py [file.json ...]
Without arguments, every file under appdb/apps/ is checked. Exit code 1 on any problem.
"""
import glob
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
APPS = os.path.join(HERE, "..", "apps")

CATEGORIES = {
    "browser", "email", "chat", "video-call", "feed-reader",
    "office", "pdf", "notes", "ebook", "education", "science", "finance", "productivity",
    "editor", "ide", "terminal", "shell", "devtool", "vcs", "database", "virtualization", "container",
    "media-player", "music", "video-editor", "audio-editor", "graphics", "photo", "3d", "cad",
    "screenshot", "screen-recorder", "streaming",
    "gaming", "launcher", "emulator",
    "password-manager", "security", "vpn", "network", "remote-desktop",
    "file-manager", "file-sync", "cloud-storage", "backup", "archive", "download", "torrent",
    "system", "utility", "fonts", "accessibility", "other",
}
WINDOWS_MANAGERS = ["winget", "choco", "scoop", "registry"]
LINUX_MANAGERS = ["pacman", "aur", "apt", "dnf", "zypper", "apk", "flatpak", "snap"]
PREFIXES = {
    "linux": ["~/", "$XDG_CONFIG_HOME/", "$XDG_DATA_HOME/", "$XDG_STATE_HOME/"],
    "windows": ["%APPDATA%/", "%LOCALAPPDATA%/", "%USERPROFILE%/"],
}
TOP_FIELDS = {"id", "name", "summary", "category", "homepage", "license", "windows", "linux", "config", "equivalents"}
CONFIG_FIELDS = {"portability", "exclude", "notes"}
ID_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")


def check(path, app, problems):
    def bad(msg):
        problems.append(f"{os.path.relpath(path)}: {msg}")

    if not isinstance(app, dict):
        bad("not a JSON object")
        return None
    for k in app:
        if k not in TOP_FIELDS:
            bad(f"unknown field {k!r}")
    aid = app.get("id", "")
    if not isinstance(aid, str) or not ID_RE.match(aid):
        bad(f"id {aid!r}: lowercase letters, digits and hyphens only")
    if aid != os.path.basename(path)[:-5]:
        bad(f"id {aid!r} does not match the file name")
    if not str(app.get("name", "")).strip():
        bad("name is missing")
    if app.get("category") not in CATEGORIES:
        bad(f"category {app.get('category')!r} is not in the list")
    hp = app.get("homepage", "")
    if hp and not (hp.startswith("https://") or hp.startswith("http://")):
        bad(f"homepage {hp!r} is not a URL")
    if "windows" not in app and "linux" not in app:
        bad("neither a windows nor a linux section")

    cfg = app.get("config")
    if not isinstance(cfg, dict):
        bad("config section is missing")
        cfg = {}
    for k in cfg:
        if k not in CONFIG_FIELDS:
            bad(f"config: unknown field {k!r}")
    port = cfg.get("portability")
    if port not in ("identical", "translatable", "untranslatable", "none"):
        bad(f"config.portability {port!r}: identical, translatable, untranslatable or none")
    for x in cfg.get("exclude", []) or []:
        if not isinstance(x, str) or not x or x.startswith("/") or x.endswith("/"):
            bad(f"config.exclude: {x!r}: a relative pattern, without leading or trailing slash")

    paths_total = 0
    keys = {}
    for osname, managers in (("windows", WINDOWS_MANAGERS), ("linux", LINUX_MANAGERS)):
        sec = app.get(osname)
        if sec is None:
            continue
        if not isinstance(sec, dict):
            bad(f"{osname}: not an object")
            continue
        for k in sec:
            if k not in managers and k != "paths":
                other = "linux" if osname == "windows" else "windows"
                if k in (LINUX_MANAGERS if osname == "windows" else WINDOWS_MANAGERS):
                    bad(f"{osname}: {k} belongs to the {other} section")
                else:
                    bad(f"{osname}: unknown field {k!r}")
        installable = False
        for m in managers:
            ids = sec.get(m, [])
            if not isinstance(ids, list):
                bad(f"{osname}.{m}: must be a list")
                continue
            for i in ids:
                if not isinstance(i, str) or not i.strip() or i != i.strip():
                    bad(f"{osname}.{m}: empty identifier or stray spaces in {i!r}")
                elif m not in ("snap", "registry") and re.search(r"\s", i):
                    bad(f"{osname}.{m}: {i!r} contains spaces")
            if ids and m != "registry":
                installable = True
        paths = sec.get("paths", {}) or {}
        if not isinstance(paths, dict):
            bad(f"{osname}.paths: must be an object")
            paths = {}
        if not installable and not sec.get("registry") and not paths:
            bad(f"{osname}: the section is empty")
        keys[osname] = set(paths)
        for key, p in paths.items():
            paths_total += 1
            if not ID_RE.match(key):
                bad(f"{osname}.paths: key {key!r}: lowercase letters, digits and hyphens only")
            if not isinstance(p, str) or not any(p.startswith(pre) and len(p) > len(pre) for pre in PREFIXES[osname]):
                bad(f"{osname}.paths.{key}: {p!r} must start with one of {' '.join(PREFIXES[osname])}")
            elif "\\" in p or "//" in p or p.endswith("/") or "/../" in p or "/./" in p:
                bad(f"{osname}.paths.{key}: {p!r}: forward slashes, no trailing slash, no . or ..")
    if port == "none" and paths_total:
        bad("config.portability is none but there are paths")
    if port not in ("none", None) and not paths_total:
        bad(f"config.portability is {port} but there are no paths; use none")
    if port == "identical" and keys.get("windows") and keys.get("linux") and not (keys["windows"] & keys["linux"]):
        bad("config.portability is identical but windows.paths and linux.paths share no key")

    eq = app.get("equivalents", []) or []
    if not isinstance(eq, list):
        bad("equivalents: must be a list")
        eq = []
    seen = set()
    for e in eq:
        if e == aid:
            bad("equivalents: an entry cannot be its own equivalent")
        if e in seen:
            bad(f"equivalents: {e!r} listed twice")
        seen.add(e)
    return app


def main(argv):
    files = argv or sorted(glob.glob(os.path.join(APPS, "*.json")))
    all_files = sorted(glob.glob(os.path.join(APPS, "*.json")))
    problems = []
    apps = {}
    claimed = {}
    # Cross-file checks need every file, even when only some were asked for.
    for path in all_files:
        try:
            with open(path, encoding="utf-8") as f:
                data = json.load(f)
        except Exception as e:  # noqa: BLE001
            problems.append(f"{os.path.relpath(path)}: {e}")
            continue
        app = check(path, data, problems if path in files else [])
        if app is None:
            continue
        aid = app.get("id")
        if aid in apps:
            problems.append(f"{os.path.relpath(path)}: id {aid!r} appears twice")
            continue
        apps[aid] = (path, app)
    for aid, (path, app) in apps.items():
        for e in app.get("equivalents", []) or []:
            if e not in apps:
                problems.append(f"{os.path.relpath(path)}: equivalent {e!r} is not in the database")
        for osname, managers in (("windows", WINDOWS_MANAGERS), ("linux", LINUX_MANAGERS)):
            sec = app.get(osname) or {}
            for m in managers:
                if m == "registry":
                    continue
                for i in sec.get(m, []) or []:
                    if not isinstance(i, str):
                        continue
                    name = i.split(" ")[0] if m == "snap" else i
                    key = (m, name.lower())
                    if key in claimed and claimed[key] != aid:
                        problems.append(f"{os.path.relpath(path)}: {m} {i!r} is already claimed by {claimed[key]}")
                    claimed.setdefault(key, aid)
    problems = sorted(set(problems))
    for p in problems:
        print(p)
    n = len(apps)
    if problems:
        print(f"\n{len(problems)} problems in {n} entries", file=sys.stderr)
        return 1
    print(f"ok: {n} entries")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
