package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseRootsConfigValid(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	a := filepath.Join(base, "a")
	spaced := filepath.Join(base, "my docs")
	mkdirs(t, a, spaced, filepath.Join(home, "notes"))

	data := "# comment\n\n   # indented comment\nport 8099\n" +
		"a " + a + "\n" +
		"  b-2_x \t " + spaced + "  \n" +
		"n ~/notes\n"
	cfg, err := ParseRootsConfig([]byte(data), "roots", home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8099 {
		t.Errorf("Port = %d, want 8099", cfg.Port)
	}
	want := []Root{
		{"a", a},
		{"b-2_x", spaced},
		{"n", filepath.Join(home, "notes")},
	}
	if len(cfg.Roots) != len(want) {
		t.Fatalf("Roots = %v, want %v", cfg.Roots, want)
	}
	for i := range want {
		if cfg.Roots[i] != want[i] {
			t.Errorf("Roots[%d] = %v, want %v", i, cfg.Roots[i], want[i])
		}
	}
}

func TestParseRootsConfigNoPortLine(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ParseRootsConfig([]byte("a "+dir+"\n"), "roots", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %d, want 0", cfg.Port)
	}
}

func TestParseRootsConfigErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	writeTestFile(t, file, "x")
	home := t.TempDir()

	tests := []struct {
		name string
		data string
		want string // "<line>: <substring>"
	}{
		{"uppercase name", "Docs " + dir, "1: invalid name"},
		{"name with dot", "a.b " + dir, "1: invalid name"},
		{"leading dash", "-a " + dir, "1: invalid name"},
		{"leading underscore", "_a " + dir, "1: invalid name"},
		{"reserved settings", "settings " + dir, "1: reserved name"},
		{"reserved livereload", "livereload " + dir, "1: reserved name"},
		{"_mdserver is refused", "_mdserver " + dir, "1: invalid name"},
		{"favicon.ico is refused", "favicon.ico " + dir, "1: invalid name"},
		{"duplicate", "a " + dir + "\na " + dir, "2: duplicate name"},
		{"no path", "a", "1: name \"a\" has no path"},
		{"relative path", "a x/y", "1: path must start with / or ~/"},
		{"dot-dot path", "\n\n\nbad ../x", "4: path must start with / or ~/"},
		{"env var path", "a $HOME/x", "1: path must start with / or ~/"},
		{"tilde user", "a ~bob/x", "1: path must start with / or ~/"},
		{"missing path", "a " + filepath.Join(dir, "nope"), "1: path does not exist"},
		{"file path", "a " + file, "1: path is not a directory"},
		{"bad port word", "port abc", "1: bad port"},
		{"port zero", "port 0", "1: bad port"},
		{"port too big", "port 65536", "1: bad port"},
		{"port empty", "port", "1: bad port"},
		{"port extra", "port 80 90", "1: bad port"},
		{"second port", "port 8000\na " + dir + "\nport 8001", "3: second port line"},
		{"no roots", "# only a comment\n", ":0: no roots defined"},
		{"port only", "port 8000", ":0: no roots defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRootsConfig([]byte(tt.data), "cfg", home)
			if err == nil {
				t.Fatal("want an error")
			}
			line, reason, _ := strings.Cut(strings.TrimPrefix(tt.want, ":"), ": ")
			if strings.HasPrefix(tt.want, ":0") {
				line, reason = "0", "no roots defined"
			}
			prefix := "cfg:" + line + ": "
			if !strings.HasPrefix(err.Error(), prefix) || !strings.Contains(err.Error(), reason) {
				t.Errorf("error = %q, want prefix %q containing %q", err, prefix, reason)
			}
		})
	}
}
