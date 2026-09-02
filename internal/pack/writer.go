package pack

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/necta/dhs/internal/passphrase"
	"github.com/necta/dhs/internal/scan"
	"github.com/necta/dhs/internal/system"
)

// Options configurează scrierea unui pachet.
type Options struct {
	// Dir e directorul pachetului; se creează. Trebuie să fie gol sau inexistent.
	Dir        string
	Level      scan.Level
	Passphrase string // gol = fără criptare, alegere explicită a utilizatorului
	Source     system.Info
	VolumeSize int64 // implicit DefaultVolumeSize
	BlockSize  int   // implicit DefaultBlockSize
	Workers    int   // implicit runtime.NumCPU()
	Tool       string
	Now        func() time.Time
	OnProgress func(Progress)
}

// Progress e starea scrierii, raportată după fiecare fișier și după fiecare volum.
type Progress struct {
	Files  int64  // fișiere adăugate
	Bytes  int64  // octeți bruți citiți
	Stored int64  // octeți scriși pe disc, criptați
	Dedup  int64  // octeți economisiți prin deduplicare
	Volume uint32 // volumul în curs
}

// File e ce trebuie să știe scriitorul despre un fișier de adăugat.
type File struct {
	Root    Root
	Path    string // relativ la Root, cu „/"
	Orig    string // calea absolută originală
	Size    int64
	Mode    uint32
	ModTime time.Time
	Class   scan.Class
	Secret  bool
	// MaybeDup cere verificarea deduplicării: fișierul se citește o dată pentru hash și, dacă nu e
	// duplicat, încă o dată pentru împachetare. Apelantul o setează doar când altă intrare are
	// aceeași dimensiune — restul fișierelor n-au cum să fie duplicate.
	MaybeDup bool
	Open     func() (io.ReadCloser, error)
}

// Writer scrie un pachet. Nu e sigur pentru apeluri concurente la Add; compresia și scrierea pe
// disc merg însă în paralel, în spatele lui.
type Writer struct {
	o       Options
	id      ID
	created time.Time

	mu           sync.Mutex // entries, blocks, blockEntries
	entries      []*Entry
	keys         map[string]struct{}
	blocks       []BlockRef
	blockEntries map[int][]*Entry
	dedup        map[Hash]*Entry
	solid        map[BlockKind]*solidBuf
	storedBuf    []byte // blocul stocat în curs de umplere

	jobs    chan *job
	results chan *job
	workers sync.WaitGroup
	emitted chan struct{} // închis când emițătorul a terminat
	em      *emitter

	failMu sync.Mutex
	failed error

	progress Progress
	closed   bool
}

type job struct {
	id     int
	kind   BlockKind
	raw    []byte
	stored []byte
	err    error
}

type solidBuf struct {
	kind    BlockKind
	buf     []byte
	members []member
}

type member struct {
	e   *Entry
	off int64
	n   int64
}

// NewWriter creează directorul pachetului, scrie manifestul provizoriu și pornește conducta.
func NewWriter(o Options) (*Writer, error) {
	if !o.Level.Valid() {
		return nil, fmt.Errorf("pack: nivel de compresie invalid %d", o.Level)
	}
	if o.Passphrase != "" {
		if err := passphrase.Check(o.Passphrase); err != nil {
			return nil, err
		}
	}
	if o.VolumeSize <= 0 {
		o.VolumeSize = DefaultVolumeSize
	}
	if o.BlockSize <= 0 {
		o.BlockSize = DefaultBlockSize
	}
	if o.BlockSize > MaxBlockSize {
		return nil, fmt.Errorf("pack: blocul de %d depășește maximul %d", o.BlockSize, MaxBlockSize)
	}
	if o.Workers <= 0 {
		o.Workers = runtime.NumCPU()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Tool == "" {
		o.Tool = "dhs"
	}

	if err := ensureEmptyDir(o.Dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(o.Dir, volumeDir), 0o755); err != nil {
		return nil, err
	}

	id, err := NewID()
	if err != nil {
		return nil, err
	}

	w := &Writer{
		o:            o,
		id:           id,
		created:      o.Now().UTC(),
		keys:         make(map[string]struct{}),
		blockEntries: make(map[int][]*Entry),
		dedup:        make(map[Hash]*Entry),
		solid:        make(map[BlockKind]*solidBuf),
		jobs:         make(chan *job, o.Workers*2),
		results:      make(chan *job, o.Workers*2),
		emitted:      make(chan struct{}),
	}
	w.progress.Volume = 1

	if err := w.writeManifest(false); err != nil {
		return nil, err
	}

	for i := 0; i < o.Workers; i++ {
		w.workers.Add(1)
		go w.worker()
	}
	w.em = newEmitter(w)
	go w.em.run()

	return w, nil
}

func ensureEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("pack: directorul %s nu e gol — nu suprascriu nimic", dir)
	}
	return nil
}

// ID e identificatorul pachetului în curs.
func (w *Writer) ID() ID { return w.id }

// Dir e directorul pachetului.
func (w *Writer) Dir() string { return w.o.Dir }

// fail reține prima eroare; toate operațiile ulterioare o întorc.
func (w *Writer) fail(err error) {
	if err == nil {
		return
	}
	w.failMu.Lock()
	if w.failed == nil {
		w.failed = err
	}
	w.failMu.Unlock()
}

// Err e prima eroare apărută în conductă, dacă a apărut.
func (w *Writer) Err() error {
	w.failMu.Lock()
	defer w.failMu.Unlock()
	return w.failed
}

// Add citește fișierul și îl pune în pachet. Întoarce eroarea de citire a fișierului (apelantul
// decide dacă continuă), sau eroarea fatală a conductei.
func (w *Writer) Add(ctx context.Context, f File) error {
	if w.closed {
		return errors.New("pack: scriitorul e închis")
	}
	if err := w.Err(); err != nil {
		return err
	}
	if f.Open == nil {
		return errors.New("pack: File.Open lipsește")
	}
	if f.Root == "" || f.Path == "" {
		return fmt.Errorf("pack: intrare fără rădăcină sau cale: %q", f.Orig)
	}

	e := &Entry{
		Root: f.Root, Path: f.Path, Orig: f.Orig,
		Size: f.Size, Mode: f.Mode, ModTime: f.ModTime.UnixNano(),
		Class: f.Class, Secret: f.Secret,
	}
	key := e.Key()
	w.mu.Lock()
	if _, dup := w.keys[key]; dup {
		w.mu.Unlock()
		return fmt.Errorf("pack: intrarea %s/%s există deja", f.Root, f.Path)
	}
	w.mu.Unlock()

	// Deduplicare: doar când există alt fișier de aceeași dimensiune (MaybeDup). Hash-uim
	// întâi; dacă am mai văzut conținutul, intrarea trimite la blocurile existente.
	if f.MaybeDup && f.Size > 0 {
		h, n, err := hashSource(f.Open)
		if err != nil {
			return err
		}
		if orig, seen := w.dedup[h]; seen && orig.Size == n {
			e.SHA256 = h
			e.Size = n
			e.Dup = true
			e.Parts = append([]Part(nil), orig.Parts...)
			w.commitEntry(e)
			w.progress.Dedup += n
			w.report()
			return nil
		}
	}

	if err := w.pack(ctx, f, e); err != nil {
		return err
	}
	w.commitEntry(e)
	if e.Size > 0 {
		w.dedup[e.SHA256] = e
	}
	w.report()
	return nil
}

func (w *Writer) commitEntry(e *Entry) {
	w.mu.Lock()
	w.entries = append(w.entries, e)
	w.keys[e.Key()] = struct{}{}
	for _, p := range e.Parts {
		w.blockEntries[p.Block] = append(w.blockEntries[p.Block], e)
	}
	w.mu.Unlock()
	w.progress.Files++
	w.progress.Bytes += e.Size
}

func (w *Writer) report() {
	if w.o.OnProgress == nil {
		return
	}
	p := w.progress
	p.Stored, p.Volume = w.em.stats()
	w.o.OnProgress(p)
}

// pack citește fișierul o dată, calculând hash-ul și împărțindu-l în blocuri.
func (w *Writer) pack(ctx context.Context, f File, e *Entry) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	hasher := sha256.New()
	var total int64

	// Clasa Unknown se lămurește pe prima mostră.
	kind := KindStored
	compressible := f.Class.Compressible()
	var pending []byte
	if f.Class == scan.Unknown {
		sample := make([]byte, min(int64(sampleSize), max(f.Size, 1)))
		n, rerr := io.ReadFull(rc, sample)
		if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
			return rerr
		}
		pending = sample[:n]
		compressible = looksCompressible(pending)
	}
	if compressible {
		kind = kindFor(w.o.Level)
	}

	chunk := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.Err(); err != nil {
			return err
		}
		var data []byte
		if pending != nil {
			data, pending = pending, nil
		} else {
			n, rerr := rc.Read(chunk)
			if n == 0 && rerr != nil {
				if errors.Is(rerr, io.EOF) {
					break
				}
				return rerr
			}
			data = chunk[:n]
		}
		hasher.Write(data)
		total += int64(len(data))
		if kind == KindStored {
			w.appendStored(e, data)
		} else {
			w.appendSolid(e, kind, data)
		}
	}
	if kind == KindStored {
		w.flushStored(e)
	}

	copy(e.SHA256[:], hasher.Sum(nil))
	e.Size = total
	return nil
}

// ─────────────────────────── blocuri stocate ───────────────────────────
//
// Fișierele incompresibile se copiază ca atare, în blocuri de câte BlockSize; fiecare bloc e o
// parte a fișierului. Bufferul curent e ținut pe scriitor ca să nu alocăm pentru fiecare chunk.

func (w *Writer) appendStored(e *Entry, data []byte) {
	for len(data) > 0 {
		if w.storedBuf == nil {
			w.storedBuf = make([]byte, 0, w.o.BlockSize)
		}
		room := w.o.BlockSize - len(w.storedBuf)
		n := min(room, len(data))
		w.storedBuf = append(w.storedBuf, data[:n]...)
		data = data[n:]
		if len(w.storedBuf) == w.o.BlockSize {
			w.flushStored(e)
		}
	}
}

func (w *Writer) flushStored(e *Entry) {
	if len(w.storedBuf) == 0 {
		return
	}
	id := w.emit(KindStored, w.storedBuf)
	e.Parts = append(e.Parts, Part{Block: id, Offset: 0, Length: int64(len(w.storedBuf))})
	w.storedBuf = nil
}

// ─────────────────────────── blocuri solide ───────────────────────────
//
// Fișierele comprimabile se adună împreună până la BlockSize și se comprimă ca un singur bloc:
// redundanța dintre fișiere mici se exploatează. Un fișier poate începe într-un bloc și continua
// în următorul; atunci are două părți.

func (w *Writer) appendSolid(e *Entry, kind BlockKind, data []byte) {
	sb := w.solid[kind]
	if sb == nil {
		sb = &solidBuf{kind: kind, buf: make([]byte, 0, w.o.BlockSize)}
		w.solid[kind] = sb
	}
	for len(data) > 0 {
		room := w.o.BlockSize - len(sb.buf)
		n := min(room, len(data))
		off := int64(len(sb.buf))
		sb.buf = append(sb.buf, data[:n]...)
		// Continuăm membrul curent dacă e al aceluiași fișier și e lipit de capăt.
		if k := len(sb.members) - 1; k >= 0 && sb.members[k].e == e && sb.members[k].off+sb.members[k].n == off {
			sb.members[k].n += int64(n)
		} else {
			sb.members = append(sb.members, member{e: e, off: off, n: int64(n)})
		}
		data = data[n:]
		if len(sb.buf) == w.o.BlockSize {
			w.flushSolid(sb)
		}
	}
}

func (w *Writer) flushSolid(sb *solidBuf) {
	if len(sb.buf) == 0 {
		return
	}
	id := w.emit(sb.kind, sb.buf)
	for _, m := range sb.members {
		m.e.Parts = append(m.e.Parts, Part{Block: id, Offset: m.off, Length: m.n})
	}
	sb.buf = make([]byte, 0, w.o.BlockSize)
	sb.members = sb.members[:0]
}

// emit înregistrează blocul, îi dă un id și îl trimite la compresie. Datele sunt preluate;
// apelantul nu le mai atinge.
func (w *Writer) emit(kind BlockKind, raw []byte) int {
	var h Hash
	s := sha256.Sum256(raw)
	copy(h[:], s[:])

	w.mu.Lock()
	id := len(w.blocks)
	w.blocks = append(w.blocks, BlockRef{Kind: kind, Raw: int64(len(raw)), SHA256: h})
	w.mu.Unlock()

	w.jobs <- &job{id: id, kind: kind, raw: raw}
	return id
}

func (w *Writer) worker() {
	defer w.workers.Done()
	for j := range w.jobs {
		if w.Err() != nil {
			j.err = w.Err()
		} else {
			j.stored, j.err = compress(j.kind, j.raw)
			if j.kind != KindStored {
				j.raw = nil // memoria brută nu mai e necesară; stocatul e ce scriem
			}
		}
		w.results <- j
	}
}

// Close golește bufferele, așteaptă conducta, încheie ultimul volum și scrie indexul final.
func (w *Writer) Close() error {
	if w.closed {
		return w.Err()
	}
	w.closed = true

	for _, sb := range w.solid {
		w.flushSolid(sb)
	}
	close(w.jobs)
	w.workers.Wait()
	close(w.results)
	<-w.emitted

	if err := w.Err(); err != nil {
		return err
	}

	w.mu.Lock()
	idx := w.snapshotIndex(true)
	w.mu.Unlock()
	if err := w.writeIndexFile(idx); err != nil {
		w.fail(err)
		return err
	}
	if err := w.writeManifest(true); err != nil {
		w.fail(err)
		return err
	}
	if err := w.writeSums(); err != nil {
		w.fail(err)
		return err
	}
	if err := appendJournal(w.o.Dir, JournalLine{Type: "complet", When: w.o.Now().UTC()}); err != nil {
		w.fail(err)
		return err
	}
	w.report()
	return nil
}

// Abort oprește totul și lasă pachetul marcat incomplet. Nu șterge nimic: ce e pe disc poate fi
// reluat sau inspectat.
func (w *Writer) Abort(cause error) {
	if cause == nil {
		cause = errors.New("pack: anulat")
	}
	w.fail(cause)
	if !w.closed {
		w.closed = true
		close(w.jobs)
		w.workers.Wait()
		close(w.results)
		<-w.emitted
	}
}

// snapshotIndex construiește indexul din starea curentă. Cu final=false, intră doar intrările ale
// căror blocuri sunt toate în volume încheiate — exact ce poate fi reluat.
func (w *Writer) snapshotIndex(final bool) *Index {
	idx := &Index{
		Format:   FormatVersion,
		ID:       w.id,
		Complete: final,
		Volumes:  append([]VolumeInfo(nil), w.em.volumes...),
		Blocks:   append([]BlockRef(nil), w.blocks...),
	}
	for _, e := range w.entries {
		if final || e.Placed(w.blocks) {
			idx.Entries = append(idx.Entries, e)
		}
	}
	return idx
}

// writeIndexFile scrie index.dhsi ca un mini-volum cu numărul 0: același antet, un singur bloc de
// tip index, același trailer. Cititorul folosește exact codul de la volume.
func (w *Writer) writeIndexFile(idx *Index) error {
	payload, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	stored, err := compress(KindIndex, payload)
	if err != nil {
		return err
	}
	var buf []byte
	hw := newHashWriter(&sliceWriter{&buf})
	enc, err := passphrase.Encrypt(hw, w.o.Passphrase)
	if err != nil {
		return err
	}
	hdr := volumeHeader{Version: FormatVersion, Volume: 0, ID: w.id, Level: uint8(w.o.Level), Cipher: w.cipher()}
	if _, err := enc.Write(hdr.encode()); err != nil {
		return err
	}
	s := sha256.Sum256(payload)
	var h Hash
	copy(h[:], s[:])
	bh := blockHeader{Kind: KindIndex, Raw: int64(len(payload)), Stored: int64(len(stored)), SHA256: h}
	if _, err := enc.Write(bh.encode()); err != nil {
		return err
	}
	if _, err := enc.Write(stored); err != nil {
		return err
	}
	pos := int64(volumeHeaderSize)
	tr := trailer{Blocks: 0, IndexPos: pos, StreamLen: pos + blockHeaderSize + int64(len(stored))}
	if _, err := enc.Write(tr.encode()); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(w.o.Dir, indexName), buf, 0o644)
}

type sliceWriter struct{ b *[]byte }

func (s *sliceWriter) Write(p []byte) (int, error) {
	*s.b = append(*s.b, p...)
	return len(p), nil
}

func (w *Writer) cipher() Cipher {
	if w.o.Passphrase == "" {
		return CipherNone
	}
	return CipherAgeScrypt
}

func (w *Writer) writeManifest(complete bool) error {
	w.mu.Lock()
	m := Manifest{
		Format:   FormatVersion,
		ID:       w.id,
		Created:  w.created,
		Source:   sourceFrom(w.o.Source),
		Cipher:   w.cipher().String(),
		Level:    w.o.Level,
		Volumes:  len(w.em.volumesSnapshot()),
		Files:    int64(len(w.entries)),
		Raw:      w.progress.Bytes,
		Complete: complete,
		Tool:     w.o.Tool,
	}
	for _, e := range w.entries {
		if e.Secret {
			m.Secrets = true
			break
		}
	}
	w.mu.Unlock()
	if w.em != nil {
		m.Stored, _ = w.em.stats()
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(w.o.Dir, manifestName), append(b, '\n'), 0o644)
}

func (w *Writer) writeSums() error {
	sums := Sums{}
	for _, name := range []string{manifestName, indexName} {
		h, _, err := hashFile(filepath.Join(w.o.Dir, name))
		if err != nil {
			return err
		}
		sums[name] = h
	}
	for _, v := range w.em.volumesSnapshot() {
		sums[filepath.ToSlash(filepath.Join(volumeDir, volumeName(v.Number)))] = v.SHA256
	}
	return writeSums(w.o.Dir, sums)
}

// hashSource citește complet o sursă doar pentru hash — pasul întâi al deduplicării.
func hashSource(open func() (io.ReadCloser, error)) (Hash, int64, error) {
	var h Hash
	rc, err := open()
	if err != nil {
		return h, 0, err
	}
	defer rc.Close()
	hs := sha256.New()
	n, err := io.Copy(hs, rc)
	if err != nil {
		return h, 0, err
	}
	copy(h[:], hs.Sum(nil))
	return h, n, nil
}
