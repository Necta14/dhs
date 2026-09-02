// Package system detectează sistemul de operare și locurile standard din profilul utilizatorului.
//
// Codul specific unui sistem de operare stă exclusiv în fișierele cu sufix _linux.go și
// _windows.go. Datorită etichetelor de build, binarul pentru Linux nu conține niciun octet din
// codul de Windows, și invers — asta ține produsul mic, așa cum cere Regula #4.
package system

import (
	"fmt"
	"runtime"
)

// OS e sistemul de operare pe care rulăm sau din care provine un pachet.
type OS string

const (
	Linux   OS = "linux"
	Windows OS = "windows"
)

// Info descrie sistemul curent, atât cât ne trebuie pentru manifest și pentru planul de restaurare.
type Info struct {
	OS       OS     `json:"os"`
	Name     string `json:"nume"`     // „Arch Linux", „Windows 11 Pro"
	Version  string `json:"versiune"` // „rolling", „23H2"
	Arch     string `json:"arh"`      // amd64, arm64
	User     string `json:"user"`
	Home     string `json:"acasa"`
	Hostname string `json:"gazda"`
}

func (i Info) String() string {
	v := i.Name
	if i.Version != "" {
		v += " " + i.Version
	}
	return fmt.Sprintf("%s (%s)", v, i.Arch)
}

// Kind numește tipul unui loc standard din profil.
type Kind string

const (
	Documents Kind = "documente"
	Pictures  Kind = "imagini"
	Videos    Kind = "video"
	Music     Kind = "muzica"
	Downloads Kind = "descarcari"
	Desktop   Kind = "desktop"
	Config    Kind = "config"
)

// KindOrder e ordinea în care locurile se arată utilizatorului — de la cel mai probabil dorit.
var KindOrder = []Kind{Documents, Pictures, Videos, Music, Downloads, Desktop, Config}

// Location e un loc standard din profil, rezolvat la o cale concretă.
type Location struct {
	Kind Kind   `json:"tip"`
	Path string `json:"cale"`
	// Exists spune dacă directorul chiar există pe disc. Un profil proaspăt nu le are pe toate.
	Exists bool `json:"exista"`
}

// Detect află pe ce rulăm. Implementarea e per sistem de operare.
func Detect() (Info, error) { return detect() }

// Locations întoarce locurile standard din profilul utilizatorului, în ordinea din KindOrder.
// Locurile inexistente sunt incluse, cu Exists fals, ca interfața să le poată arăta cenușiu.
func Locations(i Info) []Location { return locations(i) }

// Arch normalizează arhitectura la forma folosită în manifest.
func Arch() string { return runtime.GOARCH }
