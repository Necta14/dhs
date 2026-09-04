package pack

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Necta14/dhs/internal/system"
)

// RootMap ties a system's standard locations to their concrete paths, in both directions.
type RootMap struct {
	// byLen is sorted by descending path length, so that the longest prefix wins:
	// ~/Documents/Photos must beat ~/Documents if both are standard locations.
	byLen  []rootPath
	byRoot map[Root]string
	home   string
}

type rootPath struct {
	root Root
	path string // clean, without a trailing separator
}

// NewRootMap builds the map from the locations detected on the current system.
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

// Add registers one more root: an application's configuration location, "apps/<id>/<key>",
// tied to its path on this system. On the source it makes Split file those paths under the
// application instead of under "config" or "other"; on the target it tells Join where they go.
func (m *RootMap) Add(root Root, path string) {
	p := filepath.Clean(path)
	m.byRoot[root] = p
	m.byLen = append(m.byLen, rootPath{root: root, path: p})
	sort.Slice(m.byLen, func(i, j int) bool { return len(m.byLen[i].path) > len(m.byLen[j].path) })
}

// Split breaks an absolute path into (root, relative path with "/"). Whatever is not under any
// standard location gets RootOther and keeps its absolute path, normalized to "/".
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

// Join does the reverse: (root, relative) → absolute path on the current system. For RootOther,
// files land under <home>/DHS-restored/<original path>, so that we never blindly write into an
// absolute path that came from another system.
func (m *RootMap) Join(root Root, rel string) (string, bool) {
	if root == RootOther {
		return filepath.Join(m.home, "DHS-restored", filepath.FromSlash(rel)), true
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

// otherKey turns an absolute path from any system into a portable relative path:
//
//	/home/you/project/a.txt   → home/you/project/a.txt
//	C:\Users\you\a.txt          → C/Users/you/a.txt
func otherKey(abs string) string {
	s := filepath.ToSlash(abs)
	if vol := filepath.VolumeName(abs); vol != "" {
		s = strings.TrimSuffix(vol, ":") + s[len(vol):]
	}
	return strings.TrimLeft(s, "/")
}
