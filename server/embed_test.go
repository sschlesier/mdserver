package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// assertFullTemplate checks that every line of the embedded template that holds no
// template action appears in body.
func assertFullTemplate(t *testing.T, path, body, name string) {
	t.Helper()
	for _, line := range strings.Split(string(mustReadTemplateFile(name)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "{{") || strings.Contains(line, "}}") {
			continue
		}
		if !strings.Contains(body, line) {
			t.Errorf("GET %s is missing %s line %q", path, name, line)
		}
	}
}

func TestServesEmbeddedResourcesWithoutTemplateDir(t *testing.T) {
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
	assertFullTemplate(t, "/sub/", embedGet(t, h, "/sub/").Body.String(), "directory.html")
	assertFullTemplate(t, "/", embedGet(t, h, "/").Body.String(), "directory.html")
	assertFullTemplate(t, "/sub/doc.md", embedGet(t, h, "/sub/doc.md").Body.String(), "page.html")
	assertFullTemplate(t, "/settings", embedGet(t, h, "/settings").Body.String(), "settings.html")

	css := embedGet(t, h, "/_mdserver/style.css")
	if css.Body.String() != string(styleCSS) {
		t.Error("GET /_mdserver/style.css doesn't serve the embedded stylesheet")
	}
	if ct := css.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("stylesheet Content-Type = %q", ct)
	}
	if cc := css.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
		t.Errorf("stylesheet Cache-Control = %q", cc)
	}

	for _, path := range []string{"/favicon.svg", "/favicon.ico"} {
		fav := embedGet(t, h, path)
		if fav.Body.String() != string(faviconSVG) {
			t.Errorf("GET %s doesn't serve the embedded favicon", path)
		}
		if ct := fav.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Errorf("GET %s Content-Type = %q", path, ct)
		}
		if cc := fav.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
			t.Errorf("GET %s Cache-Control = %q", path, cc)
		}
	}
}

func TestIgnoresTemplateDirInWorkingDirectory(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)
	decoy := "mdserver-decoy-template"
	files := map[string]string{
		"page.html":      "<html><body>" + decoy + "</body></html>",
		"directory.html": "<html><body>" + decoy + "</body></html>",
		"settings.html":  "<html><body>" + decoy + "</body></html>",
		"style.css":      "/* " + decoy + " */",
		"favicon.svg":    "<svg><!-- " + decoy + " --></svg>",
	}
	if err := os.Mkdir(filepath.Join(wd, "template"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(wd, "template", name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	h := newEmbedTestServer(t)

	for _, path := range []string{"/", "/sub/doc.md", "/settings", "/_mdserver/style.css", "/favicon.svg"} {
		if body := embedGet(t, h, path).Body.String(); strings.Contains(body, decoy) {
			t.Errorf("GET %s served the working directory's template/ file", path)
		}
	}
	assertFullTemplate(t, "/sub/doc.md", embedGet(t, h, "/sub/doc.md").Body.String(), "page.html")
	if body := embedGet(t, h, "/_mdserver/style.css").Body.String(); body != string(styleCSS) {
		t.Error("GET /_mdserver/style.css doesn't serve the embedded stylesheet")
	}
}
