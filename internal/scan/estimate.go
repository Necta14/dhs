package scan

import (
	"fmt"
	"time"
)

// Level is the compression level chosen by the user.
type Level uint8

const (
	// LevelCompatible -- ZIP/deflate. Fast, and an unencrypted package opens on any computer.
	LevelCompatible Level = 1
	// LevelBalanced -- zstd with a long window. The default: good ratio, reasonable time.
	LevelBalanced Level = 2
	// LevelMax -- LZMA2, the equivalent of "7-Zip Ultra". Slow; worth it only on compressible data.
	LevelMax Level = 3
)

func (l Level) String() string {
	switch l {
	case LevelCompatible:
		return "1 · Compatible"
	case LevelBalanced:
		return "2 · Balanced"
	case LevelMax:
		return "3 · Maximum"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(l))
	}
}

// Valid says whether the level is one of the three.
func (l Level) Valid() bool { return l >= LevelCompatible && l <= LevelMax }

// ratio is the expected compression range: output bytes / input bytes.
type ratio struct{ min, max float64 }

// ratios are the estimates taken from the table, before any sampling. They are deliberately
// cautious: better to say "it barely fits" and have it come out better, than the other way round.
//
// The Incompressible class has a ratio of 1.0 because those files are stored, not compressed --
// see the decision on per-class solid blocks in docs/COMPRESIE.md.
var ratios = map[Level]map[Class]ratio{
	LevelCompatible: {
		Incompressible: {1.00, 1.00},
		Binary:         {0.62, 0.80},
		Text:           {0.32, 0.45},
		Unknown:        {0.75, 1.00},
	},
	LevelBalanced: {
		Incompressible: {1.00, 1.00},
		Binary:         {0.45, 0.62},
		Text:           {0.20, 0.32},
		Unknown:        {0.65, 1.00},
	},
	LevelMax: {
		Incompressible: {1.00, 1.00},
		Binary:         {0.38, 0.55},
		Text:           {0.16, 0.28},
		Unknown:        {0.60, 1.00},
	},
}

// throughput is the compression speed on one core, in bytes per second. The figures are orders
// of magnitude, not promises; they serve so the user knows whether to expect minutes or hours.
var throughput = map[Level]float64{
	LevelCompatible: 80 << 20,
	LevelBalanced:   18 << 20,
	LevelMax:        2 << 20,
}

// storedThroughput is the speed for data that is not compressed: the disk limits it, not the CPU.
const storedThroughput float64 = 150 << 20

// VolumeSize is the default size of a volume: under the FAT32 limit of 4 GiB.
const VolumeSize int64 = 3584 << 20 // 3.5 GiB

// Estimate is the answer to "how much does it take up and how long does it take", before we
// write anything to disk.
type Estimate struct {
	Level Level `json:"level"`
	// Raw is the raw total of the included files.
	Raw int64 `json:"raw"`
	// Min and Max bound the package size. We report a range, not a falsely precise figure.
	Min int64 `json:"min"`
	Max int64 `json:"max"`
	// Volumes is the number of volumes in the worst case (Max).
	Volumes int `json:"volumes"`
	// Duration is the estimated compression time, parallelised over the available cores.
	Duration time.Duration `json:"duration_ns"`
	// Sampled says whether the ratios were measured by sampling or merely assumed from the table.
	Sampled bool `json:"sampled"`
}

// Estimator computes the estimate for a given inventory.
type Estimator struct {
	// Workers is the number of cores used for compression.
	Workers int
	// VolumeSize allows changing the volume size (default 3.5 GiB).
	VolumeSize int64
	// Override, if filled in, replaces the table ratio for a class -- this is where the sampling
	// results go.
	Override map[Class]float64
}

// Estimate computes the size and time for the given inventory, at the requested level.
func (e Estimator) Estimate(r *Result, level Level) Estimate {
	workers := max(e.Workers, 1)
	volume := e.VolumeSize
	if volume <= 0 {
		volume = VolumeSize
	}
	table, ok := ratios[level]
	if !ok {
		table = ratios[LevelBalanced]
		level = LevelBalanced
	}

	out := Estimate{Level: level, Raw: r.Bytes, Sampled: len(e.Override) > 0}
	var seconds float64

	for class, bucket := range r.ByClass {
		rt, known := table[class]
		if !known {
			rt = table[Unknown]
		}
		lo, hi := rt.min, rt.max
		if measured, ok := e.Override[class]; ok {
			// A measured ratio replaces the assumption, with a small margin in both directions.
			lo, hi = measured*0.92, measured*1.08
		}
		size := float64(bucket.Bytes)
		out.Min += int64(size * lo)
		out.Max += int64(size * hi)

		speed := storedThroughput
		if class.Compressible() {
			speed = throughput[level]
		}
		seconds += size / speed
	}

	// Compression runs in parallel across cores, but writing to disk stays serial: we do not
	// promise a perfect speed-up.
	out.Duration = time.Duration(seconds / float64(workers) * 1.15 * float64(time.Second))
	out.Volumes = int((out.Max + volume - 1) / volume)
	if out.Volumes == 0 && out.Max > 0 {
		out.Volumes = 1
	}
	return out
}

// Fits says whether the package fits in the available space, in the worst case,
// and how much would remain free.
func (e Estimate) Fits(available int64) (bool, int64) {
	return e.Max <= available, available - e.Max
}
