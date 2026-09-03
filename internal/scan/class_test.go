package scan

import "testing"

func TestClassOf(t *testing.T) {
	cases := []struct {
		name string
		want Class
	}{
		// Already compressed -- we leave them alone.
		{"poza.jpg", Incompressible},
		{"POZA.JPEG", Incompressible},
		{"/home/x/Videos/film.mkv", Incompressible},
		{"melodie.flac", Incompressible},
		{"arhiva.tar.gz", Incompressible},
		{"raport.docx", Incompressible},
		{"carte.epub", Incompressible},
		{"manual.pdf", Incompressible},

		// Moderate.
		{"program.exe", Binary},
		{"libc.so", Binary},
		{"disc.iso", Binary},
		{"masina.qcow2", Binary},
		{"vechi.doc", Binary},
		{"font.ttf", Binary},

		// Highly compressible.
		{"note.txt", Text},
		{"main.go", Text},
		{"date.json", Text},
		{"jurnal.log", Text},
		{"dump.sql", Text},
		{"desen.svg", Text},
		{"captura.bmp", Text},
		{"sunet.wav", Text},

		// No extension.
		{"Makefile", Text},
		{"README", Text},
		{"/home/x/.bashrc", Text},
		{"/home/x/.gitignore", Text},
		{"ceva-ciudat", Unknown},
		{"date.xyzzy", Unknown},
	}
	for _, c := range cases {
		if got := ClassOf(c.name); got != c.want {
			t.Errorf("ClassOf(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassOfDoubleExtension(t *testing.T) {
	// ".bak" is not known, but the extension before it says what the file really is.
	if got := ClassOf("config.json.bak"); got != Text {
		t.Errorf("config.json.bak = %v, want Text", got)
	}
	if got := ClassOf("poza.jpg.orig"); got != Incompressible {
		t.Errorf("poza.jpg.orig = %v, want Incompressible", got)
	}
}

func TestClassCompressible(t *testing.T) {
	if Incompressible.Compressible() {
		t.Error("the incompressible class must not be compressed")
	}
	if !Text.Compressible() || !Binary.Compressible() {
		t.Error("text and binary must be compressed")
	}
	if Unknown.Compressible() {
		t.Error("unknown is decided by entropy, not by default")
	}
}

func TestIsSecret(t *testing.T) {
	secrets := []string{
		"/home/x/.ssh/id_rsa",
		"/home/x/.ssh/config",
		"/home/x/.gnupg/secring.gpg",
		"/home/x/.aws/credentials",
		"/home/x/proiect/.env",
		"/home/x/proiect/.env.local",
		"/home/x/server.pem",
		"/home/x/cert.pfx",
		"/home/x/parole.kdbx",
		"/home/x/.netrc",
		`C:\Users\x\.kube\config`,
	}
	for _, p := range secrets {
		if _, ok := IsSecret(p); !ok {
			t.Errorf("IsSecret(%q) = false, want true", p)
		}
	}

	safe := []string{
		"/home/x/Documents/raport.docx",
		"/home/x/environment.txt",
		"/home/x/env.md",
		"/home/x/poze/cheie.jpg",
		"/home/x/Documents/keynote.txt",
	}
	for _, p := range safe {
		if reason, ok := IsSecret(p); ok {
			t.Errorf("IsSecret(%q) = true (%s), want false", p, reason)
		}
	}
}

func TestExcluder(t *testing.T) {
	ex := DefaultExcluder()
	for _, dir := range []string{"node_modules", "NODE_MODULES", ".cache", "steamapps", ".git"} {
		if _, ok := ex.Dir(dir); !ok {
			t.Errorf("%q should be excluded by default", dir)
		}
	}
	if _, ok := ex.Dir("Documents"); ok {
		t.Error("Documents must not be excluded")
	}

	ex.Allow("node_modules")
	if _, ok := ex.Dir("node_modules"); ok {
		t.Error("Allow did not remove the rule")
	}
}
