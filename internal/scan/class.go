// Package scan takes inventory of what would be saved: it walks the tree, classifies the files
// and estimates how much the package would take up, without writing anything to disk.
package scan

import (
	"path/filepath"
	"strings"
)

// Class says how compressible a file is. It also decides whether we compress it at all:
// compressing a JPEG is time wasted for a single percent.
type Class uint8

const (
	// Unknown -- unknown extension. Decided by an entropy test at packing time.
	Unknown Class = iota
	// Incompressible -- already compressed: media, archives. Stored as-is.
	Incompressible
	// Binary -- executables, libraries, disk images. Compresses moderately.
	Binary
	// Text -- text, code, structured data. Compresses very well.
	Text
)

func (c Class) String() string {
	switch c {
	case Incompressible:
		return "incompressible"
	case Binary:
		return "binary"
	case Text:
		return "text"
	default:
		return "unknown"
	}
}

// Compressible says whether it is worth trying compression on this class.
func (c Class) Compressible() bool { return c == Text || c == Binary }

// classByExt is the classification table. Extensions are written without the dot, in lowercase.
var classByExt = map[string]Class{}

func register(c Class, exts ...string) {
	for _, e := range exts {
		classByExt[e] = c
	}
}

func init() {
	// Already compressed -- any further effort is waste.
	register(Incompressible,
		// images
		"jpg", "jpeg", "jpe", "jfif", "png", "gif", "webp", "heic", "heif", "avif", "jxl",
		// video
		"mp4", "m4v", "mkv", "avi", "mov", "wmv", "flv", "webm", "mpg", "mpeg", "ts", "m2ts", "3gp",
		// audio
		"mp3", "aac", "m4a", "ogg", "oga", "opus", "flac", "wma", "ape", "mka",
		// archives and packages
		"zip", "7z", "rar", "gz", "bz2", "xz", "zst", "lz4", "lzma", "br", "cab", "arj",
		"tgz", "tbz2", "txz", "apk", "jar", "war", "deb", "rpm", "pkg", "snap", "flatpak",
		"appimage", "dmg", "crx", "xpi", "nupkg", "whl", "egg",
		// documents that are ZIP containers
		"docx", "xlsx", "pptx", "docm", "xlsm", "pptm", "odt", "ods", "odp", "odg", "epub",
		// other already-compressed formats
		"pdf", "swf", "woff", "woff2",
	)

	// Compresses moderately.
	register(Binary,
		"exe", "dll", "sys", "msi", "msix", "appx", "so", "dylib", "a", "lib", "o", "obj",
		"bin", "dat", "db", "sqlite", "sqlite3", "mdb", "accdb", "pdb", "class", "pyc", "pyo",
		"iso", "img", "vhd", "vhdx", "vmdk", "qcow2", "vdi", "ova", "wim", "esd",
		"doc", "xls", "ppt", "rtf", "psd", "xcf", "ai", "indd", "blend", "fbx", "obj3d",
		"ttf", "otf", "eot", "icns", "ico", "cur",
	)

	// Compresses very well -- this is where the real gain is.
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

// noExtName classifies files without an extension, by their customary name.
var noExtName = map[string]Class{
	"makefile": Text, "dockerfile": Text, "readme": Text, "license": Text, "licence": Text,
	"changelog": Text, "authors": Text, "contributing": Text, "notice": Text, "copying": Text,
	"gemfile": Text, "rakefile": Text, "procfile": Text, "vagrantfile": Text, "justfile": Text,
}

// ClassOf classifies a file by its name. It does not open the file -- the inventory must stay
// fast; whatever comes out Unknown is settled later, by sampling.
func ClassOf(name string) Class {
	base := strings.ToLower(baseName(name))

	// Hidden files with no other extension (.bashrc, .gitignore) are configuration, hence text.
	// Careful: filepath.Ext(".bashrc") returns ".bashrc", not the empty string -- that is why we check first.
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
	// ".tar.gz" arrives here as "gz" and is already caught above; what remains are cases like ".json.bak".
	if trimmed := strings.TrimSuffix(base, "."+ext); strings.Contains(trimmed, ".") {
		if c, ok := classByExt[strings.TrimPrefix(filepath.Ext(trimmed), ".")]; ok {
			return c
		}
	}
	return Unknown
}
