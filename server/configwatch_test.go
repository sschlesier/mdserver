package server

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// reloadFixture is a multi-root server over roots a and b, started from a
// config file in its own directory and watching that file.
type reloadFixture struct {
	srv        *Server
	ts         *httptest.Server
	cfg        string
	a, b, c    string
	startPort  int
	lastWrites int
}

func newReloadFixture(t *testing.T, configPort int) *reloadFixture {
	t.Helper()
	old := configDebounce
	configDebounce = 40 * time.Millisecond
	t.Cleanup(func() { configDebounce = old })

	f := &reloadFixture{a: t.TempDir(), b: t.TempDir(), c: t.TempDir(), startPort: configPort}
	writeTestFile(t, filepath.Join(f.a, "x.md"), "# A\n")
	writeTestFile(t, filepath.Join(f.b, "x.md"), "# B\n")
	writeTestFile(t, filepath.Join(f.c, "x.md"), "# C\n")
	f.cfg = filepath.Join(t.TempDir(), "roots")
	f.write(t, f.valid("a", "b"))

	cfg, err := LoadRootsConfig(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.srv = NewServer(Config{Host: "localhost", Roots: cfg.Roots, EnableLiveReload: true})
	if err := f.srv.WatchConfig(f.cfg, cfg.Port); err != nil {
		t.Fatal(err)
	}
	f.ts = httptest.NewServer(f.srv.httpServer.Handler)
	t.Cleanup(f.ts.Close)
	t.Cleanup(f.srv.Stop)
	return f
}

// valid returns config text for the named roots (a, b or c) plus the port line.
func (f *reloadFixture) valid(names ...string) string {
	dirs := map[string]string{"a": f.a, "b": f.b, "c": f.c}
	var sb strings.Builder
	if f.startPort != 0 {
		fmt.Fprintf(&sb, "port %d\n", f.startPort)
	}
	for _, n := range names {
		fmt.Fprintf(&sb, "%s %s\n", n, dirs[n])
	}
	return sb.String()
}

// write saves the config in place.
func (f *reloadFixture) write(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(f.cfg, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeRename saves the config the way vim does: a temp file renamed over it.
func (f *reloadFixture) writeRename(t *testing.T, content string) {
	t.Helper()
	tmp := f.cfg + ".swp-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, f.cfg); err != nil {
		t.Fatal(err)
	}
}

func (f *reloadFixture) get(path string) (int, string) {
	rec := httptest.NewRecorder()
	f.srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil))
	return rec.Code, rec.Body.String()
}

func (f *reloadFixture) dial(t *testing.T, root string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.ts.URL, "http")+"/livereload?root="+root, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", root, err)
	}
	t.Cleanup(func() { conn.Close() })
	rt := f.srv.rootNamed(root)
	eventually(t, "the socket on "+root+" registers", func() bool { return clientCount(rt) == 1 })
	return conn
}

func clientCount(rt *root) int {
	rt.liveReload.clientsMu.RLock()
	defer rt.liveReload.clientsMu.RUnlock()
	return len(rt.liveReload.clients)
}

// eventually polls cond until it holds, failing after 2 seconds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("within 2s: %s", what)
}

// socketCloses reports whether the server closes conn within 2 seconds.
func socketCloses(conn *websocket.Conn) bool {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		return false
	}
	ne, isNet := err.(interface{ Timeout() bool })
	return !(isNet && ne.Timeout())
}

func (f *reloadFixture) serves(path string) bool {
	code, _ := f.get(path)
	return code == http.StatusOK
}

func (f *reloadFixture) landingContains(s string) bool {
	_, body := f.get("/")
	return strings.Contains(body, s)
}

// reloadLogCapture collects log output for the test.
type reloadLogCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *reloadLogCapture) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *reloadLogCapture) count(s string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.buf.String(), s)
}

func captureReloadLog(t *testing.T) *reloadLogCapture {
	t.Helper()
	lc := &reloadLogCapture{}
	log.SetOutput(lc)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return lc
}

func TestConfigReloadAddsARootWithoutDisturbingOthers(t *testing.T) {
	f := newReloadFixture(t, 0)
	rtA := f.srv.rootNamed("a")
	connA := f.dial(t, "a")
	_ = connA

	f.write(t, f.valid("a", "b", "c"))
	eventually(t, "/c/x.md serves", func() bool { return f.serves("/c/x.md") })
	if !f.landingContains("c/") {
		t.Error("/ does not list c")
	}
	if f.srv.rootNamed("a") != rtA || clientCount(rtA) != 1 {
		t.Error("root a was disturbed by adding c")
	}
}

func TestConfigReloadRemovesARoot(t *testing.T) {
	f := newReloadFixture(t, 0)
	rtB := f.srv.rootNamed("b")
	connB := f.dial(t, "b")

	f.write(t, f.valid("a"))
	eventually(t, "/b/x.md is 404", func() bool { code, _ := f.get("/b/x.md"); return code == http.StatusNotFound })
	if !socketCloses(connB) {
		t.Error("b's live-reload socket stayed open")
	}
	select {
	case <-rtB.liveReload.stopChan:
	default:
		t.Error("b's watcher was not stopped")
	}
	if f.landingContains("b/") {
		t.Error("/ still lists b")
	}
}

func TestConfigReloadChangesARootsPath(t *testing.T) {
	f := newReloadFixture(t, 0)
	rtA := f.srv.rootNamed("a")
	connB := f.dial(t, "b")

	f.write(t, fmt.Sprintf("a %s\nb %s\n", f.a, f.c))
	eventually(t, "b serves the new directory", func() bool {
		_, body := f.get("/b/x.md")
		return strings.Contains(body, ">C<")
	})
	if !socketCloses(connB) {
		t.Error("the old b socket stayed open")
	}
	if f.srv.rootNamed("a") != rtA {
		t.Error("unchanged root a was replaced")
	}
}

func TestConfigReloadDetectsRenameSave(t *testing.T) {
	f := newReloadFixture(t, 0)
	f.writeRename(t, f.valid("a", "b", "c"))
	eventually(t, "/c/x.md serves after a rename save", func() bool { return f.serves("/c/x.md") })

	f.writeRename(t, f.valid("a", "b"))
	eventually(t, "/c/x.md is gone after a second rename save", func() bool { return !f.serves("/c/x.md") })
}

func TestConfigReloadFollowsASymlinkedConfig(t *testing.T) {
	f := newReloadFixture(t, 0)
	target := f.cfg
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Config{Host: "localhost", Roots: []Root{{"a", f.a}}})
	t.Cleanup(srv.Stop)
	if err := srv.WatchConfig(link, 0); err != nil {
		t.Fatal(err)
	}
	f.writeRename(t, f.valid("a", "b", "c"))
	eventually(t, "the symlinked config's target edit is noticed", func() bool {
		return srv.rootNamed("c") != nil
	})
}

func TestConfigReloadDebouncesABurst(t *testing.T) {
	f := newReloadFixture(t, 0)
	before := f.srv.reloadCount.Load()
	for i := 0; i < 6; i++ {
		f.write(t, f.valid("a", "b", "c")+fmt.Sprintf("# edit %d\n", i))
		time.Sleep(3 * time.Millisecond)
	}
	eventually(t, "the burst is applied", func() bool { return f.serves("/c/x.md") })
	time.Sleep(3 * configDebounce) // a second reload, if any, would have fired by now
	if n := f.srv.reloadCount.Load() - before; n != 1 {
		t.Errorf("a burst of writes caused %d reloads, want 1", n)
	}
}

func TestConfigReloadInvalidEditsChangeNothing(t *testing.T) {
	cases := []struct {
		name    string
		content func(f *reloadFixture) string
		want    string
	}{
		{"relative path", func(f *reloadFixture) string { return f.valid("a", "b") + "d ../x\n" }, "path must start with / or ~/"},
		{"missing directory", func(f *reloadFixture) string { return f.valid("a", "b") + "d /nonexistent-mdserver-dir\n" }, "path does not exist"},
		{"refused path", func(f *reloadFixture) string { return f.valid("a", "b") + "d /etc\n" }, "too broad"},
		{"bad name", func(f *reloadFixture) string { return f.valid("a", "b") + "Bad " + f.c + "\n" }, "invalid name"},
		{"duplicate name", func(f *reloadFixture) string { return f.valid("a", "b") + "a " + f.c + "\n" }, "duplicate name"},
		{"zero roots", func(f *reloadFixture) string { return "# nothing\n" }, "no roots defined"},
		{"empty file", func(f *reloadFixture) string { return "" }, "no roots defined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lc := captureReloadLog(t)
			f := newReloadFixture(t, 0)
			rtA, rtB := f.srv.rootNamed("a"), f.srv.rootNamed("b")

			// The same file also carries a valid change (adding c) that must not apply.
			content := tc.content(f)
			if !strings.Contains(content, "# nothing") && content != "" {
				content += "c " + f.c + "\n"
			}
			f.write(t, content)
			eventually(t, "the banner shows the error", func() bool { return f.landingContains(tc.want) })

			if f.srv.rootNamed("a") != rtA || f.srv.rootNamed("b") != rtB || f.srv.rootNamed("c") != nil || len(f.srv.rootList()) != 2 {
				t.Error("an invalid edit changed the served roots")
			}
			if !f.serves("/a/x.md") || !f.serves("/b/x.md") {
				t.Error("working roots stopped serving")
			}
			_, body := f.get("/")
			if !strings.Contains(body, f.cfg+":") {
				t.Errorf("banner lacks file:line: %s", body)
			}

			// Saving the same mistake again logs nothing new.
			f.write(t, content+"# again\n")
			f.srv.reloadConfig(f.cfg)
			if n := lc.count(tc.want); n != 1 {
				t.Errorf("error logged %d times, want 1", n)
			}

			f.write(t, f.valid("a", "b", "c"))
			eventually(t, "a valid save clears the banner and applies", func() bool {
				return f.serves("/c/x.md") && !f.landingContains(tc.want)
			})
		})
	}
}

func TestConfigReloadSurvivesTheFileVanishing(t *testing.T) {
	f := newReloadFixture(t, 0)
	rtA := f.srv.rootNamed("a")

	if err := os.Remove(f.cfg); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the banner shows the missing file", func() bool { return f.landingContains("no such file") })
	if f.srv.rootNamed("a") != rtA || !f.serves("/b/x.md") {
		t.Error("deleting the config changed the served roots")
	}

	f.write(t, f.valid("a", "b", "c"))
	eventually(t, "the recreated file is loaded", func() bool { return f.serves("/c/x.md") && !f.landingContains("no such file") })
}

func TestConfigReloadReportsAPortChangeOnce(t *testing.T) {
	lc := captureReloadLog(t)
	f := newReloadFixture(t, 8099)
	rtA := f.srv.rootNamed("a")
	addr := f.srv.httpServer.Addr

	f.startPort = 8100
	f.write(t, f.valid("a", "b"))
	eventually(t, "the notice appears", func() bool { return f.landingContains(portNoticeText) })
	f.write(t, f.valid("a", "b")+"# touch\n")
	f.srv.reloadConfig(f.cfg)
	if n := lc.count(portNoticeText); n != 1 {
		t.Errorf("port notice logged %d times, want 1", n)
	}
	if f.srv.rootNamed("a") != rtA || f.srv.httpServer.Addr != addr {
		t.Error("a port change altered the running server")
	}

	f.startPort = 8099
	f.write(t, f.valid("a", "b"))
	eventually(t, "restoring the port clears the notice", func() bool { return !f.landingContains(portNoticeText) })
}

func TestConfigReloadKeepsPortNoticeFree(t *testing.T) {
	f := newReloadFixture(t, 0)
	f.write(t, f.valid("a", "b", "c"))
	eventually(t, "c is added", func() bool { return f.serves("/c/x.md") })
	if f.landingContains(portNoticeText) {
		t.Error("a notice appeared though the config has no port line")
	}
}

func TestVanishedRootIsRevivedByARequest(t *testing.T) {
	f := newReloadFixture(t, 0)
	old := f.srv.rootNamed("b")
	if err := os.RemoveAll(f.b); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.get("/b/x.md"); code != http.StatusGone {
		t.Fatalf("GET /b/x.md = %d, want 410", code)
	}
	writeTestFile(t, filepath.Join(f.b, "x.md"), "# B again\n")
	if code, _ := f.get("/b/x.md"); code != http.StatusOK {
		t.Fatalf("GET /b/x.md after the folder returned = %d, want 200", code)
	}
	rt := f.srv.rootNamed("b")
	if rt == old || rt.liveReload == nil {
		t.Fatal("the root was not replaced by a fresh one with live reload")
	}
	conn := f.dial(t, "b")
	writeTestFile(t, filepath.Join(f.b, "x.md"), "# B changed\n")
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, msg, err := conn.ReadMessage(); err != nil || string(msg) != "reload" {
		t.Fatalf("revived root's socket got %q, %v; want reload", msg, err)
	}
}

func TestConfigReloadStopsAfterServerStop(t *testing.T) {
	f := newReloadFixture(t, 0)
	f.srv.Stop()
	f.srv.reloadConfig(f.cfg) // must not panic or start watchers
	f.write(t, f.valid("a", "b", "c"))
	f.srv.reloadConfig(f.cfg)
	if f.srv.rootNamed("c") != nil {
		t.Error("a reload after Stop added a root")
	}
}

func TestConfigReloadUnderConcurrentRequests(t *testing.T) {
	f := newReloadFixture(t, 0)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bad []string
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, p := range []string{"/", "/a/x.md", "/b/x.md", "/c/x.md", "/settings", "/_mdserver/files?root=b"} {
					if code, _ := f.get(p); code >= 500 {
						mu.Lock()
						bad = append(bad, fmt.Sprintf("%s = %d", p, code))
						mu.Unlock()
					}
				}
			}
		}()
	}
	for i := 0; i < 40; i++ {
		switch i % 4 {
		case 0:
			f.write(t, f.valid("a", "c"))
		case 1:
			f.writeRename(t, f.valid("a", "b", "c"))
		case 2:
			f.srv.reloadConfig(f.cfg)
		default:
			f.write(t, f.valid("a", "b"))
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	if len(bad) > 0 {
		t.Errorf("5xx during reloads: %v", bad[:min(len(bad), 5)])
	}
}
