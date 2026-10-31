package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestMultiRootPagesAreRenderedWithTheirPrefix(t *testing.T) {
	srv, dirA, _ := twoRoots(t, Config{})
	writeTestFile(t, filepath.Join(dirA, "links.md"),
		"[x](/sub/b.md) ![](/i.png) [rel](sub/b.md) [abs](https://example.com/x) [proto](//cdn.example.com/y)\n")

	body := get(t, srv, "/a/links.md").Body.String()
	for _, want := range []string{`href="/a/sub/b.md"`, `src="/a/i.png"`, `href="sub/b.md"`, `href="https://example.com/x"`, `href="//cdn.example.com/y"`} {
		if !strings.Contains(body, want) {
			t.Errorf("multi-root page lacks %s", want)
		}
	}
}

func TestSingleRootPagesAreRenderedWithoutPrefix(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "links.md"), "[x](/sub/b.md) ![](/i.png)\n")
	srv := NewServer(Config{Host: "localhost", RootDir: dir})
	t.Cleanup(srv.Stop)

	body := get(t, srv, "/links.md").Body.String()
	if !strings.Contains(body, `href="/sub/b.md"`) || !strings.Contains(body, `src="/i.png"`) {
		t.Errorf("single-root page rewrote links: %.300s", body)
	}
}

func hrefs(body string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<a href="([^"]*)"`).FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestMultiRootListingsAndBreadcrumbsCarryPrefix(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{})

	top := hrefs(get(t, srv, "/a/").Body.String())
	for _, want := range []string{"/", "/a/", "/a/sub/", "/a/x.md"} {
		if !slices.Contains(top, want) {
			t.Errorf("/a/ has no link to %q; links: %v", want, top)
		}
	}
	// The ".." entry at a root's top level goes to the list of roots.
	if body := get(t, srv, "/a/").Body.String(); !strings.Contains(body, `<a href="/">../</a>`) {
		t.Errorf("/a/ has no .. entry linking to /: %s", body)
	}

	sub := hrefs(get(t, srv, "/a/sub/").Body.String())
	for _, want := range []string{"/", "/a/", "/a/sub/", "/a/sub/b.md"} {
		if !slices.Contains(sub, want) {
			t.Errorf("/a/sub/ has no link to %q; links: %v", want, sub)
		}
	}

	page := get(t, srv, "/a/sub/b.md").Body.String()
	crumbs := regexp.MustCompile(`(?s)<span class="breadcrumb-links">(.*?)</span>`).FindStringSubmatch(page)
	if crumbs == nil {
		t.Fatal("no breadcrumbs on the page")
	}
	got := hrefs(crumbs[1])
	want := []string{"/", "/a/", "/a/sub/", "/a/sub/b.md"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("breadcrumbs = %v, want %v", got, want)
	}
	if !strings.Contains(crumbs[1], ">a/</a>") {
		t.Errorf("root crumb text is not a/: %s", crumbs[1])
	}
}

func TestSingleRootTopLevelHasNoParentEntry(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "x.md"), "# X\n")
	srv := NewServer(Config{Host: "localhost", RootDir: dir})
	t.Cleanup(srv.Stop)
	if body := get(t, srv, "/").Body.String(); strings.Contains(body, "..") {
		t.Errorf("single-root top level has a .. entry: %s", body)
	}
}

func TestMultiRootPagesNameTheirRoot(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{})
	for path, want := range map[string]string{
		"/a/":         `data-root="a"`,
		"/b/x.md":     `data-root="b"`,
		"/a/sub/b.md": `data-root="a"`,
	} {
		if body := get(t, srv, path).Body.String(); !strings.Contains(body, want) {
			t.Errorf("GET %s lacks %s", path, want)
		}
	}
	// The landing page and the multi-root settings page belong to no root.
	for _, path := range []string{"/", "/settings"} {
		if body := get(t, srv, path).Body.String(); strings.Contains(body, "data-root") {
			t.Errorf("GET %s has data-root", path)
		}
	}
}

func TestMultiRootPickerSearchesOnlyItsRoot(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{})

	search := func(path string) (int, []fileResult) {
		rec := get(t, srv, path)
		var res []fileResult
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
		}
		return rec.Code, res
	}

	_, a := search("/_mdserver/files?root=a&q=")
	hrefsA := map[string]bool{}
	for _, r := range a {
		hrefsA[r.Href] = true
	}
	if !hrefsA["/a/x.md"] || !hrefsA["/a/sub/b.md"] || hrefsA["/b/only-b.md"] || len(a) != 2 {
		t.Errorf("root a results = %v", a)
	}

	_, b := search("/_mdserver/files?root=b&q=")
	hrefsB := map[string]bool{}
	for _, r := range b {
		hrefsB[r.Href] = true
	}
	if !hrefsB["/b/x.md"] || !hrefsB["/b/only-b.md"] || len(b) != 2 {
		t.Errorf("root b results = %v", b)
	}

	_, only := search("/_mdserver/files?root=b&q=only")
	if len(only) != 1 || only[0].Href != "/b/only-b.md" {
		t.Errorf("search for only in b = %v", only)
	}

	for _, path := range []string{"/_mdserver/files?q=x", "/_mdserver/files?root=nope&q=x", "/_mdserver/files?root=settings"} {
		if code, _ := search(path); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
}

func TestSingleRootPickerIgnoresRootParameter(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "x.md"), "# X\n")
	srv := NewServer(Config{Host: "localhost", RootDir: dir})
	t.Cleanup(srv.Stop)

	var res []fileResult
	rec := get(t, srv, "/_mdserver/files?root=whatever&q=x")
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || len(res) != 1 || res[0].Href != "/x.md" {
		t.Errorf("got %d %v %v", rec.Code, res, err)
	}
}

func TestMultiRootLiveReloadIsPerRoot(t *testing.T) {
	for _, changed := range []string{"a", "b"} {
		t.Run("change in "+changed, func(t *testing.T) {
			srv, dirA, dirB := twoRoots(t, Config{EnableLiveReload: true})
			ts := httptest.NewServer(srv.httpServer.Handler)
			t.Cleanup(ts.Close)

			dial := func(root string) *websocket.Conn {
				conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/livereload?root="+root, nil)
				if err != nil {
					t.Fatalf("dial %s: %v", root, err)
				}
				t.Cleanup(func() { conn.Close() })
				return conn
			}
			conns := map[string]*websocket.Conn{"a": dial("a"), "b": dial("b")}
			dirs := map[string]string{"a": dirA, "b": dirB}

			// Wait for both clients to register.
			deadline := time.Now().Add(2 * time.Second)
			for _, rt := range srv.roots {
				for time.Now().Before(deadline) {
					rt.liveReload.clientsMu.RLock()
					n := len(rt.liveReload.clients)
					rt.liveReload.clientsMu.RUnlock()
					if n == 1 {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
			}

			writeTestFile(t, filepath.Join(dirs[changed], "x.md"), "# changed\n")

			conns[changed].SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, msg, err := conns[changed].ReadMessage(); err != nil || string(msg) != "reload" {
				t.Fatalf("tab on %s got %q, %v; want reload", changed, msg, err)
			}
			other := map[string]string{"a": "b", "b": "a"}[changed]
			conns[other].SetReadDeadline(time.Now().Add(400 * time.Millisecond))
			if _, msg, err := conns[other].ReadMessage(); err == nil {
				t.Fatalf("tab on %s got %q after a change in %s", other, msg, changed)
			}
		})
	}
}

func TestMultiRootLiveReloadNeedsAKnownRoot(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{EnableLiveReload: true})
	for _, path := range []string{"/livereload", "/livereload?root=nope", "/livereload?root="} {
		if rec := get(t, srv, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestLiveReloadClientScriptCarriesTheRoot(t *testing.T) {
	srv, _, _ := twoRoots(t, Config{EnableLiveReload: true})
	if body := get(t, srv, "/a/x.md").Body.String(); !strings.Contains(body, "'/livereload' + query") || !strings.Contains(body, `data-root="a"`) {
		t.Error("page lacks the root-aware live reload client")
	}
	if body := get(t, srv, "/").Body.String(); strings.Contains(body, "/livereload") {
		t.Error("landing page carries the live reload client")
	}
}
