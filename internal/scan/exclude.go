package scan

import (
	"path/filepath"
	"strings"
)

// Rule is an exclusion rule, with a displayable reason. The reason matters: the default list is
// shown to the user, and they must understand why 18 GiB are missing from the total.
type Rule struct {
	// Dir excludes a directory with this name, anywhere in the tree.
	Dir string
	// Reason is the explanation shown to the user.
	Reason string
	// Heavy marks the rules that usually cut a lot -- they are shown first in the report.
	Heavy bool
}

// DefaultRules are the exclusions enabled by default. They are visible and can be disabled:
// nothing disappears from the backup without the user being able to find out why.
var DefaultRules = []Rule{
	// Rebuilt from a single command; carrying them around is pure waste.
	{Dir: "node_modules", Reason: "dependencies, restored by npm install"},
	{Dir: "vendor", Reason: "vendored dependencies"},
	{Dir: "__pycache__", Reason: "Python cache"},
	{Dir: ".venv", Reason: "Python virtual environment, recreated"},
	{Dir: "venv", Reason: "Python virtual environment, recreated"},
	{Dir: ".tox", Reason: "test cache"},
	{Dir: "target", Reason: "build artifacts"},
	{Dir: ".gradle", Reason: "Gradle cache"},
	{Dir: ".m2", Reason: "Maven cache"},
	{Dir: ".cargo", Reason: "Cargo cache"},
	{Dir: ".rustup", Reason: "Rust toolchain, reinstalled"},
	{Dir: ".npm", Reason: "npm cache"},
	{Dir: ".pnpm-store", Reason: "pnpm cache"},
	{Dir: ".yarn", Reason: "Yarn cache"},
	{Dir: ".nuget", Reason: "NuGet cache"},
	{Dir: ".stack", Reason: "Haskell Stack cache"},
	{Dir: ".ccache", Reason: "compiler cache"},

	// System and application caches.
	{Dir: ".cache", Reason: "cache, rebuilt on its own", Heavy: true},
	{Dir: "Temp", Reason: "temporary files"},
	{Dir: "Temporary Internet Files", Reason: "browser cache"},
	{Dir: ".thumbnails", Reason: "thumbnails, regenerated"},
	{Dir: "thumbnails", Reason: "thumbnails, regenerated"},
	{Dir: "CrashDumps", Reason: "crash reports"},

	// Trash and system files.
	{Dir: ".Trash", Reason: "trash"},
	{Dir: "Trash", Reason: "trash"},
	{Dir: "$RECYCLE.BIN", Reason: "trash"},
	{Dir: "System Volume Information", Reason: "internal system data"},
	{Dir: "lost+found", Reason: "filesystem recovery"},

	// Huge and reinstallable -- the classic source of "why is the backup 400 GiB".
	{Dir: "steamapps", Reason: "Steam games, re-downloadable", Heavy: true},
	{Dir: "Steam", Reason: "Steam library, re-downloadable", Heavy: true},
	{Dir: "EpicGamesLauncher", Reason: "Epic games, re-downloadable", Heavy: true},
	{Dir: "VirtualBox VMs", Reason: "virtual machines, very large", Heavy: true},
	{Dir: "libvirt", Reason: "virtual machines, very large", Heavy: true},
	{Dir: ".vagrant", Reason: "virtual machines"},
	{Dir: "wine", Reason: "Wine prefixes, recreated"},
	{Dir: ".wine", Reason: "Wine prefixes, recreated"},

	// Version control: the history lives remotely, and .git can be huge.
	{Dir: ".git", Reason: "git history, lives on the server"},
	{Dir: ".svn", Reason: "SVN metadata"},
	{Dir: ".hg", Reason: "Mercurial metadata"},
}

// secretDirs, secretExts and secretNames are not ordinary exclusions: they are files that do not
// enter the package except with the user's explicit consent (D4). See IsSecret.
var secretDirs = map[string]string{
	".ssh":     "SSH keys",
	".gnupg":   "GPG keys",
	".aws":     "AWS credentials",
	".azure":   "Azure credentials",
	".kube":    "Kubernetes cluster access",
	".docker":  "registry credentials",
	"keyrings": "keyrings",
}

var secretExts = map[string]string{
	"pem": "private key", "key": "private key", "p12": "certificate with key",
	"pfx": "certificate with key", "jks": "Java keystore", "keystore": "keystore",
	"kdbx": "KeePass password database", "gpg": "GPG-encrypted data", "asc": "key or signature",
}

var secretNames = map[string]string{
	"id_rsa": "SSH key", "id_dsa": "SSH key", "id_ecdsa": "SSH key",
	"id_ed25519": "SSH key", "identity": "SSH key",
	"credentials": "credentials", ".netrc": "network credentials", ".pgpass": "PostgreSQL passwords",
	".htpasswd": "HTTP passwords",
}

// Excluder decides what enters the inventory and what does not.
type Excluder struct {
	dirs map[string]Rule
	// SkipHidden skips the hidden files and directories in the profile root.
	SkipHidden bool
}

// NewExcluder builds an excluder from the given rules.
func NewExcluder(rules []Rule) *Excluder {
	e := &Excluder{dirs: make(map[string]Rule, len(rules))}
	for _, r := range rules {
		e.dirs[strings.ToLower(r.Dir)] = r
	}
	return e
}

// DefaultExcluder is the excluder with the default rules.
func DefaultExcluder() *Excluder { return NewExcluder(DefaultRules) }

// Dir says whether a directory is skipped, and why.
func (e *Excluder) Dir(name string) (Rule, bool) {
	r, ok := e.dirs[strings.ToLower(name)]
	return r, ok
}

// Allow removes a rule from the excluder -- the user has explicitly asked for the directory back.
func (e *Excluder) Allow(name string) { delete(e.dirs, strings.ToLower(name)) }

// baseName takes the last segment of a path, whatever the separator. We do not rely on
// filepath.Base alone, because on Linux it does not recognise "\" -- and we also read manifests
// that come from Windows.
func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// pathParts splits a path into segments, accepting both separators, for the same reason.
func pathParts(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
}

// IsSecret says whether a path holds a secret, and of what kind. Secrets do not enter the package
// without explicit opt-in; see decision D4.
func IsSecret(path string) (string, bool) {
	base := baseName(path)
	lower := strings.ToLower(base)

	if reason, ok := secretNames[lower]; ok {
		return reason, true
	}
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), "."); ext != "" {
		if reason, ok := secretExts[ext]; ok {
			return reason, true
		}
	}
	// ".env", ".env.local", ".env.production" -- Rule #1 in CLAUDE.md.
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return "environment variables, may contain secrets", true
	}
	for _, part := range pathParts(path) {
		if reason, ok := secretDirs[strings.ToLower(part)]; ok {
			return reason, true
		}
	}
	return "", false
}
