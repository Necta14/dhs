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

	"github.com/Necta14/dhs/internal/passphrase"
	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

// Options configures the writing of a package.
type Options struct {
	// Dir is the package directory; it gets created. It must be empty or not exist yet.
	Dir        string
	Level      scan.Level
	Passphrase string // empty = no encryption, an explicit choice by the user
	Source     system.Info
	VolumeSize int64 // default DefaultVolumeSize
	BlockSize  int   // default DefaultBlockSize
	Workers    int   // default runtime.NumCPU()
	Tool       string
	Now        func() time.Time
	OnProgress func(Progress)
}

// Progress is the state of the write, reported after every file and after every volume.
type Progress struct {
	Files  int64  // files added
	Bytes  int64  // raw bytes read
	Stored int64  // bytes written to disk, encrypted
	Dedup  int64  // bytes saved through deduplication
	Volume uint32 // volume in progress
}

// File is what the writer needs to know about a file that is to be added.
type File struct {
	Root    Root
	Path    string // relative to Root, with "/"
	Orig    string // original absolute path
	Size    int64
	Mode    uint32
	ModTime time.Time
	Class   scan.Class
	Secret  bool
	// MaybeDup asks for the deduplication check: the file is read once for the hash and, if it is
	// not a duplicate, once more for packing. The caller sets it only when another entry has the
	// same size — the remaining files cannot possibly be duplicates.
	MaybeDup bool
	Open     func() (io.ReadCloser, error)
}

// Writer writes a package. It is not safe for concurrent calls to Add; compression and writing to
// disk, however, run in parallel behind it.
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
	// dups maps an original entry to the deduplicated entries that share its blocks. They must
	// receive every part the original receives later, when its solid block is finally written.
	dups      map[*Entry][]*Entry
	solid     map[BlockKind]*solidBuf
	storedBuf []byte // the stored block currently being filled

	jobs    chan *job
	results chan *job
	workers sync.WaitGroup
	emitted chan struct{} // closed when the emitter has finished
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

// NewWriter creates the package directory, writes the provisional manifest and starts the pipeline.
func NewWriter(o Options) (*Writer, error) {
	if !o.Level.Valid() {
		return nil, fmt.Errorf("pack: invalid compression level %d", o.Level)
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
		return nil, fmt.Errorf("pack: block size %d exceeds the maximum of %d", o.BlockSize, MaxBlockSize)
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
		dups:         make(map[*Entry][]*Entry),
		solid:        make(map[BlockKind]*solidBuf),
		jobs:         make(chan *job, o.Workers*2),
		results:      make(chan *job, o.Workers*2),
		emitted:      make(chan struct{}),
	}
	w.progress.Volume = 1

	// The emitter exists before the first manifest: the manifest asks it how many volumes there are.
	w.em = newEmitter(w)
	if err := w.writeManifest(false); err != nil {
		return nil, err
	}

	for i := 0; i < o.Workers; i++ {
		w.workers.Add(1)
		go w.worker()
	}
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
		return fmt.Errorf("pack: directory %s is not empty; refusing to overwrite anything", dir)
	}
	return nil
}

// ID is the identifier of the package in progress.
func (w *Writer) ID() ID { return w.id }

// Dir is the package directory.
func (w *Writer) Dir() string { return w.o.Dir }

// fail records the first error; every later operation returns it.
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

// Err is the first error that occurred in the pipeline, if any.
func (w *Writer) Err() error {
	w.failMu.Lock()
	defer w.failMu.Unlock()
	return w.failed
}

// Add reads the file and puts it in the package. It returns the file's read error (the caller
// decides whether to go on), or the pipeline's fatal error.
func (w *Writer) Add(ctx context.Context, f File) error {
	if w.closed {
		return errors.New("pack: writer is closed")
	}
	if err := w.Err(); err != nil {
		return err
	}
	if f.Open == nil {
		return errors.New("pack: File.Open is missing")
	}
	if f.Root == "" || f.Path == "" {
		return fmt.Errorf("pack: entry without root or path: %q", f.Orig)
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
		return fmt.Errorf("pack: entry %s/%s already exists", f.Root, f.Path)
	}
	w.mu.Unlock()

	// Deduplication: only when another file of the same size exists (MaybeDup). We hash first; if
	// we have seen this content before, the entry points at the existing blocks.
	if f.MaybeDup && f.Size > 0 {
		h, n, err := hashSource(f.Open)
		if err != nil {
			return err
		}
		if orig, seen := w.dedup[h]; seen && orig.Size == n {
			e.SHA256 = h
			e.Size = n
			e.Dup = true
			// The original may still be waiting for the tail of an open solid block. Copying its
			// parts here is not enough: whatever arrives afterwards would reach the original only,
			// and the duplicate would be written with no parts at all — a file the reader then
			// rejects as "empty file with a nonzero hash". So, under one lock, take the parts it
			// has and subscribe to the ones it has yet to get.
			w.mu.Lock()
			e.Parts = append([]Part(nil), orig.Parts...)
			w.dups[orig] = append(w.dups[orig], e)
			w.mu.Unlock()
			w.commitEntry(e)
			w.mu.Lock()
			w.progress.Dedup += n
			w.mu.Unlock()
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

// commitEntry makes the entry visible to the emitter. From here on, any change to it goes through
// addPart, under the lock — the emitter may serialize it at any moment.
func (w *Writer) commitEntry(e *Entry) {
	w.mu.Lock()
	w.entries = append(w.entries, e)
	w.keys[e.Key()] = struct{}{}
	for _, p := range e.Parts {
		w.blockEntries[p.Block] = append(w.blockEntries[p.Block], e)
	}
	// Also under the lock: the manifest reads progress.Bytes from the emitter's goroutine.
	w.progress.Files++
	w.progress.Bytes += e.Size
	w.mu.Unlock()
}

// addPart adds a part to an entry. Under the lock, because the entry may already be registered:
// a solid block holds the tail of a finished file until it fills up with the following ones, and
// the emitter may serialize the entry exactly then.
func (w *Writer) addPart(e *Entry, p Part) {
	w.mu.Lock()
	w.addPartLocked(e, p)
	// Deduplicated copies point at the same bytes, so they get the same part.
	for _, d := range w.dups[e] {
		w.addPartLocked(d, p)
	}
	w.mu.Unlock()
}

// addPartLocked appends one part and, if the entry is already visible to the emitter, makes sure
// the block's index lists it. The caller holds w.mu.
func (w *Writer) addPartLocked(e *Entry, p Part) {
	e.Parts = append(e.Parts, p)
	if _, committed := w.keys[e.Key()]; committed {
		w.blockEntries[p.Block] = append(w.blockEntries[p.Block], e)
	}
}

func (w *Writer) report() {
	if w.o.OnProgress == nil {
		return
	}
	w.mu.Lock()
	p := w.progress
	w.mu.Unlock()
	p.Stored, p.Volume = w.em.stats()
	w.o.OnProgress(p)
}

// pack reads the file once, computing the hash and splitting it into blocks.
func (w *Writer) pack(ctx context.Context, f File, e *Entry) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	hasher := sha256.New()
	var total int64

	// The Unknown class gets settled on the first sample.
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

	// The entry is not registered yet, so nobody reads it; the lock is cheap anyway and keeps the
	// invariant simple: every field the emitter serializes is written under the lock.
	w.mu.Lock()
	copy(e.SHA256[:], hasher.Sum(nil))
	e.Size = total
	w.mu.Unlock()
	return nil
}

// ─────────────────────────── stored blocks ───────────────────────────
//
// Incompressible files are copied as they are, in blocks of BlockSize each; every block is one
// part of the file. The current buffer is kept on the writer so that we don't allocate for every
// chunk.

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
	w.addPart(e, Part{Block: id, Offset: 0, Length: int64(len(w.storedBuf))})
	w.storedBuf = nil
}

// ─────────────────────────── solid blocks ───────────────────────────
//
// Compressible files are gathered together up to BlockSize and compressed as a single block: the
// redundancy between small files gets exploited. A file may start in one block and continue in
// the next one; it then has two parts.

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
		// Extend the current member if it belongs to the same file and is glued to the end.
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
		w.addPart(m.e, Part{Block: id, Offset: m.off, Length: m.n})
	}
	sb.buf = make([]byte, 0, w.o.BlockSize)
	sb.members = sb.members[:0]
}

// emit registers the block, gives it an id and sends it off to compression. The data is taken
// over; the caller must not touch it again.
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
				j.raw = nil // the raw memory is no longer needed; the stored form is what we write
			}
		}
		w.results <- j
	}
}

// Close flushes the buffers, waits for the pipeline, finishes the last volume and writes the
// final index.
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
	if err := appendJournal(w.o.Dir, JournalLine{Type: "complete", When: w.o.Now().UTC()}); err != nil {
		w.fail(err)
		return err
	}
	w.report()
	return nil
}

// Abort stops everything and leaves the package marked incomplete. It deletes nothing: what is
// on disk can be resumed or inspected.
func (w *Writer) Abort(cause error) {
	if cause == nil {
		cause = errors.New("pack: aborted")
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

// snapshotIndex builds the index from the current state, with COPIES of the entries: the caller
// serializes it after releasing the lock, and meanwhile Add may add parts. Called under w.mu.
// With final=false, only the entries whose blocks are all in finished volumes go in — exactly
// what can be resumed.
func (w *Writer) snapshotIndex(final bool) *Index {
	idx := &Index{
		Format:   FormatVersion,
		ID:       w.id,
		Complete: final,
		Volumes:  w.em.volumesSnapshot(),
		Blocks:   append([]BlockRef(nil), w.blocks...),
	}
	for _, e := range w.entries {
		if final || e.Placed(w.blocks) {
			idx.Entries = append(idx.Entries, e.clone())
		}
	}
	return idx
}

// writeIndexFile writes index.dhsi as a mini-volume numbered 0: the same header, a single block of
// kind index, the same trailer. The reader uses exactly the code it uses for volumes.
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
	m.Stored, _ = w.em.stats()
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

// hashSource reads a source in full just for the hash — the first step of deduplication.
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
