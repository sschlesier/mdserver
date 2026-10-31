package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newServerWithRoots builds a server with one root per directory. Production
// code serves a single root through NewServer; this exists so tests can check
// that roots stay apart before requests can choose among them.
func newServerWithRoots(t *testing.T, config Config, dirs ...string) *Server {
	t.Helper()
	s := newServer(config, dirs...)
	t.Cleanup(s.Stop)
	return s
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// dialRoot connects a live reload client to the root's own LiveReload.
func dialRoot(t *testing.T, rt *root) *websocket.Conn {
	t.Helper()
	if rt.liveReload == nil {
		t.Fatalf("root %s has no live reload", rt.dir)
	}
	ts := httptest.NewServer(http.HandlerFunc(rt.liveReload.HandleWebSocket))
	t.Cleanup(ts.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial live reload: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rt.liveReload.clientsMu.RLock()
		n := len(rt.liveReload.clients)
		rt.liveReload.clientsMu.RUnlock()
		if n == 1 {
			return conn
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("live reload client was not registered")
	return nil
}

func TestRootsReloadIndependently(t *testing.T) {
	for _, changed := range []int{0, 1} {
		name := []string{"A", "B"}[changed]
		t.Run("change in "+name, func(t *testing.T) {
			dirs := []string{t.TempDir(), t.TempDir()}
			writeTestFile(t, filepath.Join(dirs[0], "a.md"), "# A\n")
			writeTestFile(t, filepath.Join(dirs[1], "b.md"), "# B\n")

			srv := newServerWithRoots(t, Config{Host: "localhost", EnableLiveReload: true}, dirs...)
			if srv.roots[0].liveReload == srv.roots[1].liveReload {
				t.Fatal("roots share one LiveReload")
			}
			conns := []*websocket.Conn{dialRoot(t, srv.roots[0]), dialRoot(t, srv.roots[1])}

			file := []string{"a.md", "b.md"}[changed]
			writeTestFile(t, filepath.Join(dirs[changed], file), "# changed\n")

			conns[changed].SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, msg, err := conns[changed].ReadMessage(); err != nil || string(msg) != "reload" {
				t.Errorf("client of the changed root got %q, %v; want \"reload\"", msg, err)
			}
			conns[1-changed].SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			if _, msg, err := conns[1-changed].ReadMessage(); err == nil {
				t.Errorf("client of the other root got %q; want nothing", msg)
			}
		})
	}
}

func TestRootsResolveStayInsideTheirRoot(t *testing.T) {
	base := t.TempDir()
	dirA, dirB, outside := filepath.Join(base, "A"), filepath.Join(base, "B"), filepath.Join(base, "outside")
	for _, f := range []string{
		filepath.Join(dirA, "a.md"),
		filepath.Join(dirA, ".hidden.md"),
		filepath.Join(dirA, ".d", "x.md"),
		filepath.Join(dirA, "notes.txt"),
		filepath.Join(dirB, "b.md"),
		filepath.Join(dirB, ".hidden.md"),
		filepath.Join(dirB, ".d", "x.md"),
		filepath.Join(dirB, "notes.txt"),
		filepath.Join(outside, "ext.md"),
	} {
		writeTestFile(t, f, "# x\n")
	}
	// A symlink inside A to a directory elsewhere: followed today (accepted risk).
	if err := os.Symlink(outside, filepath.Join(dirA, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	srv := newServerWithRoots(t, Config{Host: "localhost"}, dirA, dirB)

	inside := func(rt *root, path string) bool {
		rel, err := filepath.Rel(rt.dir, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}

	for i, rt := range srv.roots {
		other := srv.roots[1-i]
		otherName := filepath.Base(other.dir)
		otherFile := map[string]string{"A": "a.md", "B": "b.md"}[otherName]

		// Inputs resolve() must reject outright.
		rejected := map[string]string{
			"dot-dot to other root file":     "/../" + otherName + "/" + otherFile,
			"dot-dot to other root dir":      "/../" + otherName + "/",
			"dot-dot to other root dir bare": "/../" + otherName,
			"dot-dot to parent":              "/../",
			"dot-dot twice":                  "/../../etc/hosts",
			"dot-dot inside path":            "/sub/../../" + otherName + "/" + otherFile,
			"decoded encoded dot-dot":        "/..%2f" + otherName + "/" + otherFile,
			"dotfile":                        "/.hidden.md",
			"file in dot-directory":          "/.d/x.md",
			"dot-directory":                  "/.d/",
			"non-markdown extension":         "/notes.txt",
		}
		for name, in := range rejected {
			path, _, status := rt.resolve(in)
			if status == http.StatusOK || status == http.StatusMovedPermanently {
				t.Errorf("%s: root %s resolve(%q) = %q (status %d), want it rejected", name, rt.dir, in, path, status)
			}
		}

		// Inputs that name the other root's files. A page path under this root is
		// "ok" without a stat (the handler answers 404 for a missing file), so the
		// check is that the result stays inside this root and is not the other's file.
		naming := map[string]string{
			"other root file by name":         "/" + otherFile,
			"other root file by absolute dir": other.dir + "/" + otherFile,
			"encoded dot-dot kept literally":  "/%2e%2e/" + otherName + "/" + otherFile,
		}
		for name, in := range naming {
			path, _, status := rt.resolve(in)
			if status != http.StatusOK && status != http.StatusMovedPermanently {
				continue
			}
			if !inside(rt, path) {
				t.Errorf("%s: root %s resolve(%q) = %q, outside the root", name, rt.dir, in, path)
			}
			if _, err := os.Stat(path); err == nil {
				t.Errorf("%s: root %s resolve(%q) = %q, which exists in this root", name, rt.dir, in, path)
			}
		}

		// Control: the root's own page resolves, so rejections above mean something.
		own := map[string]string{"A": "/a.md", "B": "/b.md"}[filepath.Base(rt.dir)]
		if path, kind, status := rt.resolve(own); status != http.StatusOK || kind != kindPage || !inside(rt, path) {
			t.Errorf("root %s resolve(%q) = %q, %d, %d; want its own page", rt.dir, own, path, kind, status)
		}
	}

	// A symlink inside the root to a directory elsewhere is followed (accepted risk).
	rootA := srv.roots[0]
	path, kind, status := rootA.resolve("/link/ext.md")
	if status != http.StatusOK || kind != kindPage {
		t.Fatalf("resolve(/link/ext.md) = %q, %d, %d; want the page through the symlink", path, kind, status)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	realOutside, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(real, realOutside+string(filepath.Separator)) {
		t.Errorf("symlinked page resolved to %q, want a file under %q", real, realOutside)
	}
}

func TestStopStopsEveryRoot(t *testing.T) {
	srv := newServerWithRoots(t, Config{Host: "localhost", EnableLiveReload: true}, t.TempDir(), t.TempDir())
	srv.Stop()
	for i, rt := range srv.roots {
		select {
		case <-rt.liveReload.stopChan:
		default:
			t.Errorf("root %d live reload was not stopped", i)
		}
	}
}
