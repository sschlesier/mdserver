package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newEmbedTestServer serves a root holding sub/doc.md, with live reload on so the settings
// page lists watched directories, from a working directory with no template/ folder.
func newEmbedTestServer(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "doc.md"), []byte("# Doc\n\nHello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Config{Host: "localhost", RootDir: root, EnableLiveReload: true})
	t.Cleanup(srv.Stop)
	return srv.httpServer.Handler
}

func embedGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", path, rec.Code)
	}
	return rec
}

// readSourceFiles reads server/template/ from disk, independently of the embedded copies,
// as the expected content. Call it before changing directory.
func readSourceFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, name := range []string{"page.html", "directory.html", "settings.html", "style.css", "favicon.svg"} {
		b, err := os.ReadFile(filepath.Join("template", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(b)
	}
	return files
}

// assertFullTemplate checks that every line of the source template that holds no template
// action appears in body.
func assertFullTemplate(t *testing.T, path, body, src string) {
	t.Helper()
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "{{") || strings.Contains(line, "}}") {
			continue
		}
		if !strings.Contains(body, line) {
			t.Errorf("GET %s is missing template line %q", path, line)
		}
	}
}

func TestServesEmbeddedResourcesWithoutTemplateDir(t *testing.T) {
	src := readSourceFiles(t)
	t.Chdir(t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".", filepath.Dir(exe)} {
		if _, err := os.Stat(filepath.Join(dir, "template")); err == nil {
			t.Fatalf("%s has a template/ folder; the test needs it absent", dir)
		}
	}

	h := newEmbedTestServer(t)

	// Visiting sub/ first makes it a removable watch, so settings renders every branch.
	assertFullTemplate(t, "/sub/", embedGet(t, h, "/sub/").Body.String(), src["directory.html"])
	assertFullTemplate(t, "/", embedGet(t, h, "/").Body.String(), src["directory.html"])
	assertFullTemplate(t, "/sub/doc.md", embedGet(t, h, "/sub/doc.md").Body.String(), src["page.html"])
	assertFullTemplate(t, "/settings", embedGet(t, h, "/settings").Body.String(), src["settings.html"])

	css := embedGet(t, h, "/_mdserver/style.css")
	if css.Body.String() != src["style.css"] {
		t.Error("GET /_mdserver/style.css doesn't serve the embedded stylesheet")
	}
	if ct := css.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("stylesheet Content-Type = %q", ct)
	}

	for _, path := range []string{"/favicon.svg", "/favicon.ico"} {
		fav := embedGet(t, h, path)
		if fav.Body.String() != src["favicon.svg"] {
			t.Errorf("GET %s doesn't serve the embedded favicon", path)
		}
		if ct := fav.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Errorf("GET %s Content-Type = %q", path, ct)
		}
	}
}

const decoy = "mdserver-decoy-template"

// writeDecoys creates dir/template/ holding a decoy for every embedded file.
func writeDecoys(dir string) error {
	files := map[string]string{
		"page.html":      "<html><body>" + decoy + "</body></html>",
		"directory.html": "<html><body>" + decoy + "</body></html>",
		"settings.html":  "<html><body>" + decoy + "</body></html>",
		"style.css":      "/* " + decoy + " */",
		"favicon.svg":    "<svg><!-- " + decoy + " --></svg>",
	}
	if err := os.Mkdir(filepath.Join(dir, "template"), 0755); err != nil {
		return err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, "template", name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// assertDecoysIgnored checks that no page, the stylesheet or the favicon serves a decoy.
func assertDecoysIgnored(t *testing.T, src map[string]string, where string) {
	t.Helper()
	h := newEmbedTestServer(t)
	for _, path := range []string{"/", "/sub/doc.md", "/settings", "/_mdserver/style.css", "/favicon.svg", "/favicon.ico"} {
		if body := embedGet(t, h, path).Body.String(); strings.Contains(body, decoy) {
			t.Errorf("GET %s served the template/ file %s", path, where)
		}
	}
	assertFullTemplate(t, "/sub/doc.md", embedGet(t, h, "/sub/doc.md").Body.String(), src["page.html"])
	if body := embedGet(t, h, "/_mdserver/style.css").Body.String(); body != src["style.css"] {
		t.Error("GET /_mdserver/style.css doesn't serve the embedded stylesheet")
	}
}

func TestIgnoresTemplateDirInWorkingDirectory(t *testing.T) {
	src := readSourceFiles(t)
	wd := t.TempDir()
	t.Chdir(wd)
	if err := writeDecoys(wd); err != nil {
		t.Fatal(err)
	}
	assertDecoysIgnored(t, src, "in the working directory")
}

func TestIgnoresTemplateDirBesideBinary(t *testing.T) {
	src := readSourceFiles(t)
	t.Chdir(t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeDir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(exeDir, "template")); err == nil {
		t.Fatalf("%s already has a template/ folder", exeDir)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Join(exeDir, "template")) })
	if err := writeDecoys(exeDir); err != nil {
		t.Skipf("can't write beside the test binary: %v", err)
	}
	assertDecoysIgnored(t, src, "beside the binary")
}

// Pages carry the live reload client script only when live reload is on.
func TestLiveReloadScriptFollowsConfig(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc\n"), 0644); err != nil {
			t.Fatal(err)
		}
		srv := NewServer(Config{Host: "localhost", RootDir: root, EnableLiveReload: enabled})
		t.Cleanup(srv.Stop)
		for _, path := range []string{"/", "/doc.md", "/settings"} {
			body := embedGet(t, srv.httpServer.Handler, path).Body.String()
			if got := strings.Contains(body, "/livereload"); got != enabled {
				t.Errorf("live reload %v: GET %s has the client script = %v", enabled, path, got)
			}
		}
	}
}

// The stylesheet and favicon carry the binary's modification time as Last-Modified, and
// answer a matching If-Modified-Since with 304.
func TestEmbeddedFilesUseBinaryModTime(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate the test binary so its mtime can't be mistaken for the server's start time.
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(exe, time.Time{}, old); err != nil {
		t.Skipf("can't change the test binary's mtime: %v", err)
	}
	t.Cleanup(func() { os.Chtimes(exe, time.Time{}, info.ModTime()) })
	want := old.Format(http.TimeFormat)

	h := newEmbedTestServer(t)
	for _, path := range []string{"/_mdserver/style.css", "/favicon.svg"} {
		if got := embedGet(t, h, path).Header().Get("Last-Modified"); got != want {
			t.Errorf("GET %s Last-Modified = %q, want %q", path, got, want)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-Modified-Since", want)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotModified {
			t.Errorf("GET %s with If-Modified-Since = %d, want 304", path, rec.Code)
		}
	}
}
