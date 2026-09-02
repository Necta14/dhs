// Package pack definește formatul pachetului de migrare și îl scrie / citește.
//
// Pachetul e un director:
//
//	nume.dhs/
//	  dhs.json        manifest rădăcină, NECRIPTAT și fără nume de fișiere — doar ce e pachetul
//	  index.dhsi      indexul complet (fișiere, blocuri, plasare), CRIPTAT; mic, se citește primul
//	  volume/
//	    0001.dhsv     date: antet, blocuri, index propriu, trailer — CRIPTAT, ≤ 3,5 GiB
//	    0002.dhsv
//	  SUME.txt        SHA-256 pentru fiecare fișier din pachet
//	  jurnal.jsonl    o linie per volum încheiat, pentru reluare după întrerupere
//
// Fiecare volum e un flux scris o singură dată, secvențial, prin cifrul age. În interiorul fluxului
// stau blocuri: fie „stocate" (fișiere deja comprimate, copiate ca atare), fie „solide" (multe
// fișiere comprimabile împachetate împreună, apoi comprimate). La finalul fiecărui volum stă un
// index al volumului — redundanță pentru cazul în care index.dhsi se pierde — și un trailer fix.
//
// Toate numerele sunt little-endian. Toate căile din index folosesc „/" indiferent de sistem.
package pack

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// FormatVersion crește doar la schimbări incompatibile ale formatului binar sau ale indexului.
const FormatVersion uint16 = 1

const (
	volumeMagic  = "DHSV"
	blockMagic   = "DHSB"
	trailerMagic = "DHST"

	// DefaultVolumeSize e 3,5 GiB: sub limita FAT32 de 4 GiB, uniform pe orice mediu.
	DefaultVolumeSize int64 = 3584 << 20
	// DefaultBlockSize e cât adunăm într-un bloc solid înainte să-l comprimăm. Mai mare = compresie
	// puțin mai bună, dar mai multă memorie: la N lucrători avem ~2N blocuri în zbor.
	DefaultBlockSize = 32 << 20
	// MaxBlockSize e limita dură pe care o acceptă cititorul, ca un pachet stricat sau ostil să nu
	// ne facă să alocăm gigaocteți.
	MaxBlockSize = 256 << 20

	// volumeHeaderSize, blockHeaderSize și trailerSize sunt fixe; cititorul se bazează pe asta.
	volumeHeaderSize = 32
	blockHeaderSize  = 56
	trailerSize      = 32
)

// Cipher spune cum e protejat fluxul unui volum.
type Cipher uint8

const (
	CipherNone      Cipher = 0
	CipherAgeScrypt Cipher = 1
)

func (c Cipher) String() string {
	switch c {
	case CipherNone:
		return "niciunul"
	case CipherAgeScrypt:
		return "age-scrypt"
	default:
		return fmt.Sprintf("necunoscut(%d)", uint8(c))
	}
}

// BlockKind spune cum e codificat conținutul unui bloc.
type BlockKind uint8

const (
	KindStored  BlockKind = 0
	KindDeflate BlockKind = 1
	KindZstd    BlockKind = 2
	KindXZ      BlockKind = 3
	// KindIndex marchează indexul de la finalul volumului. Conținutul e JSON comprimat cu zstd.
	KindIndex BlockKind = 255
)

func (k BlockKind) String() string {
	switch k {
	case KindStored:
		return "stocat"
	case KindDeflate:
		return "deflate"
	case KindZstd:
		return "zstd"
	case KindXZ:
		return "xz"
	case KindIndex:
		return "index"
	default:
		return fmt.Sprintf("necunoscut(%d)", uint8(k))
	}
}

// ID identifică un pachet. Toate volumele îl poartă, ca să nu amestecăm volume din pachete diferite.
type ID [16]byte

func NewID() (ID, error) {
	var id ID
	if _, err := rand.Read(id[:]); err != nil {
		return id, fmt.Errorf("pack: nu pot genera id: %w", err)
	}
	return id, nil
}

func (id ID) String() string { return hex.EncodeToString(id[:]) }

// ParseID citește forma hex.
func ParseID(s string) (ID, error) {
	var id ID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(id) {
		return id, fmt.Errorf("pack: id invalid %q", s)
	}
	copy(id[:], b)
	return id, nil
}

// MarshalText și UnmarshalText fac ID-ul să apară în JSON ca șir hex.
func (id ID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }
func (id *ID) UnmarshalText(b []byte) error {
	parsed, err := ParseID(string(b))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// Hash e un SHA-256, serializat hex în JSON.
type Hash [32]byte

func (h Hash) String() string               { return hex.EncodeToString(h[:]) }
func (h Hash) MarshalText() ([]byte, error) { return []byte(h.String()), nil }
func (h *Hash) UnmarshalText(b []byte) error {
	d, err := hex.DecodeString(string(b))
	if err != nil || len(d) != len(h) {
		return fmt.Errorf("pack: hash invalid %q", string(b))
	}
	copy(h[:], d)
	return nil
}

// volumeHeader deschide fiecare volum: 32 de octeți.
//
//	0   magic "DHSV"      4
//	4   versiune format   2
//	6   număr volum       4
//	10  id pachet         16
//	26  nivel compresie   1
//	27  cifru             1
//	28  rezervat          4
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
		return h, fmt.Errorf("%w: volumul e în formatul %d, eu știu până la %d", ErrNewerFormat, h.Version, FormatVersion)
	}
	h.Volume = binary.LittleEndian.Uint32(b[6:10])
	copy(h.ID[:], b[10:26])
	h.Level = b[26]
	h.Cipher = Cipher(b[27])
	return h, nil
}

// blockHeader precede fiecare bloc: 56 de octeți.
//
//	0   magic "DHSB"      4
//	4   tip               1
//	5   flags             1
//	6   rezervat          2
//	8   dimensiune brută  8   (înainte de compresie)
//	16  dimensiune stocată 8  (cât urmează în flux)
//	24  sha256 brut       32
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
		return h, fmt.Errorf("%w: bloc cu dimensiuni imposibile (%d / %d)", ErrCorrupt, h.Raw, h.Stored)
	}
	return h, nil
}

// trailer închide fiecare volum: 32 de octeți.
//
//	0   magic "DHST"        4
//	4   număr blocuri       4   (fără blocul de index)
//	8   poziție bloc index  8   (offset în fluxul decriptat)
//	16  dimensiune volum    8   (octeți de flux decriptat, fără trailer)
//	24  rezervat            8
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
		return t, fmt.Errorf("%w: trailer lipsă sau stricat", ErrCorrupt)
	}
	t.Blocks = binary.LittleEndian.Uint32(b[4:8])
	t.IndexPos = int64(binary.LittleEndian.Uint64(b[8:16]))
	t.StreamLen = int64(binary.LittleEndian.Uint64(b[16:24]))
	return t, nil
}

// Erorile pe care le pot întâlni apelanții.
var (
	ErrNotVolume      = errors.New("pack: nu e un volum DHS")
	ErrNotPackage     = errors.New("pack: nu e un pachet DHS")
	ErrNewerFormat    = errors.New("pack: pachetul cere o versiune mai nouă de DHS")
	ErrCorrupt        = errors.New("pack: date corupte")
	ErrNeedPassphrase = errors.New("pack: pachetul e criptat; trebuie fraza de acces")
	ErrBadPassphrase  = errors.New("pack: fraza de acces e greșită")
	ErrIncomplete     = errors.New("pack: pachetul nu e complet")
	ErrWrongPackage   = errors.New("pack: volumul aparține altui pachet")
)

// readFull citește exact n octeți sau întoarce eroare — fără citiri parțiale tăcute.
func readFull(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: flux trunchiat", ErrCorrupt)
		}
		return nil, err
	}
	return b, nil
}
