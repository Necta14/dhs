#!/usr/bin/env python3
"""Rewrites the direct-download block of the website from a published GitHub release.

    python3 packaging/site-downloads.py 0.2.0

Asks the GitHub API (through `gh`, so the maintainer's token is used) for the assets of v<version>
and replaces everything between the markers <!-- downloads:start --> and <!-- downloads:end --> in
site/index.html and site/ro/index.html. Nothing else on the pages is touched; the pages stay
static HTML with no script.
"""
import json
import os
import re
import subprocess
import sys

REPO = "Necta14/dhs"
HERE = os.path.dirname(os.path.abspath(__file__))
SITE = os.path.join(HERE, "..", "site")

# One row per artifact, in display order: (asset name pattern, English label, Romanian label).
ROWS = [
    ("dhs_{v}_windows_amd64.zip", "Windows, 64-bit — <code>dhs.exe</code> and <code>dhs-gui.exe</code>", "Windows, 64 de biți — <code>dhs.exe</code> și <code>dhs-gui.exe</code>"),
    ("dhs_{v}_windows_arm64.zip", "Windows on ARM", "Windows pe ARM"),
    ("dhs_{v}_amd64.deb", "Debian, Ubuntu, Mint — <code>.deb</code>", "Debian, Ubuntu, Mint — <code>.deb</code>"),
    ("dhs_{v}_arm64.deb", "Debian, Ubuntu on ARM — <code>.deb</code>", "Debian, Ubuntu pe ARM — <code>.deb</code>"),
    ("dhs-{v}-1.x86_64.rpm", "Fedora, RHEL, openSUSE — <code>.rpm</code>", "Fedora, RHEL, openSUSE — <code>.rpm</code>"),
    ("dhs-{v}-1.aarch64.rpm", "Fedora, openSUSE on ARM — <code>.rpm</code>", "Fedora, openSUSE pe ARM — <code>.rpm</code>"),
    ("DHS-{v}-x86_64.AppImage", "Any Linux, installing nothing — AppImage", "Orice Linux, fără instalare — AppImage"),
    ("dhs_{v}_linux_amd64.tar.gz", "Linux, plain archive", "Linux, arhivă simplă"),
    ("dhs_{v}_linux_arm64.tar.gz", "Linux on ARM, plain archive", "Linux pe ARM, arhivă simplă"),
    ("dhs-{v}-source.tar.gz", "Source", "Sursa"),
    ("SHA256SUMS", "Checksums for everything above", "Sumele de control pentru tot ce e mai sus"),
]


def human(n: int) -> str:
    if n >= 1 << 20:
        return f"{n / (1 << 20):.1f} MiB"
    if n >= 1 << 10:
        return f"{n / (1 << 10):.0f} KiB"
    return f"{n} B"


def assets(version: str) -> dict:
    out = subprocess.run(["gh", "api", f"repos/{REPO}/releases/tags/v{version}"], capture_output=True, text=True, check=True).stdout
    rel = json.loads(out)
    if rel.get("draft"):
        sys.exit("the release is still a draft")
    return {a["name"]: a for a in rel["assets"]}


def block(version: str, found: dict, lang: str) -> str:
    li = []
    for pattern, en, ro in ROWS:
        name = pattern.format(v=version)
        a = found.get(name)
        if not a:
            continue
        label = en if lang == "en" else ro
        li.append(
            f'        <li><a href="{a["browser_download_url"]}"><strong>{label}</strong>'
            f'<span class="file">{name}</span><span class="size">{human(a["size"])}</span></a></li>'
        )
    if not li:
        sys.exit("no known asset found in the release")
    head = "Direct downloads" if lang == "en" else "Descărcări directe"
    note = (
        f'Version {version}, straight from the GitHub release. Arch users: <code>dhs-cli</code> and <code>dhs-gui</code> from the AUR recipe in <code>packaging/aur/</code>.'
        if lang == "en"
        else f'Versiunea {version}, direct din release-ul de pe GitHub. Pe Arch: <code>dhs-cli</code> și <code>dhs-gui</code> din rețeta AUR din <code>packaging/aur/</code>.'
    )
    all_ = "All releases on GitHub" if lang == "en" else "Toate versiunile pe GitHub"
    return (
        "<!-- downloads:start -->\n"
        f'      <h3 style="margin:0 0 12px;font-size:19px">{head}</h3>\n'
        f'      <p class="sub" style="margin-bottom:14px">{note}</p>\n'
        '      <ul class="downloads">\n' + "\n".join(li) + "\n      </ul>\n"
        '      <div class="ctas" style="margin-top:18px">\n'
        f'        <a class="btn" href="https://github.com/{REPO}/releases">{all_}</a>\n'
        "      </div>\n"
        "      <!-- downloads:end -->"
    )


def rewrite(path: str, new: str) -> None:
    with open(path, encoding="utf-8") as f:
        s = f.read()
    pat = re.compile(r"<!-- downloads:start -->.*?<!-- downloads:end -->", re.S)
    if not pat.search(s):
        sys.exit(f"{path}: markers not found")
    s = pat.sub(lambda _m: new, s, count=1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(s)


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    version = sys.argv[1].lstrip("v")
    found = assets(version)
    rewrite(os.path.join(SITE, "index.html"), block(version, found, "en"))
    rewrite(os.path.join(SITE, "ro", "index.html"), block(version, found, "ro"))
    print(f"downloads block rewritten for {version}: {sum(1 for p, _, _ in ROWS if p.format(v=version) in found)} files")
    return 0


if __name__ == "__main__":
    sys.exit(main())
