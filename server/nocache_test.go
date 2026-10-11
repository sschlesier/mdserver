package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

var noCacheHeaders = map[string]string{
	"Cache-Control": "no-cache, no-store, must-revalidate",
	"Pragma":        "no-cache",
	"Expires":       "0",
}

func assertNoCache(t *testing.T, what string, code int, h http.Header) {
	t.Helper()
	for header, want := range noCacheHeaders {
		if got := h.Get(header); got != want {
			t.Errorf("%s (%d) %s = %q, want %q", what, code, header, got, want)
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

	requests := []struct{ method, path, rangeHeader string }{
		{http.MethodGet, "/", ""},
		{http.MethodGet, "/doc.md", ""},
		{http.MethodGet, "/doc", ""},
		{http.MethodGet, "/pic.png", ""},
		{http.MethodGet, "/sub", ""}, // redirect
		{http.MethodGet, "/sub/", ""},
		{http.MethodGet, "/settings", ""},
		{http.MethodGet, "/_mdserver/style.css", ""},
		{http.MethodGet, "/favicon.svg", ""},
		{http.MethodGet, "/favicon.ico", ""},
		{http.MethodGet, "/missing.md", ""},
		{http.MethodGet, "/notes.txt", ""},
		{http.MethodPost, "/settings", ""}, // 405
		{http.MethodPost, "/settings/remove-watch", ""},
		// ServeContent and ServeFile drop Cache-Control from their own error responses.
		{http.MethodGet, "/_mdserver/style.css", "bytes=999999-"},
		{http.MethodGet, "/pic.png", "bytes=999999-"},
	}
	for _, req := range requests {
		r := httptest.NewRequest(req.method, req.path, nil)
		if req.rangeHeader != "" {
			r.Header.Set("Range", req.rangeHeader)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if req.rangeHeader != "" && rec.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("%s %s with Range %s = %d, want 416", req.method, req.path, req.rangeHeader, rec.Code)
		}
		assertNoCache(t, req.method+" "+req.path, rec.Code, rec.Header())
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
	assertNoCache(t, "GET / with the root gone", rec.Code, rec.Header())
	<-srv.shutdownDone
}

// The live reload WebSocket handshake carries the no-cache headers, though the
// WebSocket library writes it itself.
func TestLiveReloadUpgradeIsNoCache(t *testing.T) {
	srv := NewServer(Config{Host: "localhost", RootDir: t.TempDir(), EnableLiveReload: true})
	t.Cleanup(srv.Stop)
	ts := httptest.NewServer(srv.httpServer.Handler)
	t.Cleanup(ts.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/livereload", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	assertNoCache(t, "GET /livereload upgrade", resp.StatusCode, resp.Header)
}

// The wrapper keeps Flush working; the shutdown page flushes before the process exits.
func TestNoCacheWriterFlushes(t *testing.T) {
	h := noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bye"))
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("wrapped writer isn't an http.Flusher")
		}
		f.Flush()
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/settings/shutdown", nil))
	if !rec.Flushed {
		t.Error("Flush didn't reach the underlying writer")
	}
}
