package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func searchFiles(t *testing.T, h http.Handler, query string) []fileResult {
	t.Helper()
	req := httptest.NewRequest("GET", "/_mdserver/files?q="+query, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("q=%q: status %d, body %q", query, rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	for name := range rec.Header() {
		if strings.HasPrefix(name, "Access-Control-") {
			t.Errorf("unexpected CORS header %s", name)
		}
	}
	var results []fileResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("q=%q: bad JSON %q: %v", query, rec.Body.String(), err)
	}
	return results
}

func resultPaths(rs []fileResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

func TestFilesEndpointFiltersToPageFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"a.md", "Upper.MD", "docs/guide.md", "docs/deep/er/note.md",
		"pic.png", "notes.txt", "x.html",
		".hidden.md", ".git/config.md", "docs/.private/secret.md", ".dot/inner.md",
	)
	if err := os.MkdirAll(filepath.Join(root, "dir.md"), 0755); err != nil {
		t.Fatal(err)
	}

	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	got := resultPaths(searchFiles(t, h, ""))
	want := []string{"Upper.MD", "a.md", "docs/deep/er/note.md", "docs/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestFilesEndpointLimitsResults(t *testing.T) {
	root := t.TempDir()
	for _, c := range "abcdefghijklmnop" {
		writeFiles(t, root, string(c)+"-note.md")
	}
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	if got := searchFiles(t, h, "note"); len(got) != 10 {
		t.Errorf("got %d results, want 10", len(got))
	}
	if got := searchFiles(t, h, ""); len(got) != 10 {
		t.Errorf("empty query: got %d results, want 10", len(got))
	}
}

func TestFilesEndpointResultShape(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "my docs/Read Me.md")
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler

	got := searchFiles(t, h, "readme")
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if got[0].Path != "my docs/Read Me.md" {
		t.Errorf("path = %q", got[0].Path)
	}
	if want := "/my%20docs/Read%20Me.md"; got[0].Href != want {
		t.Errorf("href = %q, want %q", got[0].Href, want)
	}
	if len(got[0].Matches) != len("readme") {
		t.Errorf("matches = %v", got[0].Matches)
	}

	// The href is a URL the server renders.
	req := httptest.NewRequest("GET", got[0].Href, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET %s = %d, want 200", got[0].Href, rec.Code)
	}

	if none := searchFiles(t, h, "zzzz"); len(none) != 0 {
		t.Errorf("no-match query returned %v", none)
	}
	empty := httptest.NewRecorder()
	h.ServeHTTP(empty, httptest.NewRequest("GET", "/_mdserver/files?q=zzzz", nil))
	if strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Errorf("no-match body = %q, want []", empty.Body.String())
	}
}

func TestFilesEndpointFindsFilesAddedLater(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "first.md")
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler

	writeFiles(t, root, "later/newcomer.md")
	got := resultPaths(searchFiles(t, h, "newcomer"))
	if want := []string{"later/newcomer.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilesEndpointRejectsOtherMethods(t *testing.T) {
	root := t.TempDir()
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/_mdserver/files?q=a", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", rec.Code)
	}
}

func TestFilesEndpointFollowsSymlinksAndStopsAtCycles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, root, "real/page.md")
	writeFiles(t, outside, "linked.md")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink(root, filepath.Join(root, "real", "loop")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}

	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	got := resultPaths(searchFiles(t, h, ""))
	want := []string{"link/linked.md", "real/page.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestFilesEndpointStopsAtWalkCap(t *testing.T) {
	old := maxWalkEntries
	maxWalkEntries = 5
	defer func() { maxWalkEntries = old }()

	root := t.TempDir()
	for _, c := range "abcdefghij" {
		writeFiles(t, root, string(c)+".md")
	}
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	if got := searchFiles(t, h, ""); len(got) != 5 {
		t.Errorf("got %d results with a cap of 5, want 5", len(got))
	}
}

func TestFilesEndpointSkipsUnreadableDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := t.TempDir()
	writeFiles(t, root, "open/a.md", "locked/b.md")
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0755)

	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	got := resultPaths(searchFiles(t, h, ""))
	if want := []string{"open/a.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestFilesEndpointListsADirectoryReachedThroughTwoPaths(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "real/page.md")
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "alias")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}

	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	got := resultPaths(searchFiles(t, h, ""))
	want := []string{"alias/page.md", "real/page.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}
