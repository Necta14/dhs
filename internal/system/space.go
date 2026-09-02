package system

// FSKind e familia sistemului de fișiere al destinației. Ne interesează mai ales dacă e FAT32,
// fiindcă acolo limita de 4 GiB pe fișier nu e negociabilă.
type FSKind string

const (
	FSUnknown FSKind = "necunoscut"
	FSFAT32   FSKind = "FAT32"
	FSExFAT   FSKind = "exFAT"
	FSNTFS    FSKind = "NTFS"
	FSExt     FSKind = "ext2/3/4"
	FSBtrfs   FSKind = "btrfs"
	FSXFS     FSKind = "XFS"
	FSZFS     FSKind = "ZFS"
	FSF2FS    FSKind = "F2FS"
	FSTmpfs   FSKind = "tmpfs"
)

// MaxFileSize e cea mai mare dimensiune de fișier acceptată de sistemul de fișiere,
// sau 0 dacă nu e o limită relevantă în practică.
func (k FSKind) MaxFileSize() int64 {
	if k == FSFAT32 {
		return 4<<30 - 1 // 4 GiB minus un octet
	}
	return 0
}

// Volume descrie destinația unui pachet.
type Volume struct {
	Path  string `json:"cale"`
	FS    FSKind `json:"sistem_fisiere"`
	Total int64  `json:"total"`
	Free  int64  `json:"liber"`
}

// SpaceOf întoarce spațiul și tipul sistemului de fișiere pentru calea dată.
func SpaceOf(path string) (Volume, error) { return spaceOf(path) }
