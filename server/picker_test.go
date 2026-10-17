package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPickerScriptIsServed(t *testing.T) {
	h := NewServer(Config{Host: "localhost", RootDir: t.TempDir()}).httpServer.Handler
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/_mdserver/picker.js", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/_mdserver/files") {
		t.Error("picker.js does not call the files endpoint")
	}
}

func TestPagesCarryPickerButtonAndScript(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "doc.md")

	for _, enableLiveReload := range []bool{false, true} {
		srv := NewServer(Config{Host: "localhost", RootDir: root, EnableLiveReload: enableLiveReload})
		h := srv.httpServer.Handler
		for _, path := range []string{"/doc.md", "/"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			body := rec.Body.String()
			if !strings.Contains(body, "data-picker-open") {
				t.Errorf("livereload=%v GET %s: no picker button", enableLiveReload, path)
			}
			if !strings.Contains(body, `src="/_mdserver/picker.js"`) {
				t.Errorf("livereload=%v GET %s: no picker script", enableLiveReload, path)
			}
		}
		srv.Stop()
	}
}

func TestSettingsPageHasNoPicker(t *testing.T) {
	h := NewServer(Config{Host: "localhost", RootDir: t.TempDir()}).httpServer.Handler
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/settings", nil))
	if strings.Contains(rec.Body.String(), "picker") {
		t.Error("settings page should not load the picker")
	}
}
