package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twoRoots builds a multi-root server over fresh roots "a" and "b" and returns
// it with their directories.
func twoRoots(t *testing.T, config Config) (srv *Server, dirA, dirB string) {
	t.Helper()
	dirA, dirB = t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(dirA, "x.md"), "# A page\n")
	writeTestFile(t, filepath.Join(dirA, "sub", "b.md"), "# A sub\n")
	writeTestFile(t, filepath.Join(dirA, "i.png"), "png-a")
	writeTestFile(t, filepath.Join(dirA, "secret.txt"), "text")
	writeTestFile(t, filepath.Join(dirA, ".hidden", "h.md"), "# hidden\n")
	writeTestFile(t, filepath.Join(dirA, ".dot.md"), "# dot\n")
	writeTestFile(t, filepath.Join(dirB, "x.md"), "# B page\n")
	writeTestFile(t, filepath.Join(dirB, "only-b.md"), "# only b\n")

	config.Host = "localhost"
	config.Roots = []Root{{"a", dirA}, {"b", dirB}}
	srv = NewServer(config)
	t.Cleanup(srv.Stop)
	return srv, dirA, dirB
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func TestMultiRootRouting(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{})

	tests := []struct {
		path     string
		code     int
		location string
		body     string
	}{
		{"/a", 301, "/a/", ""},
		{"/b", 301, "/b/", ""},
		{"/a/", 200, "", "x.md"},
		{"/b/", 200, "", "only-b.md"},
		{"/a/x.md", 200, "", "A page"},
		{"/b/x.md", 200, "", "B page"},
		{"/a/sub/b.md", 200, "", "A sub"},
		{"/a/sub", 301, "/a/sub/", ""},
		{"/a/sub/", 200, "", "b.md"},
		{"/a/i.png", 200, "", "png-a"},
		{"/nope", 404, "", ""},
		{"/nope/x.md", 404, "", ""},
		{"/x.md", 404, "", ""},
		{"/_mdserver/other", 404, "", ""},
		{"/A/x.md", 404, "", ""},
	}
	for _, tt := range tests {
		rec := get(t, srv, tt.path)
		if rec.Code != tt.code {
			t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.code)
			continue
		}
		if tt.location != "" && rec.Header().Get("Location") != tt.location {
			t.Errorf("GET %s Location = %q, want %q", tt.path, rec.Header().Get("Location"), tt.location)
		}
		if tt.body != "" && !strings.Contains(rec.Body.String(), tt.body) {
			t.Errorf("GET %s body lacks %q", tt.path, tt.body)
		}
	}
}

func TestMultiRootReservedRoutesWin(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{})
	for _, path := range []string{"/favicon.svg", "/favicon.ico", "/_mdserver/style.css", "/_mdserver/picker.js", "/settings"} {
		if rec := get(t, srv, path); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
}

func TestMultiRootLandingListsRootsInOrder(t *testing.T) {
	dirs := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	config := Config{Host: "localhost", Roots: []Root{{"zeta", dirs[0]}, {"alpha", dirs[1]}, {"mid", dirs[2]}}}
	srv := NewServer(config)
	t.Cleanup(srv.Stop)

	rec := get(t, srv, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()
	last := -1
	for _, name := range []string{"zeta", "alpha", "mid"} {
		i := strings.Index(body, `href="/`+name+`/"`)
		if i < 0 {
			t.Fatalf("landing page has no link to /%s/", name)
		}
		if i < last {
			t.Errorf("root %s is out of config order", name)
		}
		last = i
	}
}

// TestMultiRootInvariants checks over HTTP that a root serves nothing outside
// its own directory and keeps the dotfile and file-type rules.
func TestMultiRootInvariants(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{})

	notServed := []string{
		"/a/../b/only-b.md",
		"/a/%2e%2e/b/only-b.md",
		"/a/%2E%2E/%2E%2E/etc/passwd",
		"/a/..%2fb/only-b.md",
		"/a/sub/../../b/only-b.md",
		"/a/only-b.md",
		"/b/x.md/../../a/x.md",
		"/a/.hidden/h.md",
		"/a/.hidden/",
		"/a/.dot.md",
		"/a/secret.txt",
		"/a/%2fetc/passwd",
	}
	for _, path := range notServed {
		rec := get(t, srv, path)
		body := rec.Body.String()
		if rec.Code == http.StatusOK && (strings.Contains(body, "only b") || strings.Contains(body, "hidden") || strings.Contains(body, "dot") || strings.Contains(body, "text") || strings.Contains(body, "root:")) {
			t.Errorf("GET %s = %d served content outside the rules: %.80q", path, rec.Code, body)
		}
		if rec.Code == http.StatusOK && !strings.Contains(path, "..") {
			t.Errorf("GET %s = 200, want an error", path)
		}
	}

	// Each root's listing shows only its own files.
	if body := get(t, srv, "/a/").Body.String(); strings.Contains(body, "only-b.md") {
		t.Error("listing of a shows b's file")
	}
	if body := get(t, srv, "/b/").Body.String(); strings.Contains(body, "sub") {
		t.Error("listing of b shows a's directory")
	}
	// Same file name in two roots resolves per root.
	if !strings.Contains(get(t, srv, "/a/x.md").Body.String(), "A page") || !strings.Contains(get(t, srv, "/b/x.md").Body.String(), "B page") {
		t.Error("x.md did not resolve per root")
	}
}

func TestMultiRootSymlinkOutsideStillRefusedForTraversal(t *testing.T) {
	srv, dirA, _ := twoRoots(t, Config{})
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "o.md"), "# outside\n")
	if err := os.Symlink(outside, filepath.Join(dirA, "link")); err != nil {
		t.Fatal(err)
	}
	// A symlink inside the root is followed (accepted risk) but a path that
	// climbs out of the symlink's parent is not.
	if rec := get(t, srv, "/a/link/o.md"); rec.Code != http.StatusOK {
		t.Errorf("GET /a/link/o.md = %d, want 200", rec.Code)
	}
	if rec := get(t, srv, "/a/link/../x.md"); rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "outside") {
		t.Error("traversal through a symlink served outside content")
	}
}

func TestSingleRootUnchangedByRoutingChanges(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a", "x.md"), "# X\n")
	srv := NewServer(Config{Host: "localhost", RootDir: dir})
	t.Cleanup(srv.Stop)

	if rec := get(t, srv, "/a/x.md"); rec.Code != http.StatusOK {
		t.Errorf("GET /a/x.md = %d, want 200", rec.Code)
	}
	if rec := get(t, srv, "/a"); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/a/" {
		t.Errorf("GET /a = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := get(t, srv, "/"); !strings.Contains(rec.Body.String(), `href="/a/"`) {
		t.Error("/ is not the directory listing")
	}
}
