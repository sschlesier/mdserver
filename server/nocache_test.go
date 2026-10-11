package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var noCacheHeaders = map[string]string{
	"Cache-Control": "no-cache, no-store, must-revalidate",
	"Pragma":        "no-cache",
	"Expires":       "0",
}

func assertNoCache(t *testing.T, method, path string, rec *httptest.ResponseRecorder) {
	t.Helper()
	for header, want := range noCacheHeaders {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s %s (%d) %s = %q, want %q", method, path, rec.Code, header, got, want)
		}
	}
}

// Every response tells the browser not to cache it, whatever its status.
func TestEveryResponseIsNoCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pic.png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Config{Host: "localhost", RootDir: root})
	h := srv.httpServer.Handler

	requests := []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/doc.md"},
		{http.MethodGet, "/doc"},
		{http.MethodGet, "/pic.png"},
		{http.MethodGet, "/sub"}, // redirect
		{http.MethodGet, "/sub/"},
		{http.MethodGet, "/settings"},
		{http.MethodGet, "/_mdserver/style.css"},
		{http.MethodGet, "/favicon.svg"},
		{http.MethodGet, "/favicon.ico"},
		{http.MethodGet, "/missing.md"},
		{http.MethodGet, "/notes.txt"},
		{http.MethodPost, "/settings"}, // 405
		{http.MethodPost, "/settings/remove-watch"},
	}
	for _, req := range requests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(req.method, req.path, nil))
		assertNoCache(t, req.method, req.path, rec)
	}

	// The folder-missing notice too.
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("GET / after removing the root = %d, want 410", rec.Code)
	}
	assertNoCache(t, http.MethodGet, "/", rec)
	<-srv.shutdownDone
}
