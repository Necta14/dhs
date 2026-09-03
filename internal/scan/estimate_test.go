package scan

import (
	"testing"
	"time"
)

func result(bytes map[Class]int64) *Result {
	r := &Result{ByClass: make(map[Class]Bucket, len(bytes))}
	for c, b := range bytes {
		r.ByClass[c] = Bucket{Files: 1, Bytes: b}
		r.Bytes += b
	}
	return r
}

func TestEstimateIncompressibleStaysWhole(t *testing.T) {
	// A directory holding only photos and videos does not shrink. The estimate must not lie.
	r := result(map[Class]int64{Incompressible: 12 << 30})
	e := Estimator{Workers: 8}.Estimate(r, LevelMax)
	if e.Min != r.Bytes || e.Max != r.Bytes {
		t.Errorf("incompressible: [%d, %d], want exactly %d", e.Min, e.Max, r.Bytes)
	}
}

func TestEstimateOrderingAcrossLevels(t *testing.T) {
	r := result(map[Class]int64{Text: 4 << 30, Binary: 2 << 30, Incompressible: 6 << 30})
	est := Estimator{Workers: 8}

	e1 := est.Estimate(r, LevelCompatible)
	e2 := est.Estimate(r, LevelBalanced)
	e3 := est.Estimate(r, LevelMax)

	if !(e1.Max > e2.Max && e2.Max > e3.Max) {
		t.Errorf("higher levels must compress better: %d, %d, %d", e1.Max, e2.Max, e3.Max)
	}
	if !(e1.Duration < e2.Duration && e2.Duration < e3.Duration) {
		t.Errorf("higher levels must take longer: %v, %v, %v", e1.Duration, e2.Duration, e3.Duration)
	}
	for _, e := range []Estimate{e1, e2, e3} {
		if e.Min > e.Max {
			t.Errorf("inverted range: [%d, %d]", e.Min, e.Max)
		}
		if e.Max > r.Bytes {
			t.Errorf("estimate %d exceeds the raw size %d", e.Max, r.Bytes)
		}
	}
}

func TestEstimateVolumes(t *testing.T) {
	// Exactly the requested scenario: 12 GiB of downloads, mostly incompressible.
	r := result(map[Class]int64{Incompressible: 12 << 30})
	e := Estimator{Workers: 8}.Estimate(r, LevelBalanced)

	want := int((e.Max + VolumeSize - 1) / VolumeSize)
	if e.Volumes != want {
		t.Errorf("volumes = %d, want %d", e.Volumes, want)
	}
	if e.Volumes != 4 {
		t.Errorf("12 GiB in 3.5 GiB volumes = %d volumes, want 4", e.Volumes)
	}

	// A 32 GB FAT32 stick: it fits, with room to spare.
	ok, left := e.Fits(29 << 30)
	if !ok || left <= 0 {
		t.Errorf("want it to fit on 29 GiB; ok=%v, left=%d", ok, left)
	}
	// On an 8 GiB stick it does not fit.
	if ok, _ := e.Fits(8 << 30); ok {
		t.Error("12 GiB cannot possibly fit on 8 GiB")
	}
}

func TestEstimateSamplingOverridesTable(t *testing.T) {
	r := result(map[Class]int64{Text: 1 << 30})
	base := Estimator{Workers: 4}.Estimate(r, LevelBalanced)

	// Sampling measured a much better ratio than the table's assumption.
	sampled := Estimator{Workers: 4, Override: map[Class]float64{Text: 0.05}}.Estimate(r, LevelBalanced)
	if !sampled.Sampled {
		t.Error("an estimate with Override must be marked as sampled")
	}
	if base.Sampled {
		t.Error("an estimate from the table must not be marked as sampled")
	}
	if sampled.Max >= base.Min {
		t.Errorf("the measured ratio (0.05) must beat the table: measured max=%d, table min=%d", sampled.Max, base.Min)
	}
}

func TestEstimateWorkersReduceTime(t *testing.T) {
	r := result(map[Class]int64{Text: 8 << 30})
	one := Estimator{Workers: 1}.Estimate(r, LevelMax)
	eight := Estimator{Workers: 8}.Estimate(r, LevelMax)
	if eight.Duration >= one.Duration {
		t.Errorf("8 cores must be faster: %v vs %v", eight.Duration, one.Duration)
	}
	if one.Duration < time.Minute {
		t.Errorf("8 GiB of text at level 3 on one core should take a long time, not %v", one.Duration)
	}
}

func TestEstimateEmpty(t *testing.T) {
	e := Estimator{}.Estimate(&Result{ByClass: map[Class]Bucket{}}, LevelBalanced)
	if e.Max != 0 || e.Volumes != 0 {
		t.Errorf("empty inventory: max=%d, volumes=%d; want zero", e.Max, e.Volumes)
	}
}

func TestLevelValid(t *testing.T) {
	for _, l := range []Level{LevelCompatible, LevelBalanced, LevelMax} {
		if !l.Valid() {
			t.Errorf("%v should be valid", l)
		}
	}
	if Level(0).Valid() || Level(4).Valid() {
		t.Error("only 1, 2 and 3 are valid levels")
	}
}
