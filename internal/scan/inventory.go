package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry e un fișier găsit în inventar.
type Entry struct {
	Path   string `json:"cale"`
	Size   int64  `json:"octeti"`
	Class  Class  `json:"clasa"`
	Secret string `json:"secret,omitempty"` // motivul, dacă e secret
}

// Bucket adună câte fișiere și câți octeți sunt într-o categorie.
type Bucket struct {
	Files int64 `json:"fisiere"`
	Bytes int64 `json:"octeti"`
}

func (b *Bucket) add(size int64) {
	b.Files++
	b.Bytes += size
}

// Skipped e ce a sărit o regulă de excludere.
type Skipped struct {
	Rule   Rule   `json:"-"`
	Dir    string `json:"director"`
	Reason string `json:"motiv"`
	Bucket
}

// Options configurează parcurgerea.
type Options struct {
	// Roots sunt rădăcinile de parcurs.
	Roots []string
	// Excluder decide ce directoare se sar. Nil înseamnă regulile implicite.
	Excluder *Excluder
	// IncludeSecrets aduce secretele în inventarul principal. Implicit sunt doar numărate separat.
	IncludeSecrets bool
	// TopN e câte dintre cele mai mari fișiere se rețin, ca utilizatorul să vadă ce ocupă spațiul.
	TopN int
	// OnEntry, dacă e dat, primește fiecare fișier inclus. Permite fazelor următoare (hash,
	// deduplicare) să lucreze în flux, fără ca inventarul să țină totul în memorie.
	OnEntry func(Entry)
	// OnProgress e chemat din când în când, cu numărul de fișiere văzute până acum.
	OnProgress func(files int64, bytes int64)
}

// Result e inventarul agregat.
type Result struct {
	Roots    []string         `json:"radacini"`
	Files    int64            `json:"fisiere"`
	Bytes    int64            `json:"octeti"`
	Dirs     int64            `json:"directoare"`
	ByClass  map[Class]Bucket `json:"pe_clasa"`
	Skipped  []Skipped        `json:"excluse"`
	Secrets  Bucket           `json:"secrete"`
	Largest  []Entry          `json:"cele_mai_mari"`
	Errors   []string         `json:"erori,omitempty"`
	Symlinks int64            `json:"legaturi"`
}

// SkippedBytes e totalul exclus de reguli.
func (r *Result) SkippedBytes() int64 {
	var n int64
	for _, s := range r.Skipped {
		n += s.Bytes
	}
	return n
}

const progressEvery = 2000

// Walk parcurge rădăcinile și întoarce inventarul. Nu deschide niciun fișier: se uită doar la
// nume și la dimensiune, ca un scan pe sute de mii de fișiere să dureze secunde.
//
// Legăturile simbolice nu se urmăresc niciodată — altfel un link către „/" ar duce la un ciclu
// infinit, iar unul către un director deja inclus ar dubla datele.
func Walk(opts Options) (*Result, error) {
	if len(opts.Roots) == 0 {
		return nil, errors.New("scan: nicio rădăcină de parcurs")
	}
	ex := opts.Excluder
	if ex == nil {
		ex = DefaultExcluder()
	}
	topN := opts.TopN
	if topN <= 0 {
		topN = 15
	}

	res := &Result{
		Roots:   append([]string(nil), opts.Roots...),
		ByClass: make(map[Class]Bucket, 4),
	}
	skipped := make(map[string]*Skipped)
	largest := make([]Entry, 0, topN+1)
	var sinceProgress int

	for _, root := range opts.Roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		// Un fișier dat direct ca rădăcină e inclus fără discuție: utilizatorul l-a cerut anume.
		if st, err := os.Lstat(abs); err == nil && !st.IsDir() {
			addFile(res, &largest, topN, opts, abs, st.Size())
			continue
		}

		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Un director fără drepturi de citire nu oprește tot scanul.
				res.Errors = append(res.Errors, err.Error())
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				if path != abs {
					if rule, hit := ex.Dir(d.Name()); hit {
						s := skipped[rule.Dir]
						if s == nil {
							s = &Skipped{Rule: rule, Dir: rule.Dir, Reason: rule.Reason}
							skipped[rule.Dir] = s
						}
						f, b := measure(path)
						s.Files += f
						s.Bytes += b
						return fs.SkipDir
					}
				}
				res.Dirs++
				return nil
			}

			// Nici legături simbolice, nici socketuri, nici fișiere speciale.
			if d.Type()&fs.ModeSymlink != 0 {
				res.Symlinks++
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
				return nil
			}
			addFile(res, &largest, topN, opts, path, info.Size())

			sinceProgress++
			if opts.OnProgress != nil && sinceProgress >= progressEvery {
				sinceProgress = 0
				opts.OnProgress(res.Files, res.Bytes)
			}
			return nil
		})
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
	}

	for _, s := range skipped {
		res.Skipped = append(res.Skipped, *s)
	}
	sort.Slice(res.Skipped, func(i, j int) bool { return res.Skipped[i].Bytes > res.Skipped[j].Bytes })
	res.Largest = largest
	if opts.OnProgress != nil {
		opts.OnProgress(res.Files, res.Bytes)
	}
	return res, nil
}

func addFile(res *Result, largest *[]Entry, topN int, opts Options, path string, size int64) {
	e := Entry{Path: path, Size: size, Class: ClassOf(path)}
	if reason, secret := IsSecret(path); secret {
		e.Secret = reason
		res.Secrets.add(size)
		if !opts.IncludeSecrets {
			return
		}
	}

	res.Files++
	res.Bytes += size
	b := res.ByClass[e.Class]
	b.add(size)
	res.ByClass[e.Class] = b

	keepLargest(largest, topN, e)
	if opts.OnEntry != nil {
		opts.OnEntry(e)
	}
}

// keepLargest ține sortată o listă scurtă cu cele mai mari fișiere. La topN mic (15), inserția
// directă e mai rapidă decât un heap și păstrează lista gata sortată.
func keepLargest(largest *[]Entry, topN int, e Entry) {
	l := *largest
	if len(l) == topN && e.Size <= l[len(l)-1].Size {
		return
	}
	i := sort.Search(len(l), func(i int) bool { return l[i].Size < e.Size })
	l = append(l, Entry{})
	copy(l[i+1:], l[i:])
	l[i] = e
	if len(l) > topN {
		l = l[:topN]
	}
	*largest = l
}

// measure adună dimensiunea unui subarbore exclus, ca să putem spune cât s-a sărit și de ce.
func measure(root string) (files, bytes int64) {
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil //nolint:nilerr // un subarbore ilizibil se raportează ca zero, nu oprește scanul
		}
		if info, err := d.Info(); err == nil {
			files++
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes
}

// HomeRoots întoarce rădăcinile pornite implicit dintr-un profil: locurile standard care există.
func HomeRoots(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		clean := filepath.Clean(p)
		if clean == "" || seen[clean] {
			continue
		}
		if st, err := os.Stat(clean); err != nil || !st.IsDir() {
			continue
		}
		// Sărim un director care e deja sub altă rădăcină aleasă.
		nested := false
		for existing := range seen {
			if strings.HasPrefix(clean+string(filepath.Separator), existing+string(filepath.Separator)) {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}
