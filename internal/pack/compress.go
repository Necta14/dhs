package pack

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/Necta14/dhs/internal/scan"
)

// kindFor ties the level chosen by the user to the encoding of solid blocks.
func kindFor(level scan.Level) BlockKind {
	switch level {
	case scan.LevelCompatible:
		return KindDeflate
	case scan.LevelMax:
		return KindXZ
	default:
		return KindZstd
	}
}

// zstd encoders are expensive to build; we reuse them.
var (
	zstdBest  *zstd.Encoder
	zstdFast  *zstd.Encoder
	zstdDec   *zstd.Decoder
	zstdOnce  sync.Once
	zstdError error
)

func initZstd() {
	zstdOnce.Do(func() {
		// Concurrency 1: we parallelize ourselves, across blocks, not inside a block.
		zstdBest, zstdError = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedBestCompression),
			zstd.WithEncoderConcurrency(1),
			zstd.WithWindowSize(DefaultBlockSize),
			zstd.WithZeroFrames(true),
		)
		if zstdError != nil {
			return
		}
		zstdFast, zstdError = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedFastest),
			zstd.WithEncoderConcurrency(1),
		)
		if zstdError != nil {
			return
		}
		zstdDec, zstdError = zstd.NewReader(nil,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxMemory(MaxBlockSize),
		)
	})
}

// compress encodes a block. For KindStored it returns the data unchanged.
func compress(kind BlockKind, raw []byte) ([]byte, error) {
	switch kind {
	case KindStored:
		return raw, nil
	case KindZstd, KindIndex:
		initZstd()
		if zstdError != nil {
			return nil, zstdError
		}
		return zstdBest.EncodeAll(raw, make([]byte, 0, len(raw)/2)), nil
	case KindDeflate:
		var buf bytes.Buffer
		buf.Grow(len(raw) / 2)
		w, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(raw); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case KindXZ:
		var buf bytes.Buffer
		buf.Grow(len(raw) / 2)
		cfg := xz.WriterConfig{DictCap: DefaultBlockSize}
		w, err := cfg.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(raw); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("pack: don't know how to encode blocks of kind %s", kind)
	}
}

// decompress decodes a block. rawSize is what the header says; we refuse to exceed it.
func decompress(kind BlockKind, stored []byte, rawSize int64) ([]byte, error) {
	if rawSize < 0 || rawSize > MaxBlockSize {
		return nil, fmt.Errorf("%w: block of %d bytes", ErrCorrupt, rawSize)
	}
	switch kind {
	case KindStored:
		if int64(len(stored)) != rawSize {
			return nil, fmt.Errorf("%w: stored block has %d bytes instead of %d", ErrCorrupt, len(stored), rawSize)
		}
		return stored, nil
	case KindZstd, KindIndex:
		initZstd()
		if zstdError != nil {
			return nil, zstdError
		}
		out, err := zstdDec.DecodeAll(stored, make([]byte, 0, rawSize))
		if err != nil {
			return nil, fmt.Errorf("%w: zstd: %v", ErrCorrupt, err)
		}
		return checkLen(out, rawSize)
	case KindDeflate:
		return readAllLimited(flate.NewReader(bytes.NewReader(stored)), rawSize)
	case KindXZ:
		r, err := xz.NewReader(bytes.NewReader(stored))
		if err != nil {
			return nil, fmt.Errorf("%w: xz: %v", ErrCorrupt, err)
		}
		return readAllLimited(r, rawSize)
	default:
		return nil, fmt.Errorf("%w: unknown block kind %d", ErrNewerFormat, uint8(kind))
	}
}

func readAllLimited(r io.Reader, rawSize int64) ([]byte, error) {
	out := make([]byte, rawSize)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, fmt.Errorf("%w: decompression: %v", ErrCorrupt, err)
	}
	// If anything else follows, the size in the header lied.
	var extra [1]byte
	if n, _ := r.Read(extra[:]); n != 0 {
		return nil, fmt.Errorf("%w: decompressed block is larger than the header says", ErrCorrupt)
	}
	return out, nil
}

func checkLen(out []byte, rawSize int64) ([]byte, error) {
	if int64(len(out)) != rawSize {
		return nil, fmt.Errorf("%w: decompressed block has %d bytes instead of %d", ErrCorrupt, len(out), rawSize)
	}
	return out, nil
}

// looksCompressible is the entropy test for the Unknown class: it quickly compresses a sample and
// checks whether it shrank by at least a tenth. Cheap, and right in the vast majority of cases.
func looksCompressible(sample []byte) bool {
	if len(sample) < 512 {
		return true // too small to matter; we count it as compressible, it costs nothing
	}
	initZstd()
	if zstdError != nil {
		return false
	}
	out := zstdFast.EncodeAll(sample, make([]byte, 0, len(sample)))
	return len(out) < len(sample)*9/10
}

// sampleSize is how much we read from an unknown file for the entropy test.
const sampleSize = 256 << 10
