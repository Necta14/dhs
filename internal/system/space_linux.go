//go:build linux

package system

import "golang.org/x/sys/unix"

// Numerele magice vin din include/uapi/linux/magic.h din nucleu.
var fsMagic = map[int64]FSKind{
	0x4d44:     FSFAT32, // MSDOS_SUPER_MAGIC — acoperă și FAT16/FAT32
	0x2011BAB0: FSExFAT,
	0x5346544e: FSNTFS,
	0xEF53:     FSExt,
	0x9123683E: FSBtrfs,
	0x58465342: FSXFS,
	0x2FC12FC1: FSZFS,
	0xF2F52010: FSF2FS,
	0x01021994: FSTmpfs,
}

func spaceOf(path string) (Volume, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Volume{Path: path}, err
	}
	v := Volume{
		Path:  path,
		FS:    FSUnknown,
		Total: int64(st.Blocks) * st.Bsize,
		// Bavail, nu Bfree: blocurile rezervate pentru root nu ne sunt disponibile.
		Free: int64(st.Bavail) * st.Bsize,
	}
	if k, ok := fsMagic[int64(st.Type)]; ok {
		v.FS = k
	}
	return v, nil
}
