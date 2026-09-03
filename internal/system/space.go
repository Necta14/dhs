package system

// FSKind is the family of the destination's file system. We mostly care whether it is FAT32,
// because there the 4 GiB per-file limit is not negotiable.
type FSKind string

const (
	FSUnknown FSKind = "unknown"
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

// MaxFileSize is the largest file size the file system accepts,
// or 0 if there is no limit that matters in practice.
func (k FSKind) MaxFileSize() int64 {
	if k == FSFAT32 {
		return 4<<30 - 1 // 4 GiB minus one byte
	}
	return 0
}

// Volume describes the destination of a package.
type Volume struct {
	Path  string `json:"path"`
	FS    FSKind `json:"fs"`
	Total int64  `json:"total"`
	Free  int64  `json:"free"`
}

// SpaceOf returns the space and the file system type for the given path.
func SpaceOf(path string) (Volume, error) { return spaceOf(path) }
