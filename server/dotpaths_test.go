package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDotSegmentsAreNotServed(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"docs/.env":        "SECRET",
		"docs/ok.md":       "# ok",
		"docs/pic.png":     "png",
		"notes/.secret.md": "# secret",
		".env":             "SECRET",
		".hidden/a.md":     "# a",
		".hidden/a":        "x",
		".hidden/sub/b.md": "# b",
		"visible.md":       "# visible",
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	s := NewServer(Config{Host: "localhost", RootDir: root})
	h := s.httpServer.Handler

	tests := []struct {
		path string
		want int
	}{
		{"/docs/.env", http.StatusNotFound},
		{"/notes/.secret.md", http.StatusNotFound},
		{"/notes/.secret", http.StatusNotFound},
		{"/.hidden/", http.StatusNotFound},
		{"/.hidden", http.StatusNotFound},
		{"/.hidden/sub/", http.StatusNotFound},
		{"/.hidden/a.md", http.StatusNotFound},
		{"/.hidden/a", http.StatusNotFound},
		{"/assets/docs/.env", http.StatusNotFound},
		{"/assets/.env", http.StatusNotFound},
		{"/assets/.hidden/a.md", http.StatusNotFound},
		{"/.env", http.StatusNotFound},
		{"/%2eenv", http.StatusNotFound},
		{"/docs/%2eenv", http.StatusNotFound},
		{"/notes/%2esecret.md", http.StatusNotFound},
		{"/%2ehidden/", http.StatusNotFound},
		{"/%2ehidden", http.StatusNotFound},
		{"/%2ehidden/a.md", http.StatusNotFound},
		{"/assets/docs/%2eenv", http.StatusNotFound},
		{"/assets/%2eenv", http.StatusNotFound},
		{"/.hidden/.", http.StatusTemporaryRedirect}, // mux redirects to /.hidden/, which is 404
		{"/%2ehidden%2fa.md", http.StatusNotFound},
		{"/docs%2f%2eenv", http.StatusNotFound},
		{"/", http.StatusOK},
		{"/visible.md", http.StatusOK},
		{"/visible", http.StatusOK},
		{"/docs/", http.StatusOK},
		{"/docs/ok.md", http.StatusOK},
		{"/docs/pic.png", http.StatusOK},
		{"/assets/docs/pic.png", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.want)
			}
		})
	}
}

// The mux cleans ".." before handlers run, so outside-root paths are checked directly.
func TestOutsideRootPathsStayForbidden(t *testing.T) {
	root := t.TempDir()
	s := NewServer(Config{Host: "localhost", RootDir: root})

	for _, rel := range []string{"../x", "../x.md", "../.env", "../../etc/passwd"} {
		p := filepath.Join(root, rel)
		if s.isValidPath(p) {
			t.Errorf("isValidPath(%q) = true, want false", rel)
		}
		if s.isHiddenPath(p) {
			t.Errorf("isHiddenPath(%q) = true, want false (outside root keeps 403)", rel)
		}
	}

	// A name that merely starts with ".." is a dot segment inside the root.
	p := filepath.Join(root, "..foo")
	if !s.isHiddenPath(p) {
		t.Errorf("isHiddenPath(..foo) = false, want true")
	}
	if s.isValidPath(p) {
		t.Errorf("isValidPath(..foo) = true, want false")
	}
}

func TestDotRootDirStillServed(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".notes")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# a"), 0644); err != nil {
		t.Fatal(err)
	}
	s := NewServer(Config{Host: "localhost", RootDir: root})
	for _, p := range []string{"/", "/a.md"} {
		rec := httptest.NewRecorder()
		s.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, rec.Code)
		}
	}
}
