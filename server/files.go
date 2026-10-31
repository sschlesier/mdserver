package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// maxPickerResults is how many matches the file picker gets.
const maxPickerResults = 10

// maxWalkEntries bounds how many directory entries one picker request visits;
// tests lower it.
var maxWalkEntries = 50000

// pageFiles lists the root-relative, slash-separated paths of every page file under
// the root, skipping dot segments and following symlinks. The walk stops after
// maxWalkEntries entries and returns what it found.
func (rt *root) pageFiles() []string {
	var files []string
	visited := 0
	inProgress := map[string]bool{}

	var walk func(dir, rel string)
	walk = func(dir, rel string) {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || inProgress[real] {
			return
		}
		inProgress[real] = true
		defer delete(inProgress, real)

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if visited >= maxWalkEntries {
				return
			}
			visited++

			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			info, err := os.Stat(full)
			if err != nil {
				continue
			}
			entryRel := name
			if rel != "" {
				entryRel = rel + "/" + name
			}
			switch {
			case info.IsDir():
				walk(full, entryRel)
			case info.Mode().IsRegular() && isPageFile(name):
				files = append(files, entryRel)
			}
		}
	}
	walk(rt.dir, "")
	return files
}

// fileResult is one entry in the picker's JSON response.
type fileResult struct {
	Path    string `json:"path"`
	Href    string `json:"href"`
	Matches []int  `json:"matches"`
}

// handleFiles answers the file picker: the best fuzzy matches for q among the
// root's page files. It reads the directory tree on every request, so new
// files are found without being visited first.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Multi-root requests name their root; single-root requests ignore the parameter.
	rt := s.roots[0]
	if s.multi {
		if rt = s.rootNamed(r.URL.Query().Get("root")); rt == nil {
			http.NotFound(w, r)
			return
		}
	}

	matches := matchFiles(r.URL.Query().Get("q"), rt.pageFiles(), maxPickerResults)
	results := make([]fileResult, len(matches))
	for i, m := range matches {
		results[i] = fileResult{Path: m.Path, Href: rt.prefix + pageHref(m.Path), Matches: m.Matches}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// pageHref returns the URL path for a root-relative, slash-separated file path.
func pageHref(rel string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/" + strings.Join(parts, "/")
}
