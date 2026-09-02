package pack

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Necta14/dhs/internal/system"
)

// RootMap leagă locurile standard ale unui sistem de căile lor concrete, în ambele sensuri.
type RootMap struct {
	// byLen e sortat descrescător după lungimea căii, ca prefixul cel mai lung să câștige:
	// ~/Documente/Poze trebuie să bată ~/Documente dacă ambele sunt locuri standard.
	byLen  []rootPath
	byRoot map[Root]string
	home   string
}

type rootPath struct {
	root Root
	path string // curat, fără separator final
}

// NewRootMap construiește harta din locurile detectate pe sistemul curent.
func NewRootMap(home string, locs []system.Location) *RootMap {
	m := &RootMap{byRoot: make(map[Root]string, len(locs)), home: filepath.Clean(home)}
	for _, l := range locs {
		p := filepath.Clean(l.Path)
		m.byRoot[Root(l.Kind)] = p
		m.byLen = append(m.byLen, rootPath{root: Root(l.Kind), path: p})
	}
	sort.Slice(m.byLen, func(i, j int) bool { return len(m.byLen[i].path) > len(m.byLen[j].path) })
	return m
}

// Split împarte o cale absolută în (rădăcină, cale relativă cu „/"). Ce nu e sub niciun loc
// standard primește RootOther și își păstrează calea absolută, normalizată la „/".
func (m *RootMap) Split(abs string) (Root, string) {
	clean := filepath.Clean(abs)
	for _, rp := range m.byLen {
		if clean == rp.path {
			return rp.root, "."
		}
		if strings.HasPrefix(clean, rp.path+string(filepath.Separator)) {
			return rp.root, filepath.ToSlash(clean[len(rp.path)+1:])
		}
	}
	return RootOther, otherKey(clean)
}

// Join face inversul: (rădăcină, relativ) → cale absolută pe sistemul curent. Pentru RootOther,
// fișierele ajung sub <acasă>/DHS-restaurat/<cale originală>, ca să nu scriem niciodată orbește
// într-o cale absolută venită de pe alt sistem.
func (m *RootMap) Join(root Root, rel string) (string, bool) {
	if root == RootOther {
		return filepath.Join(m.home, "DHS-restaurat", filepath.FromSlash(rel)), true
	}
	base, ok := m.byRoot[root]
	if !ok {
		return "", false
	}
	if rel == "." || rel == "" {
		return base, true
	}
	return filepath.Join(base, filepath.FromSlash(rel)), true
}

// otherKey transformă o cale absolută de pe orice sistem într-o cale relativă portabilă:
//
//	/home/necta/proiect/a.txt   → home/necta/proiect/a.txt
//	C:\Users\necta\a.txt        → C/Users/necta/a.txt
func otherKey(abs string) string {
	s := filepath.ToSlash(abs)
	if vol := filepath.VolumeName(abs); vol != "" {
		s = strings.TrimSuffix(vol, ":") + s[len(vol):]
	}
	return strings.TrimLeft(s, "/")
}
