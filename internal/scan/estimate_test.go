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
	// Un director numai cu poze și filme nu se micșorează. Estimarea nu are voie să mintă.
	r := result(map[Class]int64{Incompressible: 12 << 30})
	e := Estimator{Workers: 8}.Estimate(r, LevelMax)
	if e.Min != r.Bytes || e.Max != r.Bytes {
		t.Errorf("incompresibil: [%d, %d], aștept exact %d", e.Min, e.Max, r.Bytes)
	}
}

func TestEstimateOrderingAcrossLevels(t *testing.T) {
	r := result(map[Class]int64{Text: 4 << 30, Binary: 2 << 30, Incompressible: 6 << 30})
	est := Estimator{Workers: 8}

	e1 := est.Estimate(r, LevelCompatible)
	e2 := est.Estimate(r, LevelBalanced)
	e3 := est.Estimate(r, LevelMax)

	if !(e1.Max > e2.Max && e2.Max > e3.Max) {
		t.Errorf("nivelurile mai mari trebuie să comprime mai bine: %d, %d, %d", e1.Max, e2.Max, e3.Max)
	}
	if !(e1.Duration < e2.Duration && e2.Duration < e3.Duration) {
		t.Errorf("nivelurile mai mari trebuie să dureze mai mult: %v, %v, %v", e1.Duration, e2.Duration, e3.Duration)
	}
	for _, e := range []Estimate{e1, e2, e3} {
		if e.Min > e.Max {
			t.Errorf("interval inversat: [%d, %d]", e.Min, e.Max)
		}
		if e.Max > r.Bytes {
			t.Errorf("estimarea %d depășește brutul %d", e.Max, r.Bytes)
		}
	}
}

func TestEstimateVolumes(t *testing.T) {
	// Exact scenariul cerut: 12 GiB de descărcări, în mare parte incompresibile.
	r := result(map[Class]int64{Incompressible: 12 << 30})
	e := Estimator{Workers: 8}.Estimate(r, LevelBalanced)

	want := int((e.Max + VolumeSize - 1) / VolumeSize)
	if e.Volumes != want {
		t.Errorf("volume = %d, aștept %d", e.Volumes, want)
	}
	if e.Volumes != 4 {
		t.Errorf("12 GiB în volume de 3,5 GiB = %d volume, aștept 4", e.Volumes)
	}

	// Stick FAT32 de 32 GB: încape, cu rezervă.
	ok, left := e.Fits(29 << 30)
	if !ok || left <= 0 {
		t.Errorf("aștept să încapă pe 29 GiB; ok=%v, rămas=%d", ok, left)
	}
	// Pe un stick de 8 GiB nu încape.
	if ok, _ := e.Fits(8 << 30); ok {
		t.Error("12 GiB nu au cum să încapă pe 8 GiB")
	}
}

func TestEstimateSamplingOverridesTable(t *testing.T) {
	r := result(map[Class]int64{Text: 1 << 30})
	base := Estimator{Workers: 4}.Estimate(r, LevelBalanced)

	// Eșantionarea a măsurat un raport mult mai bun decât presupunerea din tabel.
	sampled := Estimator{Workers: 4, Override: map[Class]float64{Text: 0.05}}.Estimate(r, LevelBalanced)
	if !sampled.Sampled {
		t.Error("estimarea cu Override trebuie marcată ca eșantionată")
	}
	if base.Sampled {
		t.Error("estimarea din tabel nu trebuie marcată ca eșantionată")
	}
	if sampled.Max >= base.Min {
		t.Errorf("raportul măsurat (0,05) trebuie să bată tabelul: măsurat max=%d, tabel min=%d", sampled.Max, base.Min)
	}
}

func TestEstimateWorkersReduceTime(t *testing.T) {
	r := result(map[Class]int64{Text: 8 << 30})
	one := Estimator{Workers: 1}.Estimate(r, LevelMax)
	eight := Estimator{Workers: 8}.Estimate(r, LevelMax)
	if eight.Duration >= one.Duration {
		t.Errorf("8 nuclee trebuie să fie mai rapide: %v vs %v", eight.Duration, one.Duration)
	}
	if one.Duration < time.Minute {
		t.Errorf("8 GiB de text la nivelul 3 pe un nucleu ar trebui să dureze mult, nu %v", one.Duration)
	}
}

func TestEstimateEmpty(t *testing.T) {
	e := Estimator{}.Estimate(&Result{ByClass: map[Class]Bucket{}}, LevelBalanced)
	if e.Max != 0 || e.Volumes != 0 {
		t.Errorf("inventar gol: max=%d, volume=%d; aștept zero", e.Max, e.Volumes)
	}
}

func TestLevelValid(t *testing.T) {
	for _, l := range []Level{LevelCompatible, LevelBalanced, LevelMax} {
		if !l.Valid() {
			t.Errorf("%v ar trebui valid", l)
		}
	}
	if Level(0).Valid() || Level(4).Valid() {
		t.Error("doar 1, 2 și 3 sunt niveluri valide")
	}
}
