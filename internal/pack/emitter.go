package pack

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/necta/dhs/internal/passphrase"
)

// emitter e singurul care scrie pe disc. Primește blocurile comprimate în orice ordine, le
// reordonează după id și le scrie secvențial, rotind volumele când se umplu. Rulează într-un
// singur goroutine, deci nu are nevoie de lacăte pentru starea proprie.
type emitter struct {
	w *Writer

	mu      sync.Mutex // volumes, stored, current — citite din alt goroutine la raportare
	volumes []VolumeInfo
	stored  int64
	current uint32

	pending map[int]*job
	next    int
	vol     *volume
}

// volume e volumul în curs de scriere.
type volume struct {
	number uint32
	path   string
	tmp    string
	f      *os.File
	hw     *hashWriter    // ce ajunge pe disc: criptat
	enc    io.WriteCloser // fluxul în clar intră aici
	pos    int64          // offset în fluxul în clar
	blocks []int          // id-urile blocurilor din volum, în ordine
	touch  map[*Entry]struct{}
}

func newEmitter(w *Writer) *emitter {
	return &emitter{w: w, pending: make(map[int]*job), current: 1}
}

func (em *emitter) stats() (stored int64, current uint32) {
	em.mu.Lock()
	defer em.mu.Unlock()
	return em.stored, em.current
}

func (em *emitter) volumesSnapshot() []VolumeInfo {
	em.mu.Lock()
	defer em.mu.Unlock()
	return append([]VolumeInfo(nil), em.volumes...)
}

func (em *emitter) run() {
	defer close(em.w.emitted)
	for j := range em.w.results {
		if j.err != nil {
			em.w.fail(j.err)
		}
		em.pending[j.id] = j
		for {
			ready, ok := em.pending[em.next]
			if !ok {
				break
			}
			delete(em.pending, em.next)
			em.next++
			if em.w.Err() == nil {
				if err := em.write(ready); err != nil {
					em.w.fail(err)
				}
			}
			ready.raw, ready.stored = nil, nil
		}
	}
	// Coada s-a închis: fie Close, fie Abort. Încheiem volumul curent doar dacă totul a mers.
	if em.vol != nil {
		if em.w.Err() == nil {
			if err := em.finish(); err != nil {
				em.w.fail(err)
			}
		} else {
			em.discard()
		}
	}
}

// write pune un bloc în volumul curent, sau deschide unul nou dacă nu mai încape.
func (em *emitter) write(j *job) error {
	stored := j.stored
	if j.kind == KindStored {
		stored = j.raw
	}
	need := int64(blockHeaderSize) + int64(len(stored))

	if em.vol != nil && !em.fits(need) {
		if err := em.finish(); err != nil {
			return err
		}
	}
	if em.vol == nil {
		if err := em.open(); err != nil {
			return err
		}
	}

	var h Hash
	em.w.mu.Lock()
	ref := em.w.blocks[j.id]
	touching := em.w.blockEntries[j.id]
	em.w.mu.Unlock()
	h = ref.SHA256

	bh := blockHeader{Kind: j.kind, Raw: int64(len(j.raw)), Stored: int64(len(stored)), SHA256: h}
	if j.kind != KindStored {
		bh.Raw = ref.Raw
	}
	pos := em.vol.pos
	if err := em.vol.write(bh.encode()); err != nil {
		return err
	}
	if err := em.vol.write(stored); err != nil {
		return err
	}

	em.w.mu.Lock()
	em.w.blocks[j.id] = BlockRef{Volume: em.vol.number, Pos: pos, Kind: j.kind, Raw: bh.Raw, Stored: bh.Stored, SHA256: h}
	em.w.mu.Unlock()
	em.vol.blocks = append(em.vol.blocks, j.id)
	for _, e := range touching {
		em.vol.touch[e] = struct{}{}
	}

	em.mu.Lock()
	em.stored = em.storedSoFar()
	em.mu.Unlock()
	return nil
}

func (em *emitter) storedSoFar() int64 {
	var n int64
	for _, v := range em.volumes {
		n += v.Bytes
	}
	if em.vol != nil {
		n += em.vol.hw.n
	}
	return n
}

// fits spune dacă mai încape un bloc, lăsând loc pentru indexul volumului, trailer și pentru
// overhead-ul cifrului: age adaugă 16 octeți la fiecare 64 KiB, adică ~900 KB pe un volum de
// 3,5 GiB. Rezerva de 4 MiB acoperă asta cu marjă și ține volumul sub limita FAT32 de 4 GiB.
// Un volum gol primește orice bloc: altfel un bloc mare ar roti volumele la infinit.
func (em *emitter) fits(need int64) bool {
	if len(em.vol.blocks) == 0 {
		return true
	}
	reserve := int64(len(em.vol.touch))*320 + int64(len(em.vol.blocks))*128 + 4<<20
	return em.vol.pos+need+reserve <= em.w.o.VolumeSize
}

func (em *emitter) open() error {
	n := em.current
	path := filepath.Join(em.w.o.Dir, volumeDir, volumeName(n))
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	hw := newHashWriter(f)
	enc, err := passphrase.Encrypt(hw, em.w.o.Passphrase)
	if err != nil {
		f.Close()
		return err
	}
	v := &volume{number: n, path: path, tmp: tmp, f: f, hw: hw, enc: enc, touch: make(map[*Entry]struct{})}
	hdr := volumeHeader{Version: FormatVersion, Volume: n, ID: em.w.id, Level: uint8(em.w.o.Level), Cipher: em.w.cipher()}
	if err := v.write(hdr.encode()); err != nil {
		f.Close()
		return err
	}
	em.vol = v
	return nil
}

func (v *volume) write(b []byte) error {
	n, err := v.enc.Write(b)
	v.pos += int64(n)
	return err
}

// finish scrie indexul volumului și trailerul, închide cifrul, forțează pe disc, redenumește,
// apoi actualizează index.dhsi, manifestul, sumele și jurnalul — în ordinea asta, ca la orice
// întrerupere pachetul să fie fie „volumul N încheiat", fie „volumul N nu există".
func (em *emitter) finish() error {
	v := em.vol
	em.w.mu.Lock()
	idx := &Index{Format: FormatVersion, ID: em.w.id, Complete: false}
	for _, id := range v.blocks {
		idx.Blocks = append(idx.Blocks, em.w.blocks[id])
	}
	for e := range v.touch {
		idx.Entries = append(idx.Entries, e)
	}
	em.w.mu.Unlock()

	payload, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	stored, err := compress(KindIndex, payload)
	if err != nil {
		return err
	}
	s := sha256.Sum256(payload)
	var h Hash
	copy(h[:], s[:])
	indexPos := v.pos
	bh := blockHeader{Kind: KindIndex, Raw: int64(len(payload)), Stored: int64(len(stored)), SHA256: h}
	if err := v.write(bh.encode()); err != nil {
		return err
	}
	if err := v.write(stored); err != nil {
		return err
	}
	tr := trailer{Blocks: uint32(len(v.blocks)), IndexPos: indexPos, StreamLen: v.pos}
	if err := v.write(tr.encode()); err != nil {
		return err
	}
	if err := v.enc.Close(); err != nil {
		return err
	}
	if err := v.f.Sync(); err != nil {
		v.f.Close()
		return err
	}
	if err := v.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(v.tmp, v.path); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(v.path)); err != nil {
		return err
	}

	info := VolumeInfo{Number: v.number, Bytes: v.hw.n, Blocks: len(v.blocks), SHA256: v.hw.Sum()}
	em.mu.Lock()
	em.volumes = append(em.volumes, info)
	em.current = v.number + 1
	em.stored = em.storedSoFar()
	em.mu.Unlock()
	em.vol = nil

	// Starea reluabilă: indexul cu ce e complet, manifestul, sumele, apoi jurnalul.
	em.w.mu.Lock()
	partial := em.w.snapshotIndex(false)
	em.w.mu.Unlock()
	if err := em.w.writeIndexFile(partial); err != nil {
		return err
	}
	if err := em.w.writeManifest(false); err != nil {
		return err
	}
	if err := em.w.writeSums(); err != nil {
		return err
	}
	sha := info.SHA256
	if err := appendJournal(em.w.o.Dir, JournalLine{Type: "volum", Volume: v.number, Bytes: v.hw.n, SHA256: &sha, When: em.w.o.Now().UTC()}); err != nil {
		return err
	}
	if em.w.o.OnProgress != nil {
		em.w.report()
	}
	return nil
}

// discard aruncă volumul în curs după o eroare. Fișierul temporar rămâne cu sufixul .tmp — vizibil
// că e incomplet, și niciodată confundabil cu un volum valid.
func (em *emitter) discard() {
	if em.vol == nil {
		return
	}
	_ = em.vol.f.Close()
	em.vol = nil
}

// volumeFile întoarce calea unui volum din pachet.
func volumeFile(dir string, n uint32) string {
	return filepath.Join(dir, volumeDir, volumeName(n))
}
