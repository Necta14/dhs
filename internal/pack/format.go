// Package pack defines the migration package format and writes / reads it.
//
// The package is a directory:
//
//	name.dhs/
//	  dhs.json        root manifest, UNENCRYPTED and without file names — only what the package is
//	  index.dhsi      the complete index (files, blocks, placement), ENCRYPTED; small, read first
//	  volumes/
//	    0001.dhsv     data: header, blocks, own index, trailer — ENCRYPTED, ≤ 3.5 GiB
//	    0002.dhsv
//	  SHA256SUMS      SHA-256 of every file in the package
//	  journal.jsonl   one line per finished volume, for resuming after an interruption
//
// Every volume is a stream written once, sequentially, through the age cipher. Inside the stream
// sit blocks: either "stored" (already-compressed files, copied as they are) or "solid" (many
// compressible files packed together, then compressed). At the end of every volume sits an index
// of that volume — redundancy for the case where index.dhsi gets lost — and a fixed trailer.
//
// All numbers are little-endian. All paths in the index use "/" regardless of the system.
package pack

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// FormatVersion is bumped only on incompatible changes to the binary format or to the index.
const FormatVersion uint16 = 1

const (
	volumeMagic  = "DHSV"
	blockMagic   = "DHSB"
	trailerMagic = "DHST"

	// DefaultVolumeSize is 3.5 GiB: below the FAT32 limit of 4 GiB, uniform on any medium.
	DefaultVolumeSize int64 = 3584 << 20
	// DefaultBlockSize is how much we gather into a solid block before compressing it. Larger =
	// slightly better compression, but more memory: with N workers we have ~2N blocks in flight.
	DefaultBlockSize = 32 << 20
	// MaxBlockSize is the hard limit the reader accepts, so that a broken or hostile package cannot
	// make us allocate gigabytes.
	MaxBlockSize = 256 << 20

	// volumeHeaderSize, blockHeaderSize and trailerSize are fixed; the reader relies on that.
	volumeHeaderSize = 32
	blockHeaderSize  = 56
	trailerSize      = 32
)

// Cipher says how a volume's stream is protected.
type Cipher uint8

const (
	CipherNone      Cipher = 0
	CipherAgeScrypt Cipher = 1
)

func (c Cipher) String() string {
	switch c {
	case CipherNone:
		return "none"
	case CipherAgeScrypt:
		return "age-scrypt"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(c))
	}
}

// BlockKind says how the content of a block is encoded.
type BlockKind uint8

const (
	KindStored  BlockKind = 0
	KindDeflate BlockKind = 1
	KindZstd    BlockKind = 2
	KindXZ      BlockKind = 3
	// KindIndex marks the index at the end of the volume. The content is JSON compressed with zstd.
	KindIndex BlockKind = 255
)

func (k BlockKind) String() string {
	switch k {
	case KindStored:
		return "stored"
	case KindDeflate:
		return "deflate"
	case KindZstd:
		return "zstd"
	case KindXZ:
		return "xz"
	case KindIndex:
		return "index"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(k))
	}
}

// ID identifies a package. Every volume carries it, so that volumes of different packages never get mixed up.
type ID [16]byte

func NewID() (ID, error) {
	var id ID
	if _, err := rand.Read(id[:]); err != nil {
		return id, fmt.Errorf("pack: cannot generate id: %w", err)
	}
	return id, nil
}

func (id ID) String() string { return hex.EncodeToString(id[:]) }

// ParseID reads the hex form.
func ParseID(s string) (ID, error) {
	var id ID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(id) {
		return id, fmt.Errorf("pack: invalid id %q", s)
	}
	copy(id[:], b)
	return id, nil
}

// MarshalText and UnmarshalText make the ID appear in JSON as a hex string.
func (id ID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }
func (id *ID) UnmarshalText(b []byte) error {
	parsed, err := ParseID(string(b))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// Hash is a SHA-256, serialized as hex in JSON.
type Hash [32]byte

func (h Hash) String() string               { return hex.EncodeToString(h[:]) }
func (h Hash) MarshalText() ([]byte, error) { return []byte(h.String()), nil }
func (h *Hash) UnmarshalText(b []byte) error {
	d, err := hex.DecodeString(string(b))
	if err != nil || len(d) != len(h) {
		return fmt.Errorf("pack: invalid hash %q", string(b))
	}
	copy(h[:], d)
	return nil
}

// volumeHeader opens every volume: 32 bytes.
//
//	0   magic "DHSV"      4
//	4   format version    2
//	6   volume number     4
//	10  package id        16
//	26  compression level 1
//	27  cipher            1
//	28  reserved          4
type volumeHeader struct {
	Version uint16
	Volume  uint32
	ID      ID
	Level   uint8
	Cipher  Cipher
}

func (h volumeHeader) encode() []byte {
	b := make([]byte, volumeHeaderSize)
	copy(b[0:4], volumeMagic)
	binary.LittleEndian.PutUint16(b[4:6], h.Version)
	binary.LittleEndian.PutUint32(b[6:10], h.Volume)
	copy(b[10:26], h.ID[:])
	b[26] = h.Level
	b[27] = uint8(h.Cipher)
	return b
}

func decodeVolumeHeader(b []byte) (volumeHeader, error) {
	var h volumeHeader
	if len(b) < volumeHeaderSize || string(b[0:4]) != volumeMagic {
		return h, ErrNotVolume
	}
	h.Version = binary.LittleEndian.Uint16(b[4:6])
	if h.Version > FormatVersion {
		return h, fmt.Errorf("%w: volume is in format %d, this build understands up to %d", ErrNewerFormat, h.Version, FormatVersion)
	}
	h.Volume = binary.LittleEndian.Uint32(b[6:10])
	copy(h.ID[:], b[10:26])
	h.Level = b[26]
	h.Cipher = Cipher(b[27])
	return h, nil
}

// blockHeader precedes every block: 56 bytes.
//
//	0   magic "DHSB"      4
//	4   kind              1
//	5   flags             1
//	6   reserved          2
//	8   raw size          8   (before compression)
//	16  stored size       8   (how much follows in the stream)
//	24  raw sha256        32
type blockHeader struct {
	Kind   BlockKind
	Raw    int64
	Stored int64
	SHA256 Hash
}

func (h blockHeader) encode() []byte {
	b := make([]byte, blockHeaderSize)
	copy(b[0:4], blockMagic)
	b[4] = uint8(h.Kind)
	binary.LittleEndian.PutUint64(b[8:16], uint64(h.Raw))
	binary.LittleEndian.PutUint64(b[16:24], uint64(h.Stored))
	copy(b[24:56], h.SHA256[:])
	return b
}

func decodeBlockHeader(b []byte) (blockHeader, error) {
	var h blockHeader
	if len(b) < blockHeaderSize || string(b[0:4]) != blockMagic {
		return h, ErrCorrupt
	}
	h.Kind = BlockKind(b[4])
	h.Raw = int64(binary.LittleEndian.Uint64(b[8:16]))
	h.Stored = int64(binary.LittleEndian.Uint64(b[16:24]))
	copy(h.SHA256[:], b[24:56])
	if h.Raw < 0 || h.Stored < 0 || h.Raw > MaxBlockSize || h.Stored > MaxBlockSize+1<<20 {
		return h, fmt.Errorf("%w: block with impossible sizes (%d / %d)", ErrCorrupt, h.Raw, h.Stored)
	}
	return h, nil
}

// trailer closes every volume: 32 bytes.
//
//	0   magic "DHST"        4
//	4   block count         4   (without the index block)
//	8   index block pos     8   (offset in the decrypted stream)
//	16  volume size         8   (bytes of decrypted stream, without the trailer)
//	24  reserved            8
type trailer struct {
	Blocks    uint32
	IndexPos  int64
	StreamLen int64
}

func (t trailer) encode() []byte {
	b := make([]byte, trailerSize)
	copy(b[0:4], trailerMagic)
	binary.LittleEndian.PutUint32(b[4:8], t.Blocks)
	binary.LittleEndian.PutUint64(b[8:16], uint64(t.IndexPos))
	binary.LittleEndian.PutUint64(b[16:24], uint64(t.StreamLen))
	return b
}

func decodeTrailer(b []byte) (trailer, error) {
	var t trailer
	if len(b) < trailerSize || string(b[0:4]) != trailerMagic {
		return t, fmt.Errorf("%w: trailer missing or damaged", ErrCorrupt)
	}
	t.Blocks = binary.LittleEndian.Uint32(b[4:8])
	t.IndexPos = int64(binary.LittleEndian.Uint64(b[8:16]))
	t.StreamLen = int64(binary.LittleEndian.Uint64(b[16:24]))
	return t, nil
}

// Errors that callers may run into.
var (
	ErrNotVolume      = errors.New("pack: not a DHS volume")
	ErrNotPackage     = errors.New("pack: not a DHS package")
	ErrNewerFormat    = errors.New("pack: package requires a newer DHS")
	ErrCorrupt        = errors.New("pack: corrupt data")
	ErrNeedPassphrase = errors.New("pack: package is encrypted; passphrase required")
	ErrBadPassphrase  = errors.New("pack: wrong passphrase")
	ErrIncomplete     = errors.New("pack: package is incomplete")
	ErrWrongPackage   = errors.New("pack: volume belongs to another package")
)

// readFull reads exactly n bytes or returns an error — no silent partial reads.
func readFull(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: truncated stream", ErrCorrupt)
		}
		return nil, err
	}
	return b, nil
}
