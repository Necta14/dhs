package restore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Necta14/dhs/internal/pack"
	"github.com/Necta14/dhs/internal/system"
)

// Conflict says what we do when the destination already exists.
type Conflict string

const (
	// KeepBoth leaves the existing file untouched and writes the restored one beside it, with a
	// suffix. It is the default: we never lose something that was already on disk.
	KeepBoth Conflict = "keep-both"
	// Skip does not write the restored file if the destination exists.
	Skip Conflict = "skip"
	// Overwrite replaces the existing file. Only on explicit request.
	Overwrite Conflict = "overwrite"
)

// ParseConflict reads the policy from text.
func ParseConflict(s string) (Conflict, error) {
	switch Conflict(s) {
	case KeepBoth, Skip, Overwrite:
		return Conflict(s), nil
	case "":
		return KeepBoth, nil
	}
	return "", fmt.Errorf("unknown conflict policy %q (keep-both | skip | overwrite)", s)
}

// Action is what the plan will do with an entry.
type Action string

const (
	Write     Action = "write"
	WriteNext Action = "write-beside"
	SkipIt    Action = "skip"
	Replace   Action = "overwrite"
)

// Item is a package entry with its destination resolved.
type Item struct {
	Entry   *pack.Entry
	Dest    string // the final absolute path
	Action  Action
	Renamed bool   // the name was adapted to the target system
	Exists  bool   // the destination existed beforehand
	Note    string // why it was renamed or moved
}

// RootSummary is what goes where, in short, for display.
type RootSummary struct {
	Root  pack.Root
	Dest  string
	Files int
	Bytes int64
}

// Plan is everything that is about to happen, computed before any write.
type Plan struct {
	Items     []Item
	Files     int
	Bytes     int64 // what will actually be written (skipped entries excluded)
	Roots     []RootSummary
	Conflicts int
	Renamed   int
	Skipped   int
	Unknown   []pack.Root // roots from the package that do not exist on this system
	Policy    Conflict

	rd *pack.Reader
	os system.OS
}

// Options configures the plan.
type Options struct {
	Reader *pack.Reader
	Roots  *pack.RootMap
	Target system.OS
	// Filter chooses what gets restored; nil = everything.
	Filter   func(*pack.Entry) bool
	Conflict Conflict
	// Note, if set, explains why a root has no place on this system; used for the entries that
	// end up under DHS-restored. Nil gives the generic wording.
	Note func(root pack.Root) string
	// Suffix is what gets added to the name when we keep both. Defaults to " (DHS)".
	Suffix string
}

// Build computes the plan. It touches the disk only to check what already exists.
func Build(o Options) (*Plan, error) {
	if o.Reader == nil || o.Roots == nil {
		return nil, errors.New("restore: reader or root map missing")
	}
	if o.Conflict == "" {
		o.Conflict = KeepBoth
	}
	if o.Suffix == "" {
		o.Suffix = " (DHS)"
	}
	p := &Plan{Policy: o.Conflict, rd: o.Reader, os: o.Target}
	byRoot := map[pack.Root]*RootSummary{}
	unknown := map[pack.Root]bool{}
	// Keys already used in the plan, so two entries never end up in the same file on Windows.
	taken := map[string]bool{}

	entries := make([]*pack.Entry, 0, len(o.Reader.Index.Entries))
	for _, e := range o.Reader.Index.Entries {
		if o.Filter == nil || o.Filter(e) {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Root != entries[j].Root {
			return entries[i].Root < entries[j].Root
		}
		return entries[i].Path < entries[j].Path
	})

	for _, e := range entries {
		item := Item{Entry: e}
		root := e.Root
		rel := e.Path

		// The root does not exist on this system (e.g. "desktop" on a server): it goes under DHS-restored.
		if _, ok := o.Roots.Join(root, "."); !ok && root != pack.RootOther {
			unknown[root] = true
			rel = string(root) + "/" + rel
			item.Note = "standard location does not exist here"
			if o.Note != nil {
				if n := o.Note(root); n != "" {
					item.Note = n
				}
			}
			root = pack.RootOther
		}

		clean, renamed := Sanitize(rel, o.Target)
		if renamed {
			item.Renamed = true
			item.Note = "name adapted to this system"
		}

		// Case collision with another entry in the same plan.
		key := string(root) + "\x00" + FoldKey(clean, o.Target)
		if taken[key] {
			clean = WithSuffix(clean, " (2)")
			key = string(root) + "\x00" + FoldKey(clean, o.Target)
			for n := 3; taken[key]; n++ {
				clean = WithSuffix(clean, fmt.Sprintf(" (%d)", n))
				key = string(root) + "\x00" + FoldKey(clean, o.Target)
			}
			item.Renamed = true
			item.Note = "collided with another file in the package"
		}
		taken[key] = true

		dest, ok := o.Roots.Join(root, clean)
		if !ok {
			return nil, fmt.Errorf("restore: cannot resolve %s/%s", root, clean)
		}
		item.Dest = dest
		item.Action = Write

		if st, err := os.Lstat(dest); err == nil {
			item.Exists = true
			p.Conflicts++
			switch {
			case st.IsDir():
				// A directory with our file's name: we do not fight it.
				item.Action = WriteNext
				item.Dest, _ = o.Roots.Join(root, WithSuffix(clean, o.Suffix))
			case o.Conflict == Skip:
				item.Action = SkipIt
			case o.Conflict == Overwrite:
				item.Action = Replace
			default:
				item.Action = WriteNext
				item.Dest, _ = o.Roots.Join(root, WithSuffix(clean, o.Suffix))
			}
		}

		if item.Renamed {
			p.Renamed++
		}
		if item.Action == SkipIt {
			p.Skipped++
		} else {
			p.Files++
			p.Bytes += e.Size
			s := byRoot[root]
			if s == nil {
				base, _ := o.Roots.Join(root, ".")
				s = &RootSummary{Root: root, Dest: base}
				byRoot[root] = s
			}
			s.Files++
			s.Bytes += e.Size
		}
		p.Items = append(p.Items, item)
	}

	for _, s := range byRoot {
		p.Roots = append(p.Roots, *s)
	}
	sort.Slice(p.Roots, func(i, j int) bool { return p.Roots[i].Bytes > p.Roots[j].Bytes })
	for r := range unknown {
		p.Unknown = append(p.Unknown, r)
	}
	sort.Slice(p.Unknown, func(i, j int) bool { return p.Unknown[i] < p.Unknown[j] })
	return p, nil
}

// ExecOptions configures the execution of the plan.
type ExecOptions struct {
	// OnFile is called after each file, with its error if there was one.
	OnFile func(Item, error)
	// OnProgress receives the bytes written and the planned total.
	OnProgress func(done, total int64)
}

// Execute writes the plan. Each file goes into a temporary in the same directory, is verified
// against the hash in the index, and only then is renamed into place. A file that fails the
// check never reaches its destination.
func (p *Plan) Execute(ctx context.Context, o ExecOptions) (*pack.ExtractReport, error) {
	byEntry := make(map[*pack.Entry]Item, len(p.Items))
	for _, it := range p.Items {
		byEntry[it.Entry] = it
	}
	return p.reader().Extract(ctx, pack.ExtractOptions{
		Filter: func(e *pack.Entry) bool {
			it, ok := byEntry[e]
			return ok && it.Action != SkipIt
		},
		Open: func(e *pack.Entry) (pack.Sink, error) {
			it := byEntry[e]
			return newFileSink(it, p.target())
		},
		OnFile: func(e *pack.Entry, err error) {
			if o.OnFile != nil {
				o.OnFile(byEntry[e], err)
			}
		},
		OnProgress: o.OnProgress,
	})
}

// reader and target are set by Build through the options; we keep them on the plan so Execute
// does not have to ask for them again.
func (p *Plan) reader() *pack.Reader { return p.rd }
func (p *Plan) target() system.OS    { return p.os }

// fileSink is a file's destination: a temporary next to the final place, renamed only after Commit.
type fileSink struct {
	item   Item
	target system.OS
	tmp    string
	f      *os.File
}

func newFileSink(it Item, target system.OS) (pack.Sink, error) {
	dir := filepath.Dir(it.Dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// The temporary name starts with a dot and has its own suffix: visibly in progress, impossible
	// to mistake for the final file.
	tmp := filepath.Join(dir, "."+filepath.Base(it.Dest)+".dhs-tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &fileSink{item: it, target: target, tmp: tmp, f: f}, nil
}

func (s *fileSink) Write(b []byte) (int, error) { return s.f.Write(b) }

func (s *fileSink) Commit() error {
	if err := s.f.Sync(); err != nil {
		s.Abort()
		return err
	}
	if err := s.f.Close(); err != nil {
		os.Remove(s.tmp)
		return err
	}
	if s.target != system.Windows {
		if mode := os.FileMode(s.item.Entry.Mode) & os.ModePerm; mode != 0 {
			_ = os.Chmod(s.tmp, mode)
		}
	}
	if s.item.Action == Replace {
		// On Windows, Rename does not overwrite; we remove the target first. On Linux, Rename is atomic anyway.
		if s.target == system.Windows {
			_ = os.Remove(s.item.Dest)
		}
	}
	if err := os.Rename(s.tmp, s.item.Dest); err != nil {
		os.Remove(s.tmp)
		return err
	}
	if s.item.Entry.ModTime > 0 {
		mt := time.Unix(0, s.item.Entry.ModTime)
		_ = os.Chtimes(s.item.Dest, mt, mt)
	}
	return nil
}

func (s *fileSink) Abort() error {
	_ = s.f.Close()
	return os.Remove(s.tmp)
}
