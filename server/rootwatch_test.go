package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func watchedDirsOf(srv *Server) []string {
	return srv.roots[0].liveReload.WatchedDirs()
}

func TestServingWatchesTheDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a", "b", "c.md"), "# C\n")
	writeTestFile(t, filepath.Join(root, "a", "d", "e", "f.md"), "# F\n")
	srv := NewServer(Config{Host: "localhost", RootDir: root, EnableLiveReload: true})
	t.Cleanup(srv.Stop)

	// Only the root and its children are watched until a page or listing is served.
	if slices.Contains(watchedDirsOf(srv), filepath.Join(root, "a", "b")) {
		t.Fatal("a/b is watched before it is served")
	}

	embedGet(t, srv.httpServer.Handler, "/a/b/c.md")
	if !slices.Contains(watchedDirsOf(srv), filepath.Join(root, "a", "b")) {
		t.Errorf("serving a/b/c.md did not watch a/b; watched: %v", watchedDirsOf(srv))
	}

	embedGet(t, srv.httpServer.Handler, "/a/d/e/")
	if !slices.Contains(watchedDirsOf(srv), filepath.Join(root, "a", "d", "e")) {
		t.Errorf("listing a/d/e/ did not watch it; watched: %v", watchedDirsOf(srv))
	}
}

func TestRemoveWatchHandler(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a", "b", "c.md"), "# C\n")
	srv := NewServer(Config{Host: "localhost", RootDir: root, EnableLiveReload: true})
	t.Cleanup(srv.Stop)
	embedGet(t, srv.httpServer.Handler, "/a/b/c.md")

	post := func(dir string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/settings/remove-watch", strings.NewReader(url.Values{"dir": {dir}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		return rec
	}

	sub := filepath.Join(root, "a", "b")
	if rec := post(sub); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings" {
		t.Errorf("remove-watch a/b = %d to %q, want 303 to /settings", rec.Code, rec.Header().Get("Location"))
	}
	if slices.Contains(watchedDirsOf(srv), sub) {
		t.Errorf("a/b is still watched after remove-watch")
	}

	if rec := post(root); rec.Code != http.StatusBadRequest {
		t.Errorf("remove-watch of the root = %d, want 400", rec.Code)
	}
	if !slices.Contains(watchedDirsOf(srv), root) {
		t.Errorf("the root is no longer watched after a refused remove-watch")
	}
}
