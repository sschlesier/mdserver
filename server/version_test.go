package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Document and directory pages show the version beside the settings icon.
func TestPagesShowVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	s := NewServer(Config{RootDir: root, Version: "v9.9.9"})
	for _, path := range []string{"/doc.md", "/sub/"} {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if want := `<span class="version">v9.9.9</span>`; !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s: body lacks %s", path, want)
		}
	}
}
