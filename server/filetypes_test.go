package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wantImageCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; form-action 'none'"

func TestOnlyMarkdownAndImagesAreServed(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a.md":             "# a",
		"upper.MD":         "# upper",
		"mixed.Md":         "# mixed",
		"img.png":          "png",
		"IMG.PNG":          "png",
		"photo.jpg":        "jpg",
		"photo.jpeg":       "jpeg",
		"anim.gif":         "gif",
		"draw.svg":         `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"pic.webp":         "webp",
		"only-root.png":    "png",
		"x.html":           "<p>x</p>",
		"x.htm":            "<p>x</p>",
		"x.css":            "body{}",
		"x.js":             "alert(1)",
		"data.json":        "{}",
		"notes.txt":        "notes",
		"doc.pdf":          "pdf",
		"main.go":          "package main",
		"icon.ico":         "ico",
		"Makefile":         "all:",
		"assets/logo.png":  "png",
		"assets/page.md":   "# page",
		"assets/x.js":      "alert(1)",
		"assets/style.css": "body{}",
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

	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler

	tests := []struct {
		path      string
		want      int
		wantType  string // Content-Type prefix; checked when set
		imageHdrs bool   // whether the image CSP and nosniff headers are expected
	}{
		// Markdown, rendered, in any case
		{"/a.md", http.StatusOK, "text/html", false},
		{"/a", http.StatusOK, "text/html", false},
		{"/upper.MD", http.StatusOK, "text/html", false},
		{"/mixed.Md", http.StatusOK, "text/html", false},

		// Images, with their MIME type and headers
		{"/img.png", http.StatusOK, "image/png", true},
		{"/IMG.PNG", http.StatusOK, "image/png", true},
		{"/photo.jpg", http.StatusOK, "image/jpeg", true},
		{"/photo.jpeg", http.StatusOK, "image/jpeg", true},
		{"/anim.gif", http.StatusOK, "image/gif", true},
		{"/draw.svg", http.StatusOK, "image/svg+xml", true},
		{"/pic.webp", http.StatusOK, "image/webp", true},

		// Everything else
		{"/x.html", http.StatusNotFound, "", false},
		{"/x.htm", http.StatusNotFound, "", false},
		{"/x.css", http.StatusNotFound, "", false},
		{"/x.js", http.StatusNotFound, "", false},
		{"/data.json", http.StatusNotFound, "", false},
		{"/notes.txt", http.StatusNotFound, "", false},
		{"/doc.pdf", http.StatusNotFound, "", false},
		{"/main.go", http.StatusNotFound, "", false},
		{"/icon.ico", http.StatusNotFound, "", false},
		{"/Makefile", http.StatusNotFound, "", false},

		// /assets/ is an ordinary folder under the root
		{"/assets/logo.png", http.StatusOK, "image/png", true},
		{"/assets/page.md", http.StatusOK, "text/html", false},
		{"/assets/page", http.StatusOK, "text/html", false},
		{"/assets/x.js", http.StatusNotFound, "", false},
		{"/assets/only-root.png", http.StatusNotFound, "", false},
		{"/assets/style.css", http.StatusNotFound, "", false}, // a root .css follows the allowlist
		{"/assets/", http.StatusOK, "text/html", false},

		// mdserver's own files and pages
		{"/", http.StatusOK, "text/html", false},
		{"/_mdserver/style.css", http.StatusOK, "text/css", false},
		{"/_mdserver/other.css", http.StatusNotFound, "", false},
		{"/favicon.svg", http.StatusOK, "image/svg+xml", false},
		{"/favicon.ico", http.StatusOK, "image/svg+xml", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("GET %s = %d, want %d", tt.path, rec.Code, tt.want)
			}
			if ct := rec.Header().Get("Content-Type"); tt.wantType != "" && !strings.HasPrefix(ct, tt.wantType) {
				t.Errorf("GET %s Content-Type = %q, want %q", tt.path, ct, tt.wantType)
			}
			csp := rec.Header().Get("Content-Security-Policy")
			nosniff := rec.Header().Get("X-Content-Type-Options")
			if tt.imageHdrs {
				if csp != wantImageCSP {
					t.Errorf("GET %s Content-Security-Policy = %q, want %q", tt.path, csp, wantImageCSP)
				}
				if nosniff != "nosniff" {
					t.Errorf("GET %s X-Content-Type-Options = %q, want nosniff", tt.path, nosniff)
				}
			} else if rec.Code == http.StatusOK && (csp != "" || nosniff != "") {
				// http.NotFound sets nosniff on every 404, so only successful responses are checked.
				t.Errorf("GET %s has image headers (CSP %q, X-Content-Type-Options %q), want none", tt.path, csp, nosniff)
			}
		})
	}
}

func TestStylesheetLivesOffAssets(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"assets/logo.png":  "png",
		"assets/style.css": "body{}",
		"assets/page.md":   "# page",
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	if rec := get("/assets/logo.png"); rec.Code != http.StatusOK || rec.Body.String() != "png" {
		t.Errorf("GET /assets/logo.png = %d %q, want the folder's file", rec.Code, rec.Body.String())
	}
	if rec := get("/assets/style.css"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /assets/style.css = %d, want 404 (a root .css isn't served)", rec.Code)
	}
	for _, path := range []string{"/assets/page.md", "/assets/", "/", "/settings"} {
		if body := get(path).Body.String(); !strings.Contains(body, `href="/_mdserver/style.css"`) {
			t.Errorf("GET %s doesn't link mdserver's stylesheet", path)
		}
	}
}

// The tests run from server/, so the pages above use the inline fallback templates; this
// covers the template files that ship with the binary.
func TestTemplateFilesLinkStylesheet(t *testing.T) {
	for _, name := range []string{"page.html", "directory.html", "settings.html"} {
		b, err := os.ReadFile(filepath.Join("..", "template", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `href="/_mdserver/style.css"`) {
			t.Errorf("template/%s doesn't link mdserver's stylesheet", name)
		}
	}
}

func TestMissingMarkdownIsPlainNotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte("all:"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler

	for _, p := range []string{"/Makefile", "/nope", "/nope.md", "/assets/foo"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
		if body := rec.Body.String(); body != "404 page not found\n" {
			t.Errorf("GET %s body = %q, want the plain 404", p, body)
		}
	}
}

// Outside the root is 403 before the missing-file and allowlist checks give 404.
func TestOutsideRootIsForbiddenBeforeAllowlist(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"outside.png", "outside.txt", "x.MD"} {
		if err := os.WriteFile(filepath.Join(base, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler

	for _, p := range []string{"/..%2foutside.png", "/..%2foutside.txt", "/..%2fmissing.png", "/..%2fx.MD", "/assets/..%2f..%2fx.MD"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s = %d, want 403", p, rec.Code)
		}
	}
}

func TestListingLinksToUppercaseMarkdownOpen(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"NOTES.MD", "sub/Readme.Md"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler

	for dir, link := range map[string]string{"/": "/NOTES.MD", "/sub/": "/sub/Readme.Md"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, dir, nil))
		if !strings.Contains(rec.Body.String(), `href="`+link+`"`) {
			t.Fatalf("listing %s has no link to %s", dir, link)
		}
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, link, nil))
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s = %d %q, want 200 text/html", link, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestMarkdownExtStrippedInAnyCase(t *testing.T) {
	for _, name := range []string{"notes.md", "notes.MD", "notes.Md"} {
		if got := extractTitle("no heading", name); got != "notes" {
			t.Errorf("extractTitle(%q) = %q, want notes", name, got)
		}
		crumbs := createBreadcrumbs("docs/" + name)
		if got := string(crumbs[len(crumbs)-1].Text); got != "notes" {
			t.Errorf("createBreadcrumbs(docs/%s) last = %q, want notes", name, got)
		}
	}
	if got := extractTitle("", "pic.png"); got != "pic.png" {
		t.Errorf("extractTitle(pic.png) = %q, want pic.png", got)
	}
}
