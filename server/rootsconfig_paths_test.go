package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootPathFootguns(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "h", "me")
	mkdirs(t, filepath.Join(home, "docs"))
	link := filepath.Join(base, "link-to-home")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}

	refused := map[string]string{
		"filesystem root":           "/",
		"home":                      home,
		"home with ~/":              "~/",
		"parent of home":            filepath.Dir(home),
		"ancestor of home":          base,
		"symlink resolving to home": link,
	}
	for _, p := range []string{"/Users", "/home", "/etc", "/var", "/usr", "/bin", "/sbin", "/opt", "/private", "/private/var", "/private/etc", "/System", "/Library"} {
		if _, err := os.Stat(p); err == nil {
			refused[p] = p
		}
	}
	for name, path := range refused {
		t.Run("refuse "+name, func(t *testing.T) {
			_, err := ParseRootsConfig([]byte("a "+path+"\n"), "cfg", home)
			if err == nil || !strings.HasPrefix(err.Error(), "cfg:1: ") || !strings.Contains(err.Error(), "refusing") {
				t.Errorf("error = %v, want cfg:1: refusing ...", err)
			}
		})
	}

	t.Run("subdirectory of home is allowed", func(t *testing.T) {
		cfg, err := ParseRootsConfig([]byte("a ~/docs\n"), "cfg", home)
		if err != nil || len(cfg.Roots) != 1 {
			t.Errorf("got %v, %v; want one root", cfg, err)
		}
	})
	t.Run("subdirectory of a refused directory is allowed", func(t *testing.T) {
		// Temp directories sit under /var, /private or /tmp.
		cfg, err := ParseRootsConfig([]byte("a "+filepath.Join(home, "docs")+"\n"), "cfg", "/nonexistent-home")
		if err != nil || len(cfg.Roots) != 1 {
			t.Errorf("got %v, %v; want one root", cfg, err)
		}
	})
}

func TestConfigPath(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	defaultFile := filepath.Join(home, ".config", "mdserver", "roots")
	xdgFile := filepath.Join(xdg, "mdserver", "roots")

	t.Run("xdg file wins when it exists", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", xdg)
		writeTestFile(t, xdgFile, "")
		writeTestFile(t, defaultFile, "")
		got, err := ConfigPath("")
		if err != nil || got != xdgFile {
			t.Errorf("ConfigPath = %q, %v; want %q", got, err, xdgFile)
		}
	})
	t.Run("falls back to ~/.config when the xdg file is missing", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		got, err := ConfigPath("")
		if err != nil || got != defaultFile {
			t.Errorf("ConfigPath = %q, %v; want %q", got, err, defaultFile)
		}
	})
	t.Run("unset xdg uses ~/.config", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		got, err := ConfigPath("")
		if err != nil || got != defaultFile {
			t.Errorf("ConfigPath = %q, %v; want %q", got, err, defaultFile)
		}
	})
	t.Run("explicit path is used as given", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "mine")
		writeTestFile(t, f, "")
		got, err := ConfigPath(f)
		if err != nil || got != f {
			t.Errorf("ConfigPath = %q, %v; want %q", got, err, f)
		}
	})
	t.Run("missing names every path and creates nothing", func(t *testing.T) {
		emptyHome := t.TempDir()
		emptyXDG := t.TempDir()
		t.Setenv("HOME", emptyHome)
		t.Setenv("XDG_CONFIG_HOME", emptyXDG)
		_, err := ConfigPath("")
		if err == nil {
			t.Fatal("want an error")
		}
		for _, p := range []string{filepath.Join(emptyXDG, "mdserver", "roots"), filepath.Join(emptyHome, ".config", "mdserver", "roots")} {
			if !strings.Contains(err.Error(), p) {
				t.Errorf("error %q does not name %s", err, p)
			}
		}
		for _, d := range []string{emptyHome, emptyXDG} {
			entries, _ := os.ReadDir(d)
			if len(entries) != 0 {
				t.Errorf("%s was written to: %v", d, entries)
			}
		}
	})
	t.Run("missing explicit path is named", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope")
		_, err := ConfigPath(missing)
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("error = %v, want it to name %s", err, missing)
		}
	})
}
