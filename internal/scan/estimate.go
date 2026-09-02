package scan

import (
	"fmt"
	"time"
)

// Level e nivelul de compresie ales de utilizator.
type Level uint8

const (
	// LevelCompatible — ZIP/deflate. Rapid, iar pachetul necriptat se deschide pe orice calculator.
	LevelCompatible Level = 1
	// LevelBalanced — zstd cu fereastră lungă. Implicit: raport bun, timp rezonabil.
	LevelBalanced Level = 2
	// LevelMax — LZMA2, echivalentul „7-Zip Ultra". Lent; merită doar pe date comprimabile.
	LevelMax Level = 3
)

func (l Level) String() string {
	switch l {
	case LevelCompatible:
		return "1 · Compatibil"
	case LevelBalanced:
		return "2 · Echilibrat"
	case LevelMax:
		return "3 · Maxim"
	default:
		return fmt.Sprintf("necunoscut(%d)", uint8(l))
	}
}

// Valid spune dacă nivelul e unul dintre cele trei.
func (l Level) Valid() bool { return l >= LevelCompatible && l <= LevelMax }

// ratio e intervalul de compresie așteptat: octeți rezultați / octeți inițiali.
type ratio struct{ min, max float64 }

// ratios sunt estimările pornite din tabel, înainte de orice eșantionare. Sunt intenționat
// prudente: e mai bine să spunem „încape la limită" și să iasă mai bine, decât invers.
//
// Clasa Incompressible are raport 1.0 fiindcă acele fișiere se stochează, nu se comprimă —
// vezi decizia despre blocuri solide pe clasă din docs/COMPRESIE.md.
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

// throughput e viteza de compresie pe un nucleu, în octeți pe secundă. Cifrele sunt de ordin de
// mărime, nu promisiuni; servesc ca utilizatorul să știe dacă așteaptă minute sau ore.
var throughput = map[Level]float64{
	LevelCompatible: 80 << 20,
	LevelBalanced:   18 << 20,
	LevelMax:        2 << 20,
}

// storedThroughput e viteza pentru datele care nu se comprimă: le limitează discul, nu procesorul.
const storedThroughput float64 = 150 << 20

// VolumeSize e dimensiunea implicită a unui volum: sub limita FAT32 de 4 GiB.
const VolumeSize int64 = 3584 << 20 // 3,5 GiB

// Estimate e răspunsul la „cât ocupă și cât durează", înainte să scriem ceva pe disc.
type Estimate struct {
	Level Level `json:"nivel"`
	// Raw e totalul brut al fișierelor incluse.
	Raw int64 `json:"brut"`
	// Min și Max mărginesc dimensiunea pachetului. Raportăm interval, nu o cifră falsă precisă.
	Min int64 `json:"minim"`
	Max int64 `json:"maxim"`
	// Volumes e numărul de volume în cazul cel mai defavorabil (Max).
	Volumes int `json:"volume"`
	// Duration e timpul estimat de compresie, cu paralelizare pe nucleele disponibile.
	Duration time.Duration `json:"durata_ns"`
	// Sampled spune dacă rapoartele au fost măsurate prin eșantionare sau doar presupuse din tabel.
	Sampled bool `json:"esantionat"`
}

// Estimator calculează estimarea pentru un inventar dat.
type Estimator struct {
	// Workers e numărul de nuclee folosite la compresie.
	Workers int
	// VolumeSize permite schimbarea dimensiunii volumului (implicit 3,5 GiB).
	VolumeSize int64
	// Override, dacă e completat, înlocuiește raportul din tabel pentru o clasă — aici intră
	// rezultatele eșantionării.
	Override map[Class]float64
}

// Estimate calculează dimensiunea și timpul pentru inventarul dat, la nivelul cerut.
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
			// Un raport măsurat înlocuiește presupunerea, cu o marjă mică în ambele sensuri.
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

	// Compresia merge în paralel pe nuclee, dar scrierea pe disc rămâne în serie: nu promitem
	// o accelerare perfectă.
	out.Duration = time.Duration(seconds / float64(workers) * 1.15 * float64(time.Second))
	out.Volumes = int((out.Max + volume - 1) / volume)
	if out.Volumes == 0 && out.Max > 0 {
		out.Volumes = 1
	}
	return out
}

// Fits spune dacă pachetul încape în spațiul disponibil, în cazul cel mai defavorabil,
// și cât ar rămâne liber.
func (e Estimate) Fits(available int64) (bool, int64) {
	return e.Max <= available, available - e.Max
}
