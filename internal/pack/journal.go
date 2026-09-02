package pack

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	manifestName = "dhs.json"
	indexName    = "index.dhsi"
	sumsName     = "SUME.txt"
	journalName  = "jurnal.jsonl"
	volumeDir    = "volume"
)

func volumeName(n uint32) string { return fmt.Sprintf("%04d.dhsv", n) }

// JournalLine e o intrare din jurnal. Jurnalul e necriptat, deci nu conține nume de fișiere —
// doar ce volum s-a încheiat și când.
type JournalLine struct {
	Type   string    `json:"t"` // „volum" | „complet"
	Volume uint32    `json:"n,omitempty"`
	Bytes  int64     `json:"octeti,omitempty"`
	SHA256 *Hash     `json:"sha256,omitempty"`
	When   time.Time `json:"cand"`
}

// appendJournal adaugă o linie și o forțează pe disc: jurnalul e ce citim la reluare, deci nu are
// voie să rămână în cache-ul sistemului când se smulge cablul.
func appendJournal(dir string, line JournalLine) error {
	f, err := os.OpenFile(filepath.Join(dir, journalName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// readJournal întoarce liniile existente; un jurnal lipsă e o listă goală, nu o eroare.
func readJournal(dir string) ([]JournalLine, error) {
	f, err := os.Open(filepath.Join(dir, journalName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []JournalLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var l JournalLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			// O linie ruptă la final e exact urma unei întreruperi; o ignorăm, restul e valid.
			break
		}
		out = append(out, l)
	}
	return out, sc.Err()
}

// writeAtomic scrie un fișier prin nume temporar + redenumire, cu fsync. Fie fișierul vechi
// întreg, fie cel nou întreg — niciodată o versiune pe jumătate.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return syncDir(filepath.Dir(path))
}

// syncDir forțează pe disc intrarea de director — redenumirea în sine. Pe Windows nu se poate
// deschide un director așa; acolo redenumirea e oricum durabilă la închidere.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return nil //nolint:nilerr
	}
	defer d.Close()
	_ = d.Sync()
	return nil
}

// Sums e conținutul lui SUME.txt: sha256 → cale relativă, în formatul `sha256sum -c`.
type Sums map[string]Hash

func (s Sums) encode() []byte {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s  %s\n", s[n], n)
	}
	return []byte(b.String())
}

func writeSums(dir string, s Sums) error {
	return writeAtomic(filepath.Join(dir, sumsName), s.encode(), 0o644)
}

func readSums(dir string) (Sums, error) {
	f, err := os.Open(filepath.Join(dir, sumsName))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := Sums{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		hex, name, ok := strings.Cut(line, "  ")
		if !ok {
			return nil, fmt.Errorf("%w: linie invalidă în %s", ErrCorrupt, sumsName)
		}
		var h Hash
		if err := h.UnmarshalText([]byte(hex)); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		out[name] = h
	}
	return out, sc.Err()
}

// hashFile calculează SHA-256 al unui fișier de pe disc.
func hashFile(path string) (Hash, int64, error) {
	var h Hash
	f, err := os.Open(path)
	if err != nil {
		return h, 0, err
	}
	defer f.Close()
	hs := sha256.New()
	n, err := io.Copy(hs, f)
	if err != nil {
		return h, 0, err
	}
	copy(h[:], hs.Sum(nil))
	return h, n, nil
}

// hashWriter numără și hash-uiește ce trece prin el — pentru fișierul de volum, pe disc.
type hashWriter struct {
	w io.Writer
	h [32]byte
	s interface {
		io.Writer
		Sum([]byte) []byte
	}
	n int64
}

func newHashWriter(w io.Writer) *hashWriter { return &hashWriter{w: w, s: sha256.New()} }

func (hw *hashWriter) Write(p []byte) (int, error) {
	n, err := hw.w.Write(p)
	hw.s.Write(p[:n])
	hw.n += int64(n)
	return n, err
}

func (hw *hashWriter) Sum() Hash {
	var h Hash
	copy(h[:], hw.s.Sum(nil))
	return h
}
