package i18n

// ro is the Romanian catalog. Keys are the exact English strings that cmd/dhs passes through
// T and Tf; format verbs must stay identical between a key and its translation.
var ro = map[string]string{
	// ---- help texts --------------------------------------------------------------------------

	`dhs — Direct Handoff Suite · migrate your working environment between operating systems

Usage
  dhs <command> [options]

Commands
  scan       show what would be saved, how much it takes and whether it fits the destination
  backup     create the migration package on the external drive
  verify     verify a package, block by block, without extracting anything
  list       show what is inside a package
  restore    put the files back in place on the current system, following an approved plan
  version    version and detected system

Help for a command
  dhs <command> --help
`: `dhs — Direct Handoff Suite · migrarea mediului de lucru între sisteme de operare

Utilizare
  dhs <comandă> [opțiuni]

Comenzi
  scan       arată ce s-ar salva, cât ar ocupa și dacă încape pe destinație
  backup     creează pachetul de migrare pe mediul extern
  verify     verifică un pachet, bloc cu bloc, fără să extragă nimic
  list       arată ce e într-un pachet
  restore    pune fișierele la locul lor pe sistemul curent, după un plan aprobat
  version    versiunea și sistemul detectat

Ajutor pentru o comandă
  dhs <comandă> --help
`,

	`dhs scan — show what would be saved and how much it takes, without writing anything

Usage
  dhs scan [path...] [options]

Without paths, scans the standard places in the profile: documents, pictures, videos, music,
downloads, desktop and configuration.

Options
  --dest <path>    package destination; checks whether it fits and which filesystem it has
  --level <1|2|3>  compression level: 1 compatible, 2 balanced (default), 3 maximum
  --secrets        include keys, passwords and .env files (by default they are only counted)
  --all            exclude nothing; also show caches, games, virtual machines
  --json           JSON output, for graphical interfaces and scripts
  --help
`: `dhs scan — arată ce s-ar salva și cât ar ocupa, fără să scrie nimic

Utilizare
  dhs scan [cale...] [opțiuni]

Fără căi, scanează locurile standard din profil: documente, imagini, video, muzică,
descărcări, desktop și configurări.

Opțiuni
  --dest <cale>    destinația pachetului; verifică dacă încape și ce sistem de fișiere are
  --level <1|2|3>  nivelul de compresie: 1 compatibil, 2 echilibrat (implicit), 3 maxim
  --secrets        include chei, parole și fișiere .env (implicit sunt doar numărate)
  --all            nu exclude nimic; arată și cache-urile, jocurile, mașinile virtuale
  --json           ieșire JSON, pentru interfețe grafice și scripturi
  --help
`,

	`dhs backup — create the migration package

Usage
  dhs backup --dest <path> [path...] [options]

Without paths, saves the standard places in the profile. The package is a directory
<dest>/<name>.dhs, made of 3.5 GiB volumes. Before writing anything, it shows the estimate and
asks for confirmation.

Options
  --dest <path>       where the package is written (required)
  --name <name>       name of the package directory; default migration-<date>-<time>
  --level <1|2|3>     compression: 1 compatible, 2 balanced (default), 3 maximum
  --secrets           include keys, passwords and .env files — excluded by default
  --all               exclude nothing (caches, games, virtual machines)
  --no-encrypt        write the package unencrypted. With level 1 it gives a package that opens without DHS
  --passphrase-file <path>   read the passphrase from a file (for automation); otherwise it is asked
  --yes               do not ask for confirmation
  --verify            after writing, verify the whole package (decrypt and check every block)
  --json              JSON output
  --help
`: `dhs backup — creează pachetul de migrare

Utilizare
  dhs backup --dest <cale> [cale...] [opțiuni]

Fără căi, salvează locurile standard din profil. Pachetul e un director <dest>/<nume>.dhs,
format din volume de 3,5 GiB. Înainte să scrie ceva, arată estimarea și cere confirmare.

Opțiuni
  --dest <cale>       unde se scrie pachetul (obligatoriu)
  --name <nume>       numele directorului pachetului; implicit migration-<data>-<ora>
  --level <1|2|3>     compresie: 1 compatibil, 2 echilibrat (implicit), 3 maxim
  --secrets           include chei, parole și fișiere .env — excluse implicit
  --all               nu exclude nimic (cache-uri, jocuri, mașini virtuale)
  --no-encrypt        scrie pachetul necriptat. Cu nivelul 1 dă un pachet deschizabil fără DHS
  --passphrase-file <cale>   citește fraza de acces din fișier (pentru automatizare); altfel o cere
  --yes               nu cere confirmare
  --verify            după scriere, verifică integral pachetul (decriptează și verifică fiecare bloc)
  --json              ieșire JSON
  --help
`,

	`dhs verify — verify a package, without extracting anything

Usage
  dhs verify <package> [options]

Checks the checksum of every file in the package, then decrypts every volume and checks every
block against its checksum. If it passes, the package can be restored in full.

Options
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --json
  --help
`: `dhs verify — verifică un pachet, fără să extragă nimic

Utilizare
  dhs verify <pachet> [opțiuni]

Verifică sumele fiecărui fișier din pachet, apoi decriptează fiecare volum și verifică fiecare bloc
față de suma lui de control. Dacă trece, pachetul se poate restaura integral.

Opțiuni
  --passphrase-file <cale>   citește fraza de acces din fișier; altfel o cere
  --json
  --help
`,

	`dhs list — show what is inside a package, without extracting anything

Usage
  dhs list <package> [options]

Options
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --all                      list every file, not just the summary
  --json
  --help
`: `dhs list — arată ce e într-un pachet, fără să extragă nimic

Utilizare
  dhs list <pachet> [opțiuni]

Opțiuni
  --passphrase-file <cale>   citește fraza de acces din fișier; altfel o cere
  --all                      listează fiecare fișier, nu doar rezumatul
  --json
  --help
`,

	`dhs restore — put the files from a package back in place on the current system

Usage
  dhs restore <package> [options]

First shows the plan: what goes where, what already exists, which names had to be adapted. Writes
nothing without confirmation. Every file is written to a temporary, checked against the checksum
from the package, and only then put in its place. A file that fails never reaches the destination.

Options
  --dry-run                  only the plan, without confirmation and without writing
  --only <root,root>         restore only these places: documents, pictures, videos, music,
                             downloads, desktop, config, other
  --conflicts <policy>       what to do when the file already exists:
                               keep-both   keep the existing one, write the restored one with a " (DHS)" suffix (default)
                               skip        do not write the restored one
                               overwrite   replace the existing one — only if you know what you are doing
  --passphrase-file <path>   read the passphrase from a file; otherwise it is asked
  --yes                      do not ask for confirmation
  --json
  --help
`: `dhs restore — pune fișierele dintr-un pachet la locul lor pe sistemul curent

Utilizare
  dhs restore <pachet> [opțiuni]

Întâi arată planul: ce merge unde, ce există deja, ce nume au trebuit adaptate. Nu scrie nimic
fără confirmare. Fiecare fișier se scrie într-un temporar, se verifică față de suma din pachet și
abia apoi e pus la locul lui. Un fișier care nu trece nu ajunge niciodată la destinație.

Opțiuni
  --dry-run                  doar planul, fără confirmare și fără scriere
  --only <rad,rad>           restaurează doar aceste locuri: documents, pictures, videos, music,
                             downloads, desktop, config, other
  --conflicts <politica>     ce facem când fișierul există deja:
                               keep-both   păstrează existentul, scrie restauratul cu sufix „ (DHS)” (implicit)
                               skip        nu scrie restauratul
                               overwrite   înlocuiește existentul — doar dacă știi ce faci
  --passphrase-file <cale>   citește fraza de acces din fișier; altfel o cere
  --yes                      nu cere confirmare
  --json
  --help
`,

	// ---- main --------------------------------------------------------------------------------
	"dhs: unknown command %q\n\n%s": "dhs: comandă necunoscută %q\n\n%s",

	// ---- shared: errors and labels used by several commands ----------------------------------
	"cannot detect the system: %w":              "nu pot detecta sistemul: %w",
	"cannot read the passphrase from %s: %w":    "nu pot citi fraza din %s: %w",
	"the package did not pass verification":     "pachetul nu a trecut verificarea",
	"cancelled":                                 "anulat",
	"level %d does not exist; choose 1, 2 or 3": "nivelul %d nu există; alege 1, 2 sau 3",
	"Package":                          "Pachet",
	"Verified":                         "Verificat",
	"Source":                           "Sursă",
	"Contents":                         "Conținut",
	"%s%s %s (%s) · created %s · %s\n": "%s%s %s (%s) · creat %s · %s\n",
	"\r\033[K  reading… %s files, %s":  "\r\033[K  citesc… %s fișiere, %s",
	"\r\033[K  verifying… %s / %s":     "\r\033[K  verific… %s / %s",
	"unknown":                          "necunoscut", // scan.Class and system.FSKind

	// ---- scan --------------------------------------------------------------------------------
	"no standard place found in %s; give the paths explicitly": "n-am găsit niciun loc standard în %s; dă căile explicit",
	"cannot read the destination %s: %w":                       "nu pot citi destinația %s: %w",
	"System":                                                   "Sistem",
	"Roots":                                                    "Rădăcini",
	"%s%s in %s files   %s\n":                                  "%s%s în %s fișiere   %s\n",
	"To include":                                               "De inclus",
	"(scanned in %s)":                                          "(scanat în %s)",
	"Excluded":                                                 "Excluse",
	"--all brings them back":                                   "--all le include înapoi",
	"… and %d more rules":                                      "… și încă %d reguli",
	"Secrets":                                                  "Secrete",
	"excluded by default; --secrets includes them": "excluse implicit; --secrets le include",
	"Composition":                              "Compoziție",
	"stored, not compressed":                   "se stochează, nu se comprimă",
	"settled by an entropy test while packing": "se lămurește prin test de entropie la împachetare",
	"Level":          "Nivel",
	"Estimate":       "Estimare",
	"%s on %d cores": "%s pe %d nuclee",
	"estimate from table; --precise would measure it by sampling": "estimare din tabel; --precise o va măsura prin eșantionare",
	"Volumes":                     "Volume",
	"Destination":                 "Destinație",
	"%s%s   %s free of %s   %s\n": "%s%s   %s liberi din %s   %s\n",
	"%s limits files to %s — the %s volumes are under the limit": "%s limitează fișierele la %s — volumele de %s sunt sub limită",
	"✓ Fits, with %s to spare.":                                  "✓ Încape, cu %s de rezervă.",
	"✗ Does not fit: %s missing.":                                "✗ Nu încape: lipsesc %s.",
	"exclude some of the largest items, raise the compression level, or split the package across several drives": "exclude din ce e mai mare, urcă nivelul de compresie, sau împarte pachetul pe mai multe medii",
	"Largest":      "Cele mai mari",
	"Inaccessible": "Inaccesibile",
	"%d places skipped (missing permissions)": "%d locuri sărite (drepturi lipsă)",

	// scan.Class names (besides "unknown", above)
	"incompressible": "incompresibil",
	"binary":         "binar",
	"text":           "text",

	// scan.Level names
	"1 · Compatible": "1 · Compatibil",
	"2 · Balanced":   "2 · Echilibrat",
	"3 · Maximum":    "3 · Maxim",

	// scan.Rule reasons (internal/scan/exclude.go)
	"dependencies, restored by npm install": "dependențe, se refac cu npm install",
	"vendored dependencies":                 "dependențe vandorizate",
	"Python cache":                          "cache Python",
	"Python virtual environment, recreated": "mediu virtual Python, se reface",
	"test cache":                            "cache de testare",
	"build artifacts":                       "artefacte de build",
	"Gradle cache":                          "cache Gradle",
	"Maven cache":                           "cache Maven",
	"Cargo cache":                           "cache Cargo",
	"Rust toolchain, reinstalled":           "toolchain Rust, se reinstalează",
	"npm cache":                             "cache npm",
	"pnpm cache":                            "cache pnpm",
	"Yarn cache":                            "cache Yarn",
	"NuGet cache":                           "cache NuGet",
	"Haskell Stack cache":                   "cache Haskell Stack",
	"compiler cache":                        "cache de compilare",
	"cache, rebuilt on its own":             "cache, se reface singur",
	"cache, regenerated":                    "cache, se reface singur", // variant from the translation brief
	"temporary files":                       "fișiere temporare",
	"browser cache":                         "cache de browser",
	"thumbnails, regenerated":               "miniaturi, se refac",
	"crash reports":                         "rapoarte de eroare",
	"trash":                                 "coș de gunoi",
	"internal system data":                  "date interne de sistem",
	"system internals":                      "date interne de sistem", // variant from the translation brief
	"filesystem recovery":                   "recuperare de sistem de fișiere",
	"Steam games, re-downloadable":          "jocuri Steam, se redescarcă",
	"Steam library, re-downloadable":        "bibliotecă Steam, se redescarcă",
	"Epic games, re-downloadable":           "jocuri Epic, se redescarcă",
	"virtual machines, very large":          "mașini virtuale, foarte mari",
	"virtual machines":                      "mașini virtuale",
	"Wine prefixes, recreated":              "prefixe Wine, se recreează",
	"git history, lives on the server":      "istoric git, e pe server",
	"SVN metadata":                          "metadate SVN",
	"Mercurial metadata":                    "metadate Mercurial",

	// scan secret reasons (internal/scan/exclude.go). Not shown by cmd today — the CLI only counts
	// secrets — but kept here so a future listing of them comes out translated.
	"SSH keys":                  "chei SSH",
	"GPG keys":                  "chei GPG",
	"AWS credentials":           "credențiale AWS",
	"Azure credentials":         "credențiale Azure",
	"Kubernetes cluster access": "acces la clustere Kubernetes",
	"registry credentials":      "credențiale de registry",
	"keyrings":                  "inele de chei",
	"private key":               "cheie privată",
	"certificate with key":      "certificat cu cheie",
	"Java keystore":             "depozit de chei Java",
	"keystore":                  "depozit de chei",
	"KeePass password database": "bază de parole KeePass",
	"GPG-encrypted data":        "date cifrate GPG",
	"key or signature":          "cheie sau semnătură",
	"SSH key":                   "cheie SSH",
	"credentials":               "credențiale",
	"network credentials":       "credențiale de rețea",
	"PostgreSQL passwords":      "parole PostgreSQL",
	"HTTP passwords":            "parole HTTP",
	"environment variables, may contain secrets": "variabile de mediu, pot conține secrete",

	// ---- backup ------------------------------------------------------------------------------
	"backup: --dest is required":                               "backup: --dest e obligatoriu",
	"backup: nothing to save":                                  "backup: nimic de salvat",
	"destination %s: %w":                                       "destinația %s: %w",
	"destination %s is not a directory":                        "destinația %s nu e un director",
	"cannot read the free space on %s: %w":                     "nu pot citi spațiul de pe %s: %w",
	"does not fit: %s missing on %s":                           "nu încape: lipsesc %s pe %s",
	"backup: the inventory is empty":                           "backup: inventarul e gol",
	"Writing %s files (%s) to %s. Continue?":                   "Scriu %s fișiere (%s) în %s. Continui?",
	"backup --json requires --passphrase-file or --no-encrypt": "backup --json cere --passphrase-file sau --no-encrypt",
	"The package will be UNENCRYPTED: anyone who gets hold of the drive can read your files.": "Pachetul va fi NECRIPTAT: oricine pune mâna pe disc îți citește fișierele.",
	"Continue without encryption?":                                 "Continui fără criptare?",
	"\r\033[K  %s / %s files · %s read · %s written · volume %d":   "\r\033[K  %s / %s fișiere · %s citiți · %s scriși · volumul %d",
	"interrupted; the package %s is incomplete and can be deleted": "întrerupt; pachetul %s e incomplet și poate fi șters",
	"writing failed: %w":                                           "scrierea a eșuat: %w",
	"closing the package failed: %w":                               "închiderea pachetului a eșuat: %w",
	"the package was written but cannot be reopened: %w":           "pachetul s-a scris dar nu se poate redeschide: %w",
	"%s%s files · %s → %s on disk · %d volumes · %s\n":             "%s%s fișiere · %s → %s pe disc · %d volume · %s\n",
	"Written": "Scris",
	"%s%d files could not be read (see --json for the list)\n": "%s%d fișiere n-au putut fi citite (vezi --json pentru listă)\n",
	"Skipped":                                  "Sărite",
	"✓ all blocks and checksums are fine":      "✓ toate blocurile și sumele sunt în regulă",
	"✗ %d problems":                            "✗ %d probleme",
	"  — verifies the whole package, any time": "  — verifică integral, oricând",
	"Without the passphrase the package is lost for good. We cannot recover it.": "Fără fraza de acces, pachetul e pierdut definitiv. N-o putem recupera.",

	// ---- verify ------------------------------------------------------------------------------
	"verify: give exactly one package":                                 "verify: dă exact un pachet",
	"%s%s files · %s → %s · %d volumes · level %s · cipher %s\n":       "%s%s fișiere · %s → %s · %d volume · nivel %s · cifru %s\n",
	"%s%d package files, %d volumes, %d blocks, %s raw\n":              "%s%d fișiere de pachet, %d volume, %d blocuri, %s bruți\n",
	"✓ The package is intact.":                                         "✓ Pachetul e întreg.",
	"✗ %d problems:":                                                   "✗ %d probleme:",
	"the package is encrypted; with --json you need --passphrase-file": "pachetul e criptat; cu --json trebuie --passphrase-file",
	"Package passphrase: ":                                             "Fraza de acces a pachetului: ",
	"wrong passphrase":                                                 "fraza de acces e greșită",

	// ---- list --------------------------------------------------------------------------------
	"list: give exactly one package": "list: dă exact un pachet",
	"complete":                       "complet",
	"INCOMPLETE":                     "INCOMPLET",
	"%s%s files · %s → %s on disk · %d volumes · level %s · cipher %s · %s\n": "%s%s fișiere · %s → %s pe disc · %d volume · nivel %s · cifru %s · %s\n",
	"contains secrets (keys, passwords)":                                      "conține secrete (chei, parole)",
	"%s%s %s files  %s\n":                                                     "%s%s %s fișiere  %s\n",
	"%d files are duplicates of others and take up space only once":           "%d fișiere sunt duplicate ale altora și ocupă loc o singură dată",
	" (secret)": " (secret)",

	// ---- restore -----------------------------------------------------------------------------
	"restore: give exactly one package": "restore: dă exact un pachet",
	"The package is marked INCOMPLETE: its writing was interrupted. Only what is intact will be restored.": "Pachetul e marcat INCOMPLET: scrierea lui a fost întreruptă. Se restaurează doar ce e întreg.",
	"Nothing to write.":                                                "Nimic de scris.",
	"--dry-run: nothing was written.":                                  "--dry-run: nu s-a scris nimic.",
	"no room on %s: %s needed, %s free":                                "nu e loc pe %s: trebuie %s, sunt %s liberi",
	"%d existing files will be OVERWRITTEN. They cannot be recovered.": "%d fișiere existente vor fi SUPRASCRISE. Nu se pot recupera.",
	"Writing %s files (%s). Continue?":                                 "Scriu %s fișiere (%s). Continui?",
	"restore --json requires --yes or --dry-run":                       "restore --json cere --yes sau --dry-run",
	"\r\033[K  restoring… %s / %s":                                     "\r\033[K  restaurez… %s / %s",
	"the restore stopped: %w":                                          "restaurarea s-a oprit: %w",
	"%s%s files · %s · %s\n":                                           "%s%s fișiere · %s · %s\n",
	"Restored":                                                         "Restaurat",
	"Failed":                                                           "Eșuate",
	"%d files could not be restored intact — nothing was written for them:": "%d fișiere nu s-au putut restaura întregi — nu s-a scris nimic pentru ele:",
	"… and %d more (see --json)":                                            "… și încă %d (vezi --json)",
	"incomplete restore":                                                    "restaurare incompletă",
	"✓ All files were verified and put back in place.":                      "✓ Toate fișierele au fost verificate și puse la loc.",
	"Migration":                      "Migrare",
	"%s%s files · %s · created %s\n": "%s%s fișiere · %s · creat %s\n",
	"Where it goes":                  "Unde merge",
	"%s files, %s":                   "%s fișiere, %s",
	"places that do not exist here, moved under DHS-restored: ": "locuri care nu există aici, mutate sub DHS-restored: ",
	"%s%s files · %s\n": "%s%s fișiere · %s\n",
	"To write":          "De scris",
	"the existing one stays, the restored one gets the \" (DHS)\" suffix": "existentul rămâne, restauratul primește sufixul „ (DHS)”",
	"the restored one is not written":                                     "restauratul nu se scrie",
	"the existing one is REPLACED":                                        "existentul e ÎNLOCUIT",
	"%s%d already exist → %s\n":                                           "%s%d există deja → %s\n",
	"Conflicts":                                                           "Conflicte",
	"%s%d names adapted to this system (forbidden characters, case, reserved names)\n": "%s%d nume adaptate acestui sistem (caractere interzise, majuscule, nume rezervate)\n",
	"Renamed":              "Redenumite",
	"… the rest in --json": "… restul în --json",

	// restore.Action values, as shown in the plan (internal/restore/plan.go)
	"write":        "scrie",
	"write-beside": "scrie-alături",
	"skip":         "sare",
	"overwrite":    "suprascrie",

	// ---- prompt ------------------------------------------------------------------------------
	"%s [y/N] ":                            "%s [d/N] ",
	"Choose a passphrase for the package.": "Alege o frază de acces pentru pachet.",
	"If you lose it, the package is lost for good. There is no recovery, no \"I forgot my password\".": "Dacă o pierzi, pachetul e pierdut definitiv. Nu există recuperare, nu există „am uitat parola”.",
	"At least 8 characters. A long sentence you remember beats a short, complicated string.":           "Cel puțin 8 caractere. O propoziție lungă pe care o ții minte bate un șir scurt și complicat.",
	"Passphrase: ":       "Fraza de acces: ",
	"Once more:  ":       "Încă o dată:    ",
	"They do not match.": "Nu se potrivesc.",
	"too many attempts":  "prea multe încercări",

	// ---- version -----------------------------------------------------------------------------
	"system: %s\n": "sistem: %s\n",
	"home:   %s\n": "acasă:  %s\n",
}
