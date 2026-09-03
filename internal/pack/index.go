package pack

import (
	"time"

	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

// Root is the standard profile location a file belongs to. It is stored instead of the absolute
// path so that restoring works across systems: "documents/report.docx" lands in ~/Documents on
// Linux and in C:\Users\X\Documents on Windows, wherever those happen to be configured.
type Root string

// RootOther is for files outside the standard locations; they keep their original path.
const RootOther Root = "other"

// Manifest is dhs.json — unencrypted, hence strictly without file names, paths or user.
type Manifest struct {
	Format   uint16     `json:"format"`
	ID       ID         `json:"id"`
	Created  time.Time  `json:"created"`
	Source   SourceInfo `json:"source"`
	Cipher   string     `json:"cipher"`
	Level    scan.Level `json:"level"`
	Volumes  int        `json:"volumes"`
	Files    int64      `json:"files"`
	Raw      int64      `json:"raw_bytes"`
	Stored   int64      `json:"stored_bytes"`
	Secrets  bool       `json:"secrets_included"`
	Complete bool       `json:"complete"`
	Tool     string     `json:"tool"`
}

// SourceInfo is what we say about the system of origin — no user and no host name.
type SourceInfo struct {
	OS      system.OS `json:"os"`
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Arch    string    `json:"arch"`
}

func sourceFrom(i system.Info) SourceInfo {
	return SourceInfo{OS: i.OS, Name: i.Name, Version: i.Version, Arch: i.Arch}
}

// Index is the content of index.dhsi and of the index block in every volume.
type Index struct {
	Format   uint16       `json:"format"`
	ID       ID           `json:"id"`
	Complete bool         `json:"complete"`
	Volumes  []VolumeInfo `json:"volumes"`
	Blocks   []BlockRef   `json:"blocks"`
	Entries  []*Entry     `json:"entries"`
}

// VolumeInfo describes a finished volume.
type VolumeInfo struct {
	Number uint32 `json:"n"`
	Bytes  int64  `json:"bytes"` // size of the file on disk, encrypted
	Blocks int    `json:"blocks"`
	SHA256 Hash   `json:"sha256"` // of the file on disk
}

// BlockRef says where a block is and what is in it. The index into Index.Blocks is the block's identity.
type BlockRef struct {
	Volume uint32    `json:"v"`
	Pos    int64     `json:"pos"` // offset of the header, in the volume's decrypted stream
	Kind   BlockKind `json:"kind"`
	Raw    int64     `json:"raw"`
	Stored int64     `json:"stored"`
	SHA256 Hash      `json:"sha256"`
}

// Entry is a file in the package.
type Entry struct {
	Root    Root       `json:"root"`
	Path    string     `json:"path"`           // relative to Root, with "/"
	Orig    string     `json:"orig,omitempty"` // original absolute path, for the report and for RootOther
	Size    int64      `json:"size"`
	Mode    uint32     `json:"mode"`
	ModTime int64      `json:"mtime"` // Unix, nanoseconds
	SHA256  Hash       `json:"sha256"`
	Class   scan.Class `json:"class"`
	Secret  bool       `json:"secret,omitempty"`
	// Dup marks a file identical to another one in the package: its parts point at the same blocks.
	Dup   bool   `json:"dup,omitempty"`
	Parts []Part `json:"parts"`
}

// Part is a piece of a file: a range within a decompressed block.
type Part struct {
	Block  int   `json:"b"`
	Offset int64 `json:"off"`
	Length int64 `json:"len"`
}

// Placed says whether every part of the entry has its block written in a finished volume.
func (e *Entry) Placed(blocks []BlockRef) bool {
	for _, p := range e.Parts {
		if p.Block < 0 || p.Block >= len(blocks) || blocks[p.Block].Volume == 0 {
			return false
		}
	}
	return true
}

// Key uniquely identifies an entry within the package.
func (e *Entry) Key() string { return string(e.Root) + "\x00" + e.Path }

// clone copies the entry together with its parts. The writer uses it to serialize the index from
// a goroutine other than the one still adding parts, without holding the lock for as long as the
// JSON takes.
func (e *Entry) clone() *Entry {
	c := *e
	c.Parts = append([]Part(nil), e.Parts...)
	return &c
}
