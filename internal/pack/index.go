package pack

import (
	"time"

	"github.com/Necta14/dhs/internal/scan"
	"github.com/Necta14/dhs/internal/system"
)

// Root e locul standard din profil de care ține un fișier. Se stochează în loc de calea absolută
// ca restaurarea să funcționeze între sisteme: „documente/raport.docx" ajunge în ~/Documents pe
// Linux și în C:\Users\X\Documents pe Windows, oriunde ar fi ele configurate.
type Root string

// RootOther e pentru fișiere din afara locurilor standard; ele păstrează calea originală.
const RootOther Root = "alt"

// Manifest e dhs.json — necriptat, deci strict fără nume de fișiere, căi sau utilizator.
type Manifest struct {
	Format   uint16     `json:"format"`
	ID       ID         `json:"id"`
	Created  time.Time  `json:"creat"`
	Source   SourceInfo `json:"sursa"`
	Cipher   string     `json:"cifru"`
	Level    scan.Level `json:"nivel"`
	Volumes  int        `json:"volume"`
	Files    int64      `json:"fisiere"`
	Raw      int64      `json:"octeti_brut"`
	Stored   int64      `json:"octeti_stocati"`
	Secrets  bool       `json:"secrete_incluse"`
	Complete bool       `json:"complet"`
	Tool     string     `json:"unealta"`
}

// SourceInfo e ce spunem despre sistemul de origine — fără utilizator și fără nume de gazdă.
type SourceInfo struct {
	OS      system.OS `json:"os"`
	Name    string    `json:"nume"`
	Version string    `json:"versiune"`
	Arch    string    `json:"arh"`
}

func sourceFrom(i system.Info) SourceInfo {
	return SourceInfo{OS: i.OS, Name: i.Name, Version: i.Version, Arch: i.Arch}
}

// Index e conținutul lui index.dhsi și al blocului de index din fiecare volum.
type Index struct {
	Format   uint16       `json:"format"`
	ID       ID           `json:"id"`
	Complete bool         `json:"complet"`
	Volumes  []VolumeInfo `json:"volume"`
	Blocks   []BlockRef   `json:"blocuri"`
	Entries  []*Entry     `json:"intrari"`
}

// VolumeInfo descrie un volum încheiat.
type VolumeInfo struct {
	Number uint32 `json:"n"`
	Bytes  int64  `json:"octeti"` // dimensiunea fișierului pe disc, criptat
	Blocks int    `json:"blocuri"`
	SHA256 Hash   `json:"sha256"` // a fișierului de pe disc
}

// BlockRef spune unde e un bloc și ce e în el. Indexul în Index.Blocks e identitatea blocului.
type BlockRef struct {
	Volume uint32    `json:"v"`
	Pos    int64     `json:"poz"` // offset al antetului, în fluxul decriptat al volumului
	Kind   BlockKind `json:"tip"`
	Raw    int64     `json:"brut"`
	Stored int64     `json:"stocat"`
	SHA256 Hash      `json:"sha256"`
}

// Entry e un fișier din pachet.
type Entry struct {
	Root    Root       `json:"rad"`
	Path    string     `json:"cale"`           // relativ la Root, cu „/"
	Orig    string     `json:"orig,omitempty"` // calea originală absolută, pentru raport și pentru RootOther
	Size    int64      `json:"octeti"`
	Mode    uint32     `json:"mod"`
	ModTime int64      `json:"mtime"` // Unix, nanosecunde
	SHA256  Hash       `json:"sha256"`
	Class   scan.Class `json:"clasa"`
	Secret  bool       `json:"secret,omitempty"`
	// Dup marchează un fișier identic cu altul din pachet: părțile lui trimit la aceleași blocuri.
	Dup   bool   `json:"dup,omitempty"`
	Parts []Part `json:"parti"`
}

// Part e o bucată de fișier: un interval dintr-un bloc decomprimat.
type Part struct {
	Block  int   `json:"b"`
	Offset int64 `json:"poz"`
	Length int64 `json:"lung"`
}

// Placed spune dacă toate părțile intrării au blocul scris într-un volum încheiat.
func (e *Entry) Placed(blocks []BlockRef) bool {
	for _, p := range e.Parts {
		if p.Block < 0 || p.Block >= len(blocks) || blocks[p.Block].Volume == 0 {
			return false
		}
	}
	return true
}

// Key identifică unic o intrare în cadrul pachetului.
func (e *Entry) Key() string { return string(e.Root) + "\x00" + e.Path }
