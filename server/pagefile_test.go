package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectoryListingShowsOnlyPagesAndDirectories(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "page.md", "UPPER.MD", "pic.png", "notes.txt", "sub/inner.md")

	h := NewServer(Config{Host: "localhost", RootDir: root}).httpServer.Handler
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	for _, want := range []string{"page.md", "UPPER.MD", "sub/"} {
		if !strings.Contains(body, ">"+want+"<") {
			t.Errorf("listing is missing %s", want)
		}
	}
	for _, unwanted := range []string{"pic.png", "notes.txt"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("listing shows %s", unwanted)
		}
	}
}

func TestIsPageFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"a.md", true},
		{"docs/A.MD", true},
		{"b.Md", true},
		{"a.markdown", false},
		{"a.png", false},
		{"md", false},
		{"a.md.txt", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isPageFile(tt.name); got != tt.want {
			t.Errorf("isPageFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
