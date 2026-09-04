# The application database

One JSON file per application, in [`apps/`](apps/). DHS embeds them all in the binary, so it
recognises applications, installs their counterparts and places their configuration **offline**,
with nothing guessed: an application is known only if a file here names it (Rule #2 in
[`CLAUDE.md`](../CLAUDE.md)).

The data is [CC BY 4.0](LICENSE). Anyone may reuse it; contributions arrive as pull requests.

## Adding an application

1. Copy an existing file — [`apps/firefox.json`](apps/firefox.json) shows everything — and name it
   `<id>.json`.
2. Fill in what you can **verify**: a package name you have installed, a path you have looked at.
   Leave out what you have not checked. An absent field is honest; a wrong one sends someone's
   configuration to the wrong place.
3. Run `python3 appdb/tools/validate.py`. It applies the same rules as the Go tests.
4. Open a pull request. Sign off your commit (`git commit -s`, the DCO).

Where to check package names: [Repology](https://repology.org/) lists one project across every
distribution; [winget.run](https://winget.run/) and the
[winget-pkgs](https://github.com/microsoft/winget-pkgs/tree/master/manifests) repository hold the
winget identifiers; [community.chocolatey.org](https://community.chocolatey.org/packages) and
[scoop.sh](https://scoop.sh/) the other two; [flathub.org](https://flathub.org/) and
[snapcraft.io](https://snapcraft.io/) the sandboxed ones.

## The format

```json
{
  "id": "firefox",
  "name": "Mozilla Firefox",
  "summary": "Web browser",
  "category": "browser",
  "homepage": "https://www.mozilla.org/firefox/",
  "license": "MPL-2.0",
  "windows": {
    "winget":   ["Mozilla.Firefox"],
    "choco":    ["firefox"],
    "scoop":    ["extras/firefox"],
    "registry": ["Mozilla Firefox"],
    "paths":    { "profiles": "%APPDATA%/Mozilla/Firefox" }
  },
  "linux": {
    "pacman":  ["firefox"],
    "aur":     [],
    "apt":     ["firefox", "firefox-esr"],
    "dnf":     ["firefox"],
    "zypper":  ["MozillaFirefox"],
    "apk":     ["firefox"],
    "flatpak": ["org.mozilla.firefox"],
    "snap":    ["firefox"],
    "paths":   { "profiles": "~/.mozilla/firefox" }
  },
  "config": {
    "portability": "identical",
    "exclude": ["cache2", "startupCache", "*.lock"],
    "notes": "The profile directory has the same layout on every platform."
  },
  "equivalents": []
}
```

### Identity

| Field | Rule |
|---|---|
| `id` | Lowercase letters, digits, hyphens; **equals the file name**. Short and recognisable: `firefox`, `vscode`, `notepad-plus-plus`, `obs-studio`. Add a vendor only to disambiguate (`gnome-calculator`, `kde-connect`). |
| `name` | The name the application shows about itself. |
| `summary` | One line, what it is, no marketing. |
| `category` | One of the closed list below. |
| `homepage` | The official site, `https://`. |
| `license` | An SPDX identifier (`GPL-3.0`, `MIT`, `MPL-2.0`) or `proprietary`. Optional. |

Categories: `browser` `email` `chat` `video-call` `feed-reader` · `office` `pdf` `notes` `ebook`
`education` `science` `finance` `productivity` · `editor` `ide` `terminal` `shell` `devtool` `vcs`
`database` `virtualization` `container` · `media-player` `music` `video-editor` `audio-editor`
`graphics` `photo` `3d` `cad` `screenshot` `screen-recorder` `streaming` · `gaming` `launcher`
`emulator` · `password-manager` `security` `vpn` `network` `remote-desktop` · `file-manager`
`file-sync` `cloud-storage` `backup` `archive` `download` `torrent` · `system` `utility` `fonts`
`accessibility` `other`.

### Platform sections

`windows` and `linux` each list the identifiers under which the platform's managers know the
application. Every field is a list, because a package may go by two names (`firefox` and
`firefox-esr` on Debian); **the first one is what DHS installs**, the others are recognised when
found. An application that does not exist on a platform simply has no section for it.

| Manager | Where the identifier comes from | Example |
|---|---|---|
| `winget` | `PackageIdentifier` in winget-pkgs, `Publisher.Name` | `Mozilla.Firefox` |
| `choco` | package id on community.chocolatey.org | `firefox` |
| `scoop` | `bucket/name`; the `main` bucket may be written without the bucket | `extras/firefox`, `git` |
| `registry` | `DisplayName` in *Apps & features*, **without** version or architecture suffixes. Detection only. Matched whole or as a prefix followed by a space or `(`, so `Git` does not claim `GitHub Desktop`. | `Mozilla Firefox` |
| `pacman` | the Arch package name (official repositories only) | `firefox` |
| `aur` | the AUR package name; installed through `paru` or `yay`, never through `makepkg` by hand | `visual-studio-code-bin` |
| `apt` | Debian/Ubuntu package name | `firefox-esr` |
| `dnf` | Fedora package name | `firefox` |
| `zypper` | openSUSE package name | `MozillaFirefox` |
| `apk` | Alpine package name | `firefox` |
| `flatpak` | the application id on Flathub | `org.mozilla.firefox` |
| `snap` | the snap name, with `--classic` after a space when the snap requires it | `code --classic` |

Name only what is really in the official repositories of that distribution today. A package that
needs a third-party repository (RPM Fusion, a PPA, Microsoft's own repository) does **not** go
under `dnf` or `apt`; use `flatpak` or `snap` for those, or `aur` on Arch.

### Paths — where the configuration lives

`paths` maps a **key** to a location. The key is what ties the platforms together: `"profiles"` on
Windows and `"profiles"` on Linux are the same thing, and that is how DHS knows where a directory
from one system goes on the other. Keys are lowercase, short, and identical across the platform
sections. A location may be a directory or a single file.

Allowed starts, and nothing else:

| Linux | Windows |
|---|---|
| `~/` — the home directory | `%USERPROFILE%/` — the home directory |
| `$XDG_CONFIG_HOME/` — `~/.config` unless the user moved it | `%APPDATA%/` — `AppData/Roaming` |
| `$XDG_DATA_HOME/` — `~/.local/share` | `%LOCALAPPDATA%/` — `AppData/Local` |
| `$XDG_STATE_HOME/` — `~/.local/state` | |

Forward slashes on both platforms. Write `$XDG_CONFIG_HOME/foo`, not `~/.config/foo`: the first
also finds the configuration of a Flatpak or Snap build, which lives in the sandbox
(`~/.var/app/<id>/config/foo`, `~/snap/<name>/current/.config/foo`). DHS derives those on its own.

Only **user** configuration. Never a cache, a log directory, a game library, a mail store measured
in gigabytes, or anything under `/etc` or `Program Files`. Inside the locations you do name, list
what should stay behind under `config.exclude`: a pattern without `/` matches any path segment
(`Cache`, `*.log`), one with `/` matches the whole relative path (`storage/default/*/cache`).

Secrets inside a configuration directory — SSH keys, `logins.json`, `key4.db`, `Login Data`,
`.kdbx` files — need no marking here. DHS recognises them by name and leaves them out unless the
user asks for them with `--secrets` (D4).

### Portability

`config.portability` says how far the configuration travels **between operating systems**. On the
same operating system (Arch → Fedora) everything is restored in place regardless.

| Value | Meaning | What DHS does on the other OS |
|---|---|---|
| `identical` | The same files, in the same layout. Text formats with no absolute paths: Firefox profiles, VS Code settings, `.gitconfig`, most Electron applications. | Restores in place, into the location the target platform's `paths` gives for the same key. |
| `translatable` | The same settings in a different form or with platform paths inside: `vlcrc`, most Qt `.conf` files. | Keeps aside under `~/DHS-restored/apps/<id>/` and says so. v1 translates nothing. |
| `untranslatable` | Bound to the platform it runs on: registry-backed Windows applications, desktop-environment-specific Linux ones (`dolphinrc`, GNOME dconf-backed settings). | Keeps aside and says so. |
| `none` | Nothing worth carrying. No `paths` on any platform. | — |

Be strict with `identical`. If an application writes absolute paths or platform module names into
its files, it is `translatable`, however tempting. The safety of a migration comes from DHS not
pretending.

### Equivalents

`equivalents` lists the ids of other entries that do the same job, in order of preference. It is
what DHS proposes when the application has no section for the target platform: Notepad++ is
Windows-only, so its entry names `kate` and `vscodium`. The list is symmetric in spirit but not by
rule — write what genuinely replaces what. Every id named must exist as a file here.

An application that exists on both platforms needs no equivalents, though it may name a lighter
or freer alternative.

## Validation

`python3 appdb/tools/validate.py` checks every file: JSON, unknown fields, the id against the file
name, categories, path prefixes, portability against paths, duplicate identifiers across files,
dangling equivalents. The Go tests in `appdb_test.go` apply the same rules and run on every
change. A database that does not validate does not build.
