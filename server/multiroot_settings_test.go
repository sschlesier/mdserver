package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func postForm(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func TestMultiRootSettingsGroupWatchesByRoot(t *testing.T) {
	srv, _, dirB := twoRoots(t, Config{EnableLiveReload: true})
	writeTestFile(t, filepath.Join(dirB, "deep", "d.md"), "# D\n")
	get(t, srv, "/b/deep/") // starts watching b/deep

	body := get(t, srv, "/settings").Body.String()
	ia := strings.Index(body, `<h3 class="watched-root-name">a</h3>`)
	ib := strings.Index(body, `<h3 class="watched-root-name">b</h3>`)
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("settings does not group by root name in config order: %s", body)
	}
	groupA, groupB := body[ia:ib], body[ib:]
	if !strings.Contains(groupA, "sub") || strings.Contains(groupA, "deep") {
		t.Errorf("group a lists the wrong directories: %s", groupA)
	}
	if !strings.Contains(groupB, "deep") || strings.Contains(groupB, ">sub<") {
		t.Errorf("group b lists the wrong directories: %s", groupB)
	}
	if !strings.Contains(groupB, `name="root" value="b"`) {
		t.Error("remove forms in group b do not name root b")
	}
}

func TestMultiRootRemoveWatchIsScopedToTheNamedRoot(t *testing.T) {
	srv, dirA, dirB := twoRoots(t, Config{EnableLiveReload: true})
	subA := filepath.Join(dirA, "sub")
	deepB := filepath.Join(dirB, "deep")
	writeTestFile(t, filepath.Join(deepB, "d.md"), "# D\n")
	get(t, srv, "/b/deep/")
	watched := func(i int) []string { return srv.roots[i].liveReload.WatchedDirs() }

	if !slices.Contains(watched(0), subA) || !slices.Contains(watched(1), deepB) {
		t.Fatalf("setup: a watches %v, b watches %v", watched(0), watched(1))
	}

	// A directory of another root cannot be removed through the wrong root.
	if rec := postForm(t, srv, "/settings/remove-watch", url.Values{"root": {"a"}, "dir": {deepB}}); rec.Code != http.StatusBadRequest {
		t.Errorf("removing b's directory via root a = %d, want 400", rec.Code)
	}
	if !slices.Contains(watched(1), deepB) {
		t.Error("b lost its watch through root a")
	}
	// Unknown or missing root.
	for _, form := range []url.Values{{"dir": {subA}}, {"root": {"zzz"}, "dir": {subA}}} {
		if rec := postForm(t, srv, "/settings/remove-watch", form); rec.Code != http.StatusBadRequest {
			t.Errorf("remove-watch %v = %d, want 400", form, rec.Code)
		}
	}
	if !slices.Contains(watched(0), subA) {
		t.Error("a lost its watch on a bad request")
	}
	// A root's top-level watch cannot be removed.
	if rec := postForm(t, srv, "/settings/remove-watch", url.Values{"root": {"a"}, "dir": {dirA}}); rec.Code != http.StatusBadRequest {
		t.Errorf("removing a's top level = %d, want 400", rec.Code)
	}
	if !slices.Contains(watched(0), dirA) {
		t.Error("a's top-level watch was removed")
	}
	// The right root removes it.
	rec := postForm(t, srv, "/settings/remove-watch", url.Values{"root": {"a"}, "dir": {subA}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings" {
		t.Errorf("remove-watch = %d to %q, want 303 to /settings", rec.Code, rec.Header().Get("Location"))
	}
	if slices.Contains(watched(0), subA) {
		t.Error("a still watches sub")
	}
	if !slices.Contains(watched(1), deepB) {
		t.Error("b's watch changed")
	}
}

func TestMultiRootMissingRootIs410AndOthersKeepServing(t *testing.T) {
	for _, liveReload := range []bool{false, true} {
		srv, _, dirB := twoRoots(t, Config{EnableLiveReload: liveReload})
		if err := os.RemoveAll(dirB); err != nil {
			t.Fatal(err)
		}

		for _, path := range []string{"/b/", "/b/x.md", "/b/sub/", "/_mdserver/files?root=b&q="} {
			if rec := get(t, srv, path); rec.Code != http.StatusGone {
				t.Errorf("live reload %v: GET %s = %d, want 410", liveReload, path, rec.Code)
			}
		}
		if body := get(t, srv, "/b/x.md").Body.String(); !strings.Contains(body, "Folder missing") || !strings.Contains(body, dirB) {
			t.Errorf("410 page does not name the folder: %s", body)
		}
		if rec := get(t, srv, "/a/x.md"); rec.Code != http.StatusOK {
			t.Errorf("GET /a/x.md = %d, want 200", rec.Code)
		}

		landing := get(t, srv, "/").Body.String()
		if !strings.Contains(landing, `href="/a/"`) || strings.Contains(landing, `href="/b/"`) || !strings.Contains(landing, "unavailable") {
			t.Errorf("landing page does not mark b unavailable: %s", landing)
		}

		select {
		case <-srv.rootGone:
			t.Error("the server shut down because a root vanished")
		default:
		}
	}
}

func TestMultiRootStaysUpWhenEveryRootVanishes(t *testing.T) {
	srv, dirA, dirB := twoRoots(t, Config{EnableLiveReload: true})
	os.RemoveAll(dirA)
	os.RemoveAll(dirB)

	for _, path := range []string{"/a/", "/b/"} {
		if rec := get(t, srv, path); rec.Code != http.StatusGone {
			t.Errorf("GET %s = %d, want 410", path, rec.Code)
		}
	}
	if rec := get(t, srv, "/"); rec.Code != http.StatusOK {
		t.Errorf("GET / = %d, want 200", rec.Code)
	}
	select {
	case <-srv.rootGone:
		t.Error("the server shut down after every root vanished")
	default:
	}
}

func TestMultiRootRevivesOnRequestAfterTheFolderReturns(t *testing.T) {
	srv, _, dirB := twoRoots(t, Config{})
	os.RemoveAll(dirB)
	if rec := get(t, srv, "/b/"); rec.Code != http.StatusGone {
		t.Fatalf("GET /b/ = %d, want 410", rec.Code)
	}
	writeTestFile(t, filepath.Join(dirB, "x.md"), "# back\n")
	if rec := get(t, srv, "/b/x.md"); rec.Code != http.StatusOK {
		t.Errorf("GET /b/x.md after the folder returned = %d, want 200", rec.Code)
	}
}
