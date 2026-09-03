package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is a file found in the inventory.
type Entry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Class  Class  `json:"class"`
	Secret string `json:"secret,omitempty"` // the reason, if it is a secret
}

// Bucket counts how many files and how many bytes are in a category.
type Bucket struct {
	Files int64 `json:"files"`
	Bytes int64 `json:"bytes"`
}

func (b *Bucket) add(size int64) {
	b.Files++
	b.Bytes += size
}

// Skipped is what an exclusion rule skipped.
type Skipped struct {
	Rule   Rule   `json:"-"`
	Dir    string `json:"dir"`
	Reason string `json:"reason"`
	Bucket
}

// Options configures the walk.
type Options struct {
	// Roots are the roots to walk.
	Roots []string
	// Excluder decides which directories are skipped. Nil means the default rules.
	Excluder *Excluder
	// IncludeSecrets brings secrets into the main inventory. By default they are only counted separately.
	IncludeSecrets bool
	// TopN is how many of the largest files are kept, so the user can see what takes up the space.
	TopN int
	// OnEntry, if given, receives every included file. It lets the following phases (hashing,
	// deduplication) work in a stream, without the inventory holding everything in memory.
	OnEntry func(Entry)
	// OnProgress is called now and then, with the number of files seen so far.
	OnProgress func(files int64, bytes int64)
}

// Result is the aggregated inventory.
type Result struct {
	Roots    []string         `json:"roots"`
	Files    int64            `json:"files"`
	Bytes    int64            `json:"bytes"`
	Dirs     int64            `json:"dirs"`
	ByClass  map[Class]Bucket `json:"by_class"`
	Skipped  []Skipped        `json:"skipped"`
	Secrets  Bucket           `json:"secrets"`
	Largest  []Entry          `json:"largest"`
	Errors   []string         `json:"errors,omitempty"`
	Symlinks int64            `json:"symlinks"`
}

// SkippedBytes is the total excluded by the rules.
func (r *Result) SkippedBytes() int64 {
	var n int64
	for _, s := range r.Skipped {
		n += s.Bytes
	}
	return n
}

const progressEvery = 2000

// Walk walks the roots and returns the inventory. It opens no file: it looks only at names and
// sizes, so that a scan over hundreds of thousands of files takes seconds.
//
// Symbolic links are never followed -- otherwise a link to "/" would lead to an infinite cycle,
// and one to an already-included directory would double the data.
func Walk(opts Options) (*Result, error) {
	if len(opts.Roots) == 0 {
		return nil, errors.New("scan: no roots to walk")
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
		// A file given directly as a root is included without question: the user asked for it specifically.
		if st, err := os.Lstat(abs); err == nil && !st.IsDir() {
			addFile(res, &largest, topN, opts, abs, st.Size())
			continue
		}

		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// A directory without read permission does not stop the whole scan.
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

			// No symbolic links, no sockets, no special files.
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

// keepLargest keeps a short list of the largest files sorted. For a small topN (15), direct
// insertion is faster than a heap and keeps the list ready-sorted.
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

// measure adds up the size of an excluded subtree, so we can say how much was skipped and why.
func measure(root string) (files, bytes int64) {
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil //nolint:nilerr // an unreadable subtree is reported as zero, it does not stop the scan
		}
		if info, err := d.Info(); err == nil {
			files++
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes
}

// HomeRoots returns the roots enabled by default from a profile: the standard places that exist.
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
		// Skip a directory that is already under another chosen root.
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
