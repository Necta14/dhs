# The Windows interface

`dhs-gui.exe` is a separate process that drives `dhs.exe` through its `--json` output. It holds no
business rule of its own: every number, every warning and every refusal on screen comes from the
core. That is decision D9, and it is why the same interface can exist twice without the logic
existing twice.

```bash
go build -ldflags "-s -w -H windowsgui" -o dhs-gui.exe ./gui/windows
```

About 2.9 MiB, amd64 and arm64, no cgo. Put it next to `dhs.exe` and both fit on a stick; nothing
is installed and nothing is left running.

## Why it is drawn by hand

The requirement was a portable single `.exe` that looks current and matches the GNOME front end.
That ruled out the obvious answers: WinUI needs the Windows App SDK, anything web-based drags in a
runtime, and plain themed Win32 controls look like a 2009 utility however modern the theme is.

So the window is Win32 and the contents are drawn: the header, the sidebar, the cards, the toggle
switches, the buttons. The palette and the metrics come from libadwaita, in `theme.go`, so the two
interfaces read as one program. Real `EDIT` controls are kept for text entry, because nobody should
reimplement selection, IME and clipboard behaviour, but their sunken border is gone and the rounded
frame behind them is drawn like everything else.

Two Windows courtesies matter more than they sound: `DwmSetWindowAttribute` for a dark title bar
and rounded window corners. Without them a custom-drawn window still announces itself as old.

## The one real constraint

GDI has no antialiasing, and GDI+ cannot be called from Go for anything that takes a float: the
Microsoft x64 convention passes floats in XMM registers while Go's `syscall` puts every argument in
an integer register, so `GdipCreateFont` and friends would receive rubbish. Writing an assembly
trampoline for that is not worth a dependency of its own.

Instead, `render.go` draws every shape into a surface twice the size with plain integer GDI and
shrinks it with a halftone stretch, which gives smooth rounded corners; the text is drawn afterwards
at native size so ClearType stays crisp. Hence two passes over the same layout code: shapes first,
text second.

## Files

| | |
|---|---|
| `main.go` | the window, the three pages, the layout and the event wiring |
| `render.go` | the two-pass renderer |
| `theme.go` | the libadwaita palette and the metrics |
| `win32.go` | the syscall bindings, only the ones actually used |
| `cli.go` | running `dhs --json` and turning the result into text |
| `dhs-gui.exe.manifest` | Common Controls v6, per-monitor DPI, long paths, `asInvoker` |
| `rsrc_windows_*.syso` | the manifest and the icon, compiled in |

The `.syso` files are generated and committed, so a plain `go build` needs no extra tool:

```bash
go run github.com/akavel/rsrc@v0.10.2 -manifest gui/windows/dhs-gui.exe.manifest \
  -ico gui/windows/dhs.ico -arch amd64 -o gui/windows/rsrc_windows_amd64.syso
```

`asInvoker` in the manifest is deliberate. DHS reads and writes the user's own profile and must
never ask for administrator rights: a migration tool that demands elevation is one nobody should
trust.
