package server

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// A ".." entry inside a root's subdirectory points at the parent under the
// root's prefix, and at the root's own top level from the first level down.
func TestMultiRootParentEntryCarriesPrefix(t *testing.T) {
	srv, dirA, _ := twoRoots(t, Config{})
	writeTestFile(t, dirA+"/sub/deeper/d.md", "# D\n")

	for path, want := range map[string]string{
		"/a/sub/":        `<a href="/a/">../</a>`,
		"/a/sub/deeper/": `<a href="/a/sub/">../</a>`,
	} {
		if body := get(t, srv, path).Body.String(); !strings.Contains(body, want) {
			t.Errorf("GET %s lacks %s: %s", path, want, body)
		}
	}
}

// The watcher alone marks a vanished root unavailable: a folder removed and
// put back before any request arrives stays unavailable until restart.
func TestMultiRootWatcherMarksRootUnavailable(t *testing.T) {
	srv, _, dirB := twoRoots(t, Config{EnableLiveReload: true})
	rtB := srv.rootNamed("b")

	if err := os.RemoveAll(dirB); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !rtB.gone.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !rtB.gone.Load() {
		t.Fatal("the watcher did not mark b unavailable")
	}

	writeTestFile(t, dirB+"/x.md", "# back\n")
	if rec := get(t, srv, "/b/x.md"); rec.Code != http.StatusGone {
		t.Errorf("GET /b/x.md after the folder returned = %d, want 410", rec.Code)
	}
}
