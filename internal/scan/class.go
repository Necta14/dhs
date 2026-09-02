// Package scan face inventarul a ce s-ar salva: parcurge arborele, clasifică fișierele și
// estimează cât ar ocupa pachetul, fără să scrie nimic pe disc.
package scan

import (
	"path/filepath"
	"strings"
)

// Class spune cât de comprimabil e un fișier. De ea depinde și dacă îl comprimăm deloc:
// a comprima un JPEG e timp pierdut pentru un procent.
type Class uint8

const (
	// Unknown — extensie necunoscută. Se decide printr-un test de entropie la împachetare.
	Unknown Class = iota
	// Incompressible — deja comprimat: media, arhive. Se stochează ca atare.
	Incompressible
	// Binary — executabile, biblioteci, imagini de disc. Se comprimă moderat.
	Binary
	// Text — text, cod, date structurate. Se comprimă foarte bine.
	Text
)

func (c Class) String() string {
	switch c {
	case Incompressible:
		return "incompresibil"
	case Binary:
		return "binar"
	case Text:
		return "text"
	default:
		return "necunoscut"
	}
}

// Compressible spune dacă merită să încercăm compresia pe clasa asta.
func (c Class) Compressible() bool { return c == Text || c == Binary }

// classByExt e tabelul de clasificare. Extensiile sunt scrise fără punct, cu litere mici.
var classByExt = map[string]Class{}

func register(c Class, exts ...string) {
	for _, e := range exts {
		classByExt[e] = c
	}
}

func init() {
	// Deja comprimate — orice efort suplimentar e risipă.
	register(Incompressible,
		// imagini
		"jpg", "jpeg", "jpe", "jfif", "png", "gif", "webp", "heic", "heif", "avif", "jxl",
		// video
		"mp4", "m4v", "mkv", "avi", "mov", "wmv", "flv", "webm", "mpg", "mpeg", "ts", "m2ts", "3gp",
		// audio
		"mp3", "aac", "m4a", "ogg", "oga", "opus", "flac", "wma", "ape", "mka",
		// arhive și pachete
		"zip", "7z", "rar", "gz", "bz2", "xz", "zst", "lz4", "lzma", "br", "cab", "arj",
		"tgz", "tbz2", "txz", "apk", "jar", "war", "deb", "rpm", "pkg", "snap", "flatpak",
		"appimage", "dmg", "crx", "xpi", "nupkg", "whl", "egg",
		// documente care sunt containere ZIP
		"docx", "xlsx", "pptx", "docm", "xlsm", "pptm", "odt", "ods", "odp", "odg", "epub",
		// altele deja comprimate
		"pdf", "swf", "woff", "woff2",
	)

	// Se comprimă moderat.
	register(Binary,
		"exe", "dll", "sys", "msi", "msix", "appx", "so", "dylib", "a", "lib", "o", "obj",
		"bin", "dat", "db", "sqlite", "sqlite3", "mdb", "accdb", "pdb", "class", "pyc", "pyo",
		"iso", "img", "vhd", "vhdx", "vmdk", "qcow2", "vdi", "ova", "wim", "esd",
		"doc", "xls", "ppt", "rtf", "psd", "xcf", "ai", "indd", "blend", "fbx", "obj3d",
		"ttf", "otf", "eot", "icns", "ico", "cur",
	)

	// Se comprimă foarte bine — aici e câștigul real.
	register(Text,
		"txt", "md", "markdown", "rst", "adoc", "org", "tex", "log", "csv", "tsv",
		"json", "jsonl", "ndjson", "xml", "yaml", "yml", "toml", "ini", "cfg", "conf", "properties",
		"html", "htm", "xhtml", "css", "scss", "sass", "less", "svg",
		"js", "mjs", "cjs", "jsx", "ts", "tsx", "vue", "svelte",
		"go", "rs", "c", "h", "cc", "cpp", "cxx", "hpp", "hh", "java", "kt", "kts", "scala",
		"py", "pyi", "rb", "php", "pl", "pm", "lua", "r", "jl", "swift", "m", "mm",
		"cs", "fs", "vb", "sql", "sh", "bash", "zsh", "fish", "ps1", "psm1", "bat", "cmd",
		"dockerfile", "makefile", "cmake", "gradle", "sbt", "nix", "tf", "hcl", "proto",
		"patch", "diff", "srt", "vtt", "ass", "sub", "bmp", "tif", "tiff", "wav", "aiff", "pcm",
		"ppm", "pgm", "pbm", "xpm", "eps", "ps", "dxf", "gpx", "kml", "vcf", "ics", "mbox", "eml",
	)
}

// noExtName clasifică fișiere fără extensie, după numele lor uzual.
var noExtName = map[string]Class{
	"makefile": Text, "dockerfile": Text, "readme": Text, "license": Text, "licence": Text,
	"changelog": Text, "authors": Text, "contributing": Text, "notice": Text, "copying": Text,
	"gemfile": Text, "rakefile": Text, "procfile": Text, "vagrantfile": Text, "justfile": Text,
}

// ClassOf clasifică un fișier după nume. Nu deschide fișierul — inventarul trebuie să rămână
// rapid; ce iese Unknown se lămurește mai târziu, prin eșantionare.
func ClassOf(name string) Class {
	base := strings.ToLower(baseName(name))

	// Fișierele ascunse fără altă extensie (.bashrc, .gitignore) sunt configurări, deci text.
	// Atenție: filepath.Ext(".bashrc") întoarce „.bashrc", nu șirul gol — de aceea verificăm întâi.
	if rest, hidden := strings.CutPrefix(base, "."); hidden && rest != "" {
		if !strings.Contains(rest, ".") {
			if c, ok := classByExt[rest]; ok {
				return c
			}
			return Text
		}
	}

	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	if ext == "" {
		if c, ok := noExtName[base]; ok {
			return c
		}
		return Unknown
	}
	if c, ok := classByExt[ext]; ok {
		return c
	}
	if c, ok := noExtName[base]; ok {
		return c
	}
	// „.tar.gz" ajunge aici ca „gz" și e deja prins mai sus; rămân cazuri ca „.bak.json".
	if trimmed := strings.TrimSuffix(base, "."+ext); strings.Contains(trimmed, ".") {
		if c, ok := classByExt[strings.TrimPrefix(filepath.Ext(trimmed), ".")]; ok {
			return c
		}
	}
	return Unknown
}
