package scan

import (
	"path/filepath"
	"strings"
)

// Rule e o regulă de excludere, cu motivul afișabil. Motivul contează: lista implicită se arată
// utilizatorului, iar el trebuie să înțeleagă de ce lipsesc 18 GiB din total.
type Rule struct {
	// Dir exclude un director cu acest nume, oriunde în arbore.
	Dir string
	// Reason e explicația arătată utilizatorului.
	Reason string
	// Heavy marchează regulile care taie de obicei mult — se arată primele în raport.
	Heavy bool
}

// DefaultRules sunt excluderile pornite implicit. Sunt vizibile și se pot dezactiva:
// nimic nu dispare din backup fără ca utilizatorul să poată afla de ce.
var DefaultRules = []Rule{
	// Se refac dintr-o comandă; a le căra e risipă curată.
	{Dir: "node_modules", Reason: "dependențe, se refac cu npm install"},
	{Dir: "vendor", Reason: "dependențe vandorizate"},
	{Dir: "__pycache__", Reason: "cache Python"},
	{Dir: ".venv", Reason: "mediu virtual Python, se reface"},
	{Dir: "venv", Reason: "mediu virtual Python, se reface"},
	{Dir: ".tox", Reason: "cache de testare"},
	{Dir: "target", Reason: "artefacte de build"},
	{Dir: ".gradle", Reason: "cache Gradle"},
	{Dir: ".m2", Reason: "cache Maven"},
	{Dir: ".cargo", Reason: "cache Cargo"},
	{Dir: ".rustup", Reason: "toolchain Rust, se reinstalează"},
	{Dir: ".npm", Reason: "cache npm"},
	{Dir: ".pnpm-store", Reason: "cache pnpm"},
	{Dir: ".yarn", Reason: "cache Yarn"},
	{Dir: ".nuget", Reason: "cache NuGet"},
	{Dir: ".stack", Reason: "cache Haskell Stack"},
	{Dir: ".ccache", Reason: "cache de compilare"},

	// Cache-uri de sistem și de aplicații.
	{Dir: ".cache", Reason: "cache, se reface singur", Heavy: true},
	{Dir: "Temp", Reason: "fișiere temporare"},
	{Dir: "Temporary Internet Files", Reason: "cache de browser"},
	{Dir: ".thumbnails", Reason: "miniaturi, se refac"},
	{Dir: "thumbnails", Reason: "miniaturi, se refac"},
	{Dir: "CrashDumps", Reason: "rapoarte de eroare"},

	// Gunoi și fișiere de sistem.
	{Dir: ".Trash", Reason: "coș de gunoi"},
	{Dir: "Trash", Reason: "coș de gunoi"},
	{Dir: "$RECYCLE.BIN", Reason: "coș de gunoi"},
	{Dir: "System Volume Information", Reason: "date interne de sistem"},
	{Dir: "lost+found", Reason: "recuperare de sistem de fișiere"},

	// Enorme și reinstalabile — sursa clasică de „de ce are backupul 400 GiB".
	{Dir: "steamapps", Reason: "jocuri Steam, se redescarcă", Heavy: true},
	{Dir: "Steam", Reason: "bibliotecă Steam, se redescarcă", Heavy: true},
	{Dir: "EpicGamesLauncher", Reason: "jocuri Epic, se redescarcă", Heavy: true},
	{Dir: "VirtualBox VMs", Reason: "mașini virtuale, foarte mari", Heavy: true},
	{Dir: "libvirt", Reason: "mașini virtuale, foarte mari", Heavy: true},
	{Dir: ".vagrant", Reason: "mașini virtuale"},
	{Dir: "wine", Reason: "prefixe Wine, se recreează"},
	{Dir: ".wine", Reason: "prefixe Wine, se recreează"},

	// Controlul versiunilor: istoricul e la distanță, iar .git poate fi uriaș.
	{Dir: ".git", Reason: "istoric git, e pe server"},
	{Dir: ".svn", Reason: "metadate SVN"},
	{Dir: ".hg", Reason: "metadate Mercurial"},
}

// secretPatterns nu sunt excluderi obișnuite: sunt fișiere care nu intră în pachet decât cu
// acordul explicit al utilizatorului (D4). Vezi IsSecret.
var secretDirs = map[string]string{
	".ssh":     "chei SSH",
	".gnupg":   "chei GPG",
	".aws":     "credențiale AWS",
	".azure":   "credențiale Azure",
	".kube":    "acces la clustere Kubernetes",
	".docker":  "credențiale de registry",
	"keyrings": "inele de chei",
}

var secretExts = map[string]string{
	"pem": "cheie privată", "key": "cheie privată", "p12": "certificat cu cheie",
	"pfx": "certificat cu cheie", "jks": "depozit de chei Java", "keystore": "depozit de chei",
	"kdbx": "bază de parole KeePass", "gpg": "date cifrate GPG", "asc": "cheie sau semnătură",
}

var secretNames = map[string]string{
	"id_rsa": "cheie SSH", "id_dsa": "cheie SSH", "id_ecdsa": "cheie SSH",
	"id_ed25519": "cheie SSH", "identity": "cheie SSH",
	"credentials": "credențiale", ".netrc": "credențiale de rețea", ".pgpass": "parole PostgreSQL",
	".htpasswd": "parole HTTP",
}

// Excluder decide ce intră în inventar și ce nu.
type Excluder struct {
	dirs map[string]Rule
	// SkipHidden sare peste fișierele și directoarele ascunse din rădăcina profilului.
	SkipHidden bool
}

// NewExcluder construiește un excluder din regulile date.
func NewExcluder(rules []Rule) *Excluder {
	e := &Excluder{dirs: make(map[string]Rule, len(rules))}
	for _, r := range rules {
		e.dirs[strings.ToLower(r.Dir)] = r
	}
	return e
}

// DefaultExcluder e excluderul cu regulile implicite.
func DefaultExcluder() *Excluder { return NewExcluder(DefaultRules) }

// Dir spune dacă un director se sare, și de ce.
func (e *Excluder) Dir(name string) (Rule, bool) {
	r, ok := e.dirs[strings.ToLower(name)]
	return r, ok
}

// Allow scoate o regulă din excluder — utilizatorul a cerut explicit directorul înapoi.
func (e *Excluder) Allow(name string) { delete(e.dirs, strings.ToLower(name)) }

// baseName ia ultimul segment al unei căi, indiferent de separator. Nu folosim filepath.Base
// singur, fiindcă pe Linux el nu recunoaște „\" — iar noi citim și manifeste venite de pe Windows.
func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// pathParts sparge o cale în segmente, acceptând ambii separatori, din același motiv.
func pathParts(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
}

// IsSecret spune dacă o cale conține un secret și ce fel. Secretele nu intră în pachet fără
// opt-in explicit; vezi decizia D4.
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
	// „.env", „.env.local", „.env.production" — Regula #1 din CLAUDE.md.
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return "variabile de mediu, pot conține secrete", true
	}
	for _, part := range pathParts(path) {
		if reason, ok := secretDirs[strings.ToLower(part)]; ok {
			return reason, true
		}
	}
	return "", false
}
