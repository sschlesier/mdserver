package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryWithoutTrailingSlashRedirects(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "sub", "b.md"), "# B\n")
	srv := NewServer(Config{Host: "localhost", RootDir: root})

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/sub/" {
		t.Errorf("GET /sub = %d to %q, want 301 to /sub/", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "b.md") {
		t.Errorf("GET /sub/ = %d, want a listing containing b.md", rec.Code)
	}
}
