package pack

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Necta14/dhs/internal/passphrase"
)

// Reader deschide un pachet și citește din el.
type Reader struct {
	Dir      string
	Manifest Manifest
	Index    Index
	pass     string
}

// Peek citește doar manifestul necriptat — ce se poate spune despre pachet înainte de parolă.
func Peek(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, ErrNotPackage
		}
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("%w: %s invalid: %v", ErrNotPackage, manifestName, err)
	}
	if m.Format > FormatVersion {
		return m, fmt.Errorf("%w: formatul %d, eu știu până la %d", ErrNewerFormat, m.Format, FormatVersion)
	}
	return m, nil
}

// Open citește manifestul și indexul. Pentru un pachet criptat fără frază întoarce ErrNeedPassphrase,
// ca interfața să știe că trebuie să o ceară.
func Open(dir, pass string) (*Reader, error) {
	m, err := Peek(dir)
	if err != nil {
		return nil, err
	}
	if m.Cipher != CipherNone.String() && pass == "" {
		return nil, ErrNeedPassphrase
	}
	r := &Reader{Dir: dir, Manifest: m, pass: pass}
	idx, err := r.readIndexFile()
	if err != nil {
		return nil, err
	}
	if idx.ID != m.ID {
		return nil, fmt.Errorf("%w: indexul nu aparține acestui pachet", ErrCorrupt)
	}
	r.Index = *idx
	return r, nil
}

// Complete spune dacă scrierea s-a încheiat. Un pachet incomplet poate fi citit parțial.
func (r *Reader) Complete() bool { return r.Manifest.Complete && r.Index.Complete }

func (r *Reader) readIndexFile() (*Index, error) {
	f, err := os.Open(filepath.Join(r.Dir, indexName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: lipsește %s", ErrIncomplete, indexName)
		}
		return nil, err
	}
	defer f.Close()

	vs, err := openStream(f, r.pass, r.Manifest.ID, 0)
	if err != nil {
		return nil, err
	}
	bh, stored, err := vs.nextBlock()
	if err != nil {
		return nil, err
	}
	if bh.Kind != KindIndex {
		return nil, fmt.Errorf("%w: %s nu începe cu un bloc de index", ErrCorrupt, indexName)
	}
	payload, err := decodeBlock(bh, stored)
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(payload, &idx); err != nil {
		return nil, fmt.Errorf("%w: index ilizibil: %v", ErrCorrupt, err)
	}
	return &idx, nil
}

// stream e un volum deschis pentru citire secvențială, în clar, cu poziția urmărită.
type stream struct {
	r   *bufio.Reader
	pos int64
	hdr volumeHeader
}

func openStream(f io.Reader, pass string, want ID, wantVolume uint32) (*stream, error) {
	dec, err := passphrase.Decrypt(f, pass)
	if err != nil {
		if errors.Is(err, passphrase.ErrWrong) {
			return nil, ErrBadPassphrase
		}
		return nil, err
	}
	s := &stream{r: bufio.NewReaderSize(dec, 1<<20)}
	b, err := readFull(s.r, volumeHeaderSize)
	if err != nil {
		// age dă eroarea de parolă abia la prima citire, nu la deschidere.
		if errors.Is(err, ErrCorrupt) {
			return nil, err
		}
		return nil, err
	}
	s.pos = int64(len(b))
	hdr, err := decodeVolumeHeader(b)
	if err != nil {
		return nil, err
	}
	if hdr.ID != want {
		return nil, ErrWrongPackage
	}
	if hdr.Volume != wantVolume {
		return nil, fmt.Errorf("%w: aștept volumul %d, fișierul spune %d", ErrCorrupt, wantVolume, hdr.Volume)
	}
	s.hdr = hdr
	return s, nil
}

// nextBlock citește antetul și conținutul stocat al blocului următor.
func (s *stream) nextBlock() (blockHeader, []byte, error) {
	hb, err := readFull(s.r, blockHeaderSize)
	if err != nil {
		return blockHeader{}, nil, err
	}
	bh, err := decodeBlockHeader(hb)
	if err != nil {
		return bh, nil, err
	}
	stored, err := readFull(s.r, int(bh.Stored))
	if err != nil {
		return bh, nil, err
	}
	s.pos += int64(blockHeaderSize) + bh.Stored
	return bh, stored, nil
}

// trailer citește și validează trailerul, după blocul de index.
func (s *stream) trailer() (trailer, error) {
	b, err := readFull(s.r, trailerSize)
	if err != nil {
		return trailer{}, err
	}
	t, err := decodeTrailer(b)
	if err != nil {
		return t, err
	}
	if t.StreamLen != s.pos {
		return t, fmt.Errorf("%w: trailerul spune %d octeți, am citit %d", ErrCorrupt, t.StreamLen, s.pos)
	}
	return t, nil
}

// decodeBlock decomprimă și verifică hash-ul: un bloc nu e acceptat până nu e dovedit întreg.
func decodeBlock(bh blockHeader, stored []byte) ([]byte, error) {
	raw, err := decompress(bh.Kind, stored, bh.Raw)
	if err != nil {
		return nil, err
	}
	if sum := sha256.Sum256(raw); sum != [32]byte(bh.SHA256) {
		return nil, fmt.Errorf("%w: blocul nu se potrivește cu suma lui de control", ErrCorrupt)
	}
	return raw, nil
}

// ───────────────────────────── verificare ─────────────────────────────

// VerifyReport e rezultatul verificării.
type VerifyReport struct {
	Files    int   // fișiere din pachet verificate după SUME.txt
	Volumes  int   // volume parcurse
	Blocks   int   // blocuri decomprimate și verificate
	Bytes    int64 // octeți bruți verificați
	Problems []string
}

// OK spune dacă nu s-a găsit nimic în neregulă.
func (v *VerifyReport) OK() bool { return len(v.Problems) == 0 }

// Verify verifică pachetul de la un capăt la altul: sumele fișierelor de pe disc, apoi fiecare
// bloc din fiecare volum, decomprimat și comparat cu hash-ul lui. Nu extrage nimic.
func (r *Reader) Verify(ctx context.Context, onProgress func(done, total int64)) (*VerifyReport, error) {
	rep := &VerifyReport{}
	problem := func(format string, a ...any) { rep.Problems = append(rep.Problems, fmt.Sprintf(format, a...)) }

	sums, err := readSums(r.Dir)
	if err != nil {
		problem("%s: %v", sumsName, err)
	}
	for name, want := range sums {
		got, _, err := hashFile(filepath.Join(r.Dir, filepath.FromSlash(name)))
		if err != nil {
			problem("%s: %v", name, err)
			continue
		}
		if got != want {
			problem("%s: suma nu se potrivește", name)
		}
		rep.Files++
	}
	for _, v := range r.Index.Volumes {
		if _, ok := sums[filepath.ToSlash(filepath.Join(volumeDir, volumeName(v.Number)))]; !ok {
			problem("volumul %d nu apare în %s", v.Number, sumsName)
		}
	}

	var total int64
	for _, v := range r.Index.Volumes {
		total += v.Bytes
	}
	var done int64
	for _, v := range r.Index.Volumes {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		n, err := r.verifyVolume(v, rep)
		rep.Volumes++
		rep.Bytes += n
		done += v.Bytes
		if err != nil {
			problem("volumul %d: %v", v.Number, err)
		}
		if onProgress != nil {
			onProgress(done, total)
		}
	}
	if !r.Complete() {
		problem("pachetul e marcat incomplet")
	}
	return rep, nil
}

func (r *Reader) verifyVolume(v VolumeInfo, rep *VerifyReport) (int64, error) {
	f, err := os.Open(volumeFile(r.Dir, v.Number))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	s, err := openStream(f, r.pass, r.Manifest.ID, v.Number)
	if err != nil {
		return 0, err
	}
	var raw int64
	blocks := 0
	for {
		bh, stored, err := s.nextBlock()
		if err != nil {
			return raw, err
		}
		if _, err := decodeBlock(bh, stored); err != nil {
			return raw, fmt.Errorf("blocul %d: %w", blocks, err)
		}
		if bh.Kind == KindIndex {
			break
		}
		blocks++
		rep.Blocks++
		raw += bh.Raw
	}
	t, err := s.trailer()
	if err != nil {
		return raw, err
	}
	if int(t.Blocks) != blocks || blocks != v.Blocks {
		return raw, fmt.Errorf("%w: %d blocuri în flux, trailerul spune %d, indexul %d", ErrCorrupt, blocks, t.Blocks, v.Blocks)
	}
	return raw, nil
}

// ───────────────────────────── extragere ─────────────────────────────

// Sink primește conținutul unui fișier extras. Commit se cheamă doar după ce hash-ul s-a
// verificat; Abort, dacă ceva a mers prost — și atunci nimic nu trebuie să rămână la destinație.
type Sink interface {
	io.Writer
	Commit() error
	Abort() error
}

// ExtractOptions configurează extragerea.
type ExtractOptions struct {
	// Filter alege ce intrări se extrag; nil = toate.
	Filter func(*Entry) bool
	// Open deschide destinația unei intrări. Se cheamă o singură dată per intrare, la prima ei parte.
	Open func(*Entry) (Sink, error)
	// OnFile e chemat după fiecare fișier încheiat, reușit sau nu.
	OnFile func(*Entry, error)
	// OnProgress primește octeții bruți scriși și totalul de scris.
	OnProgress func(done, total int64)
}

// ExtractReport e rezultatul extragerii.
type ExtractReport struct {
	Files  int
	Bytes  int64
	Failed []Failure
}

// Failure e o intrare care n-a putut fi extrasă. Nimic nu s-a scris pentru ea.
type Failure struct {
	Entry *Entry
	Err   error
}

type sinkState struct {
	e      *Entry
	sink   Sink
	hasher interface {
		io.Writer
		Sum([]byte) []byte
	}
	written int64
	left    int // părți rămase
}

// Extract parcurge volumele o singură dată, secvențial, și scrie fiecare fișier pe măsură ce îi
// apar părțile. Blocurile de care nu are nevoie nimeni nu se decomprimă deloc. Un fișier ajunge la
// Commit numai dacă hash-ul lui întreg e cel din index.
func (r *Reader) Extract(ctx context.Context, opts ExtractOptions) (*ExtractReport, error) {
	if opts.Open == nil {
		return nil, errors.New("pack: ExtractOptions.Open lipsește")
	}
	rep := &ExtractReport{}

	// Ce părți așteaptă fiecare bloc, și cât avem de scris în total.
	type ref struct {
		e   *Entry
		p   Part
		idx int
	}
	byBlock := make(map[int][]ref)
	var total int64
	wanted := 0
	for _, e := range r.Index.Entries {
		if opts.Filter != nil && !opts.Filter(e) {
			continue
		}
		wanted++
		total += e.Size
		if len(e.Parts) == 0 {
			// Fișier gol: se creează direct.
			if err := r.emitEmpty(e, opts, rep); err != nil {
				return rep, err
			}
			continue
		}
		for i, p := range e.Parts {
			if p.Block < 0 || p.Block >= len(r.Index.Blocks) {
				rep.Failed = append(rep.Failed, Failure{e, fmt.Errorf("%w: parte către blocul inexistent %d", ErrCorrupt, p.Block)})
				break
			}
			byBlock[p.Block] = append(byBlock[p.Block], ref{e, p, i})
		}
	}
	if wanted == 0 {
		return rep, nil
	}

	// Pentru fiecare volum, ce blocuri (id) îl compun, în ordinea pozițiilor.
	byVolume := make(map[uint32][]int)
	for id, b := range r.Index.Blocks {
		if b.Volume == 0 {
			continue
		}
		byVolume[b.Volume] = append(byVolume[b.Volume], id)
	}

	open := make(map[*Entry]*sinkState)
	var done int64

	for _, v := range r.Index.Volumes {
		if err := ctx.Err(); err != nil {
			r.abortAll(open, rep, err)
			return rep, err
		}
		ids := byVolume[v.Number]
		// Sărim volumul dacă nimeni nu are nevoie de el.
		needed := false
		for _, id := range ids {
			if len(byBlock[id]) > 0 {
				needed = true
				break
			}
		}
		if !needed {
			continue
		}

		f, err := os.Open(volumeFile(r.Dir, v.Number))
		if err != nil {
			r.abortAll(open, rep, err)
			return rep, err
		}
		s, err := openStream(f, r.pass, r.Manifest.ID, v.Number)
		if err != nil {
			f.Close()
			r.abortAll(open, rep, err)
			return rep, err
		}

		for ordinal := 0; ; ordinal++ {
			if err := ctx.Err(); err != nil {
				f.Close()
				r.abortAll(open, rep, err)
				return rep, err
			}
			bh, stored, err := s.nextBlock()
			if err != nil {
				f.Close()
				r.abortAll(open, rep, err)
				return rep, fmt.Errorf("volumul %d: %w", v.Number, err)
			}
			if bh.Kind == KindIndex {
				break
			}
			if ordinal >= len(ids) {
				f.Close()
				err := fmt.Errorf("%w: volumul %d are mai multe blocuri decât știe indexul", ErrCorrupt, v.Number)
				r.abortAll(open, rep, err)
				return rep, err
			}
			id := ids[ordinal]
			refs := byBlock[id]
			if len(refs) == 0 {
				continue
			}
			raw, err := decodeBlock(bh, stored)
			if err != nil {
				// Un bloc stricat compromite doar fișierele care trec prin el.
				for _, rf := range refs {
					r.failEntry(open, rf.e, rep, fmt.Errorf("blocul %d din volumul %d: %w", ordinal, v.Number, err), opts)
				}
				continue
			}
			for _, rf := range refs {
				st := open[rf.e]
				if st == nil {
					if failedAlready(rep, rf.e) {
						continue
					}
					sink, err := opts.Open(rf.e)
					if err != nil {
						r.failEntry(open, rf.e, rep, err, opts)
						continue
					}
					st = &sinkState{e: rf.e, sink: sink, hasher: sha256.New(), left: len(rf.e.Parts)}
					open[rf.e] = st
				}
				end := rf.p.Offset + rf.p.Length
				if rf.p.Offset < 0 || end > int64(len(raw)) {
					r.failEntry(open, rf.e, rep, fmt.Errorf("%w: partea depășește blocul", ErrCorrupt), opts)
					continue
				}
				piece := raw[rf.p.Offset:end]
				if _, err := st.sink.Write(piece); err != nil {
					r.failEntry(open, rf.e, rep, err, opts)
					continue
				}
				st.hasher.Write(piece)
				st.written += int64(len(piece))
				st.left--
				done += int64(len(piece))
				if st.left == 0 {
					r.finishEntry(open, st, rep, opts)
				}
			}
			if opts.OnProgress != nil {
				opts.OnProgress(done, total)
			}
		}
		if _, err := s.trailer(); err != nil {
			f.Close()
			r.abortAll(open, rep, err)
			return rep, fmt.Errorf("volumul %d: %w", v.Number, err)
		}
		f.Close()
	}

	// Ce a rămas deschis are părți în volume care lipsesc.
	for _, st := range open {
		r.failEntry(open, st.e, rep, fmt.Errorf("%w: lipsesc părți din fișier", ErrIncomplete), opts)
	}
	return rep, nil
}

func (r *Reader) emitEmpty(e *Entry, opts ExtractOptions, rep *ExtractReport) error {
	sink, err := opts.Open(e)
	if err != nil {
		rep.Failed = append(rep.Failed, Failure{e, err})
		if opts.OnFile != nil {
			opts.OnFile(e, err)
		}
		return nil
	}
	if sum := sha256.Sum256(nil); sum != [32]byte(e.SHA256) {
		_ = sink.Abort()
		err := fmt.Errorf("%w: fișier gol cu hash nenul", ErrCorrupt)
		rep.Failed = append(rep.Failed, Failure{e, err})
		if opts.OnFile != nil {
			opts.OnFile(e, err)
		}
		return nil
	}
	if err := sink.Commit(); err != nil {
		rep.Failed = append(rep.Failed, Failure{e, err})
		if opts.OnFile != nil {
			opts.OnFile(e, err)
		}
		return nil
	}
	rep.Files++
	if opts.OnFile != nil {
		opts.OnFile(e, nil)
	}
	return nil
}

func (r *Reader) finishEntry(open map[*Entry]*sinkState, st *sinkState, rep *ExtractReport, opts ExtractOptions) {
	delete(open, st.e)
	var got Hash
	copy(got[:], st.hasher.Sum(nil))
	if got != st.e.SHA256 || st.written != st.e.Size {
		_ = st.sink.Abort()
		err := fmt.Errorf("%w: fișierul refăcut nu se potrivește cu originalul", ErrCorrupt)
		rep.Failed = append(rep.Failed, Failure{st.e, err})
		if opts.OnFile != nil {
			opts.OnFile(st.e, err)
		}
		return
	}
	if err := st.sink.Commit(); err != nil {
		rep.Failed = append(rep.Failed, Failure{st.e, err})
		if opts.OnFile != nil {
			opts.OnFile(st.e, err)
		}
		return
	}
	rep.Files++
	rep.Bytes += st.written
	if opts.OnFile != nil {
		opts.OnFile(st.e, nil)
	}
}

func (r *Reader) failEntry(open map[*Entry]*sinkState, e *Entry, rep *ExtractReport, err error, opts ExtractOptions) {
	if st, ok := open[e]; ok {
		_ = st.sink.Abort()
		delete(open, e)
	}
	if failedAlready(rep, e) {
		return
	}
	rep.Failed = append(rep.Failed, Failure{e, err})
	if opts.OnFile != nil {
		opts.OnFile(e, err)
	}
}

func (r *Reader) abortAll(open map[*Entry]*sinkState, rep *ExtractReport, err error) {
	for e, st := range open {
		_ = st.sink.Abort()
		delete(open, e)
		rep.Failed = append(rep.Failed, Failure{e, err})
	}
}

func failedAlready(rep *ExtractReport, e *Entry) bool {
	for _, f := range rep.Failed {
		if f.Entry == e {
			return true
		}
	}
	return false
}
