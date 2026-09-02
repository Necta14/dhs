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

// Conflict spune ce facem când destinația există deja.
type Conflict string

const (
	// KeepBoth păstrează fișierul existent neatins și scrie pe cel restaurat alături, cu sufix.
	// E implicitul: nu pierdem niciodată ceva ce era deja pe disc.
	KeepBoth Conflict = "alaturi"
	// Skip nu scrie fișierul restaurat dacă destinația există.
	Skip Conflict = "sari"
	// Overwrite înlocuiește fișierul existent. Doar la cerere explicită.
	Overwrite Conflict = "suprascrie"
)

// ParseConflict citește politica din text.
func ParseConflict(s string) (Conflict, error) {
	switch Conflict(s) {
	case KeepBoth, Skip, Overwrite:
		return Conflict(s), nil
	case "":
		return KeepBoth, nil
	}
	return "", fmt.Errorf("politică de conflict necunoscută %q (alaturi | sari | suprascrie)", s)
}

// Action e ce va face planul cu o intrare.
type Action string

const (
	Write     Action = "scrie"
	WriteNext Action = "scrie-alaturi"
	SkipIt    Action = "sare"
	Replace   Action = "suprascrie"
)

// Item e o intrare din pachet cu destinația ei rezolvată.
type Item struct {
	Entry   *pack.Entry
	Dest    string // calea absolută finală
	Action  Action
	Renamed bool   // numele a fost adaptat sistemului destinație
	Exists  bool   // destinația exista înainte
	Note    string // de ce s-a redenumit sau s-a mutat
}

// RootSummary e ce merge unde, pe scurt, pentru afișare.
type RootSummary struct {
	Root  pack.Root
	Dest  string
	Files int
	Bytes int64
}

// Plan e tot ce urmează să se întâmple, calculat înainte de orice scriere.
type Plan struct {
	Items     []Item
	Files     int
	Bytes     int64 // ce se va scrie efectiv (fără cele sărite)
	Roots     []RootSummary
	Conflicts int
	Renamed   int
	Skipped   int
	Unknown   []pack.Root // rădăcini din pachet care nu există pe sistemul ăsta
	Policy    Conflict

	rd *pack.Reader
	os system.OS
}

// Options configurează planul.
type Options struct {
	Reader *pack.Reader
	Roots  *pack.RootMap
	Target system.OS
	// Filter alege ce se restaurează; nil = tot.
	Filter   func(*pack.Entry) bool
	Conflict Conflict
	// Suffix e ce se adaugă numelui când păstrăm ambele. Implicit „ (DHS)".
	Suffix string
}

// Build calculează planul. Nu atinge discul decât pentru a verifica ce există deja.
func Build(o Options) (*Plan, error) {
	if o.Reader == nil || o.Roots == nil {
		return nil, errors.New("restore: lipsesc cititorul sau harta rădăcinilor")
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
	// Cheile deja folosite în plan, ca două intrări să nu ajungă în același fișier pe Windows.
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

		// Rădăcina nu există pe sistemul ăsta (ex. „desktop" pe un server): merge sub DHS-restaurat.
		if _, ok := o.Roots.Join(root, "."); !ok && root != pack.RootOther {
			unknown[root] = true
			rel = string(root) + "/" + rel
			root = pack.RootOther
			item.Note = "loc standard inexistent aici"
		}

		clean, renamed := Sanitize(rel, o.Target)
		if renamed {
			item.Renamed = true
			item.Note = "nume adaptat sistemului"
		}

		// Coliziune de majuscule cu altă intrare din același plan.
		key := string(root) + "\x00" + FoldKey(clean, o.Target)
		if taken[key] {
			clean = WithSuffix(clean, " (2)")
			key = string(root) + "\x00" + FoldKey(clean, o.Target)
			for n := 3; taken[key]; n++ {
				clean = WithSuffix(clean, fmt.Sprintf(" (%d)", n))
				key = string(root) + "\x00" + FoldKey(clean, o.Target)
			}
			item.Renamed = true
			item.Note = "se ciocnea cu alt fișier din pachet"
		}
		taken[key] = true

		dest, ok := o.Roots.Join(root, clean)
		if !ok {
			return nil, fmt.Errorf("restore: nu pot rezolva %s/%s", root, clean)
		}
		item.Dest = dest
		item.Action = Write

		if st, err := os.Lstat(dest); err == nil {
			item.Exists = true
			p.Conflicts++
			switch {
			case st.IsDir():
				// Un director cu numele fișierului nostru: nu ne batem cu el.
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

// ExecOptions configurează execuția planului.
type ExecOptions struct {
	// OnFile e chemat după fiecare fișier, cu eroarea lui dacă a fost.
	OnFile func(Item, error)
	// OnProgress primește octeții scriși și totalul planificat.
	OnProgress func(done, total int64)
}

// Execute scrie planul. Fiecare fișier merge într-un temporar din același director, se verifică
// față de hash-ul din index și abia apoi e redenumit la locul lui. Un fișier care nu trece nu
// ajunge niciodată la destinație.
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

// reader și target sunt setate de Build prin opțiuni; le păstrăm pe plan ca Execute să nu le
// ceară din nou.
func (p *Plan) reader() *pack.Reader { return p.rd }
func (p *Plan) target() system.OS    { return p.os }

// fileSink e destinația unui fișier: temporar lângă locul final, redenumit doar după Commit.
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
	// Numele temporar începe cu punct și are sufix propriu: vizibil că e în lucru, imposibil de
	// confundat cu fișierul final.
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
		// Pe Windows, Rename nu suprascrie; scoatem întâi ținta. Pe Linux, Rename e atomic oricum.
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
