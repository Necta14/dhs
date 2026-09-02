//go:build windows

package system

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func spaceOf(path string) (Volume, error) {
	v := Volume{Path: path, FS: FSUnknown}

	abs, err := filepath.Abs(path)
	if err != nil {
		return v, err
	}
	root := filepath.VolumeName(abs) + `\`
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return v, err
	}

	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(rootPtr, &free, &total, &totalFree); err != nil {
		return v, err
	}
	v.Total = int64(total)
	v.Free = int64(free)

	// Numele sistemului de fișiere vine ca text: „NTFS", „FAT32", „exFAT".
	nameBuf := make([]uint16, 256)
	fsBuf := make([]uint16, 256)
	var serial, maxComponent, flags uint32
	if err := windows.GetVolumeInformation(
		rootPtr,
		&nameBuf[0], uint32(len(nameBuf)),
		&serial, &maxComponent, &flags,
		&fsBuf[0], uint32(len(fsBuf)),
	); err == nil {
		switch strings.ToUpper(windows.UTF16ToString(fsBuf)) {
		case "FAT32", "FAT", "FAT16":
			v.FS = FSFAT32
		case "EXFAT":
			v.FS = FSExFAT
		case "NTFS":
			v.FS = FSNTFS
		case "REFS":
			v.FS = FSKind("ReFS")
		}
	}
	return v, nil
}
