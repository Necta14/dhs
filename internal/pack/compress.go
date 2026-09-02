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

// kindFor leagă nivelul ales de utilizator de codificarea blocurilor solide.
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

// Encoderele zstd sunt scumpe de construit; le refolosim.
var (
	zstdBest  *zstd.Encoder
	zstdFast  *zstd.Encoder
	zstdDec   *zstd.Decoder
	zstdOnce  sync.Once
	zstdError error
)

func initZstd() {
	zstdOnce.Do(func() {
		// Concurrency 1: paralelizăm noi, pe blocuri, nu în interiorul unui bloc.
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

// compress codifică un bloc. Pentru KindStored întoarce datele neschimbate.
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
		return nil, fmt.Errorf("pack: nu știu să codific blocuri de tip %s", kind)
	}
}

// decompress decodifică un bloc. rawSize e ce spune antetul; refuzăm să depășim.
func decompress(kind BlockKind, stored []byte, rawSize int64) ([]byte, error) {
	if rawSize < 0 || rawSize > MaxBlockSize {
		return nil, fmt.Errorf("%w: bloc de %d octeți", ErrCorrupt, rawSize)
	}
	switch kind {
	case KindStored:
		if int64(len(stored)) != rawSize {
			return nil, fmt.Errorf("%w: bloc stocat cu %d octeți în loc de %d", ErrCorrupt, len(stored), rawSize)
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
		return nil, fmt.Errorf("%w: bloc de tip necunoscut %d", ErrNewerFormat, uint8(kind))
	}
}

func readAllLimited(r io.Reader, rawSize int64) ([]byte, error) {
	out := make([]byte, rawSize)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, fmt.Errorf("%w: decompresie: %v", ErrCorrupt, err)
	}
	// Dacă mai urmează ceva, dimensiunea din antet a mințit.
	var extra [1]byte
	if n, _ := r.Read(extra[:]); n != 0 {
		return nil, fmt.Errorf("%w: blocul decomprimat e mai mare decât spune antetul", ErrCorrupt)
	}
	return out, nil
}

func checkLen(out []byte, rawSize int64) ([]byte, error) {
	if int64(len(out)) != rawSize {
		return nil, fmt.Errorf("%w: blocul decomprimat are %d octeți în loc de %d", ErrCorrupt, len(out), rawSize)
	}
	return out, nil
}

// looksCompressible face testul de entropie pentru clasa Unknown: comprimă rapid o mostră și se
// uită dacă a scăzut măcar cu o zecime. Ieftin, și corect în marea majoritate a cazurilor.
func looksCompressible(sample []byte) bool {
	if len(sample) < 512 {
		return true // prea mic ca să conteze; îl punem la comprimabile, costă nimic
	}
	initZstd()
	if zstdError != nil {
		return false
	}
	out := zstdFast.EncodeAll(sample, make([]byte, 0, len(sample)))
	return len(out) < len(sample)*9/10
}

// sampleSize e cât citim din un fișier necunoscut pentru testul de entropie.
const sampleSize = 256 << 10
