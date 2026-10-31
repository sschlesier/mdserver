package server

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mdserver/renderer"
)

// Breadcrumb represents a single breadcrumb navigation item
type Breadcrumb struct {
	Href string
	Text template.HTML
}

// handleMarkdown serves a markdown file as HTML
func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request, rt *root, filePath string) {
	log.Printf("markdown: %s", rt.relPath(filePath))
	if rt.liveReload != nil {
		rt.liveReload.EnsureWatching(filepath.Dir(filePath))
	}
	// Read markdown file
	content, err := os.ReadFile(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read file: %v", err), http.StatusNotFound)
		return
	}

	// Render markdown to HTML; root-relative links stay inside the root's prefix.
	htmlContent, err := renderer.RenderMarkdownWithBase(content, rt.prefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render markdown: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract title from first h1 or use filename
	title := extractTitle(string(content), filepath.Base(filePath))

	// Calculate relative path from root directory for breadcrumbs
	relPath, err := filepath.Rel(rt.dir, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}

	// Generate breadcrumbs
	breadcrumbs := rt.breadcrumbs(relPath)

	tmpl := s.templates.page

	data := struct {
		Title       string
		Content     template.HTML
		Breadcrumbs []Breadcrumb
		Root        string
	}{
		Title:       title,
		Content:     template.HTML(htmlContent),
		Breadcrumbs: breadcrumbs,
		Root:        rt.name,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// DirectoryEntry represents a file or directory in a listing
type DirectoryEntry struct {
	Name       string
	Path       string
	IsDir      bool
	IsMarkdown bool
}

// handleIndex generates a directory index page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request, rt *root, dirPath string) {
	if rt.isHiddenPath(dirPath) {
		http.NotFound(w, r)
		return
	}
	log.Printf("dir: %s", rt.relPath(dirPath))
	if rt.liveReload != nil {
		rt.liveReload.EnsureWatching(dirPath)
	}
	// Read directory entries
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate relative path from root directory for breadcrumbs
	relPath, err := filepath.Rel(rt.dir, dirPath)
	if err != nil {
		relPath = "."
	}

	// Generate breadcrumbs
	breadcrumbs := rt.breadcrumbs(relPath)

	// Build list of entries
	var dirEntries []DirectoryEntry

	// Check if we're at root directory (for parent directory link)
	absDirPath, err := filepath.Abs(dirPath)
	if err != nil {
		absDirPath = dirPath
	}
	absRootDir, err := filepath.Abs(rt.dir)
	if err != nil {
		absRootDir = rt.dir
	}
	isAtRoot := absDirPath == absRootDir

	for _, entry := range entries {
		// Skip hidden files/directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Only include directories or markdown files
		isMarkdown := !entry.IsDir() && isPageFile(entry.Name())
		if !entry.IsDir() && !isMarkdown {
			continue
		}

		entryPath := filepath.Join(dirPath, entry.Name())
		relEntryPath, err := filepath.Rel(rt.dir, entryPath)
		if err != nil {
			continue
		}

		// Build URL path with proper encoding
		relSlash := filepath.ToSlash(relEntryPath)
		parts := strings.Split(relSlash, "/")
		encodedParts := make([]string, len(parts))
		for i, part := range parts {
			encodedParts[i] = url.PathEscape(part)
		}
		urlPath := rt.prefix + "/" + strings.Join(encodedParts, "/")
		if entry.IsDir() {
			urlPath += "/"
		}

		dirEntries = append(dirEntries, DirectoryEntry{
			Name:       entry.Name(),
			Path:       urlPath,
			IsDir:      entry.IsDir(),
			IsMarkdown: isMarkdown,
		})
	}

	// Add parent directory entry if not at root
	if !isAtRoot {
		// Calculate parent directory path
		parentDir := filepath.Dir(dirPath)
		relParentPath, err := filepath.Rel(rt.dir, parentDir)
		if err == nil {
			// Build URL path for parent directory using same logic as regular entries
			relSlash := filepath.ToSlash(relParentPath)
			parts := strings.Split(relSlash, "/")
			encodedParts := make([]string, 0)
			for _, part := range parts {
				if part != "." && part != "" {
					encodedParts = append(encodedParts, url.PathEscape(part))
				}
			}
			var parentURLPath string
			if len(encodedParts) == 0 {
				parentURLPath = rt.prefix + "/"
			} else {
				parentURLPath = rt.prefix + "/" + strings.Join(encodedParts, "/") + "/"
			}

			// Prepend parent directory entry
			parentEntry := DirectoryEntry{
				Name:       "..",
				Path:       parentURLPath,
				IsDir:      true,
				IsMarkdown: false,
			}
			dirEntries = append([]DirectoryEntry{parentEntry}, dirEntries...)
		}
	} else if s.multi {
		// A root's top level links back to the list of roots.
		dirEntries = append([]DirectoryEntry{{Name: "..", Path: "/", IsDir: true}}, dirEntries...)
	}

	tmpl := s.templates.directory

	// Determine title
	title := "Index"
	if relPath != "." && relPath != "" {
		title = filepath.Base(dirPath)
	}

	data := struct {
		Title       string
		Breadcrumbs []Breadcrumb
		Entries     []DirectoryEntry
		Root        string
	}{
		Title:       title,
		Breadcrumbs: breadcrumbs,
		Entries:     dirEntries,
		Root:        rt.name,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// WatchedDir represents a watched directory for the settings page.
type WatchedDir struct {
	Path    string
	Display string
	IsRoot  bool
}

// WatchedGroup is one root's watched directories on the multi-root settings page.
type WatchedGroup struct {
	Name string
	Dirs []WatchedDir
}

// watchedDirs lists the directories the root's live reload watches, or nil when
// it has none.
func (rt *root) watchedDirs() []WatchedDir {
	if rt.liveReload == nil {
		return nil
	}
	var dirs []WatchedDir
	absRoot, _ := filepath.Abs(rt.dir)
	for _, dir := range rt.liveReload.WatchedDirs() {
		rel, err := filepath.Rel(absRoot, dir)
		if err != nil {
			rel = dir
		}
		isRoot := dir == absRoot
		display := rel
		if isRoot {
			display = "."
		}
		dirs = append(dirs, WatchedDir{Path: dir, Display: display, IsRoot: isRoot})
	}
	return dirs
}

// handleSettings renders the settings page.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	breadcrumbs := []Breadcrumb{
		{Href: "/", Text: template.HTML(`<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style="vertical-align: middle; display: inline-block;"><path d="M8 0L0 7h2v9h5v-6h2v6h5V7h2L8 0z"/></svg>/`)},
		{Href: "/settings", Text: "Settings"},
	}

	var watchedDirs []WatchedDir
	var groups []WatchedGroup
	liveReloadEnabled := s.hasLiveReload()

	if s.multi {
		for _, rt := range s.roots {
			groups = append(groups, WatchedGroup{Name: rt.name, Dirs: rt.watchedDirs()})
		}
	} else {
		watchedDirs = s.roots[0].watchedDirs()
	}

	tmpl := s.templates.settings

	data := struct {
		Title             string
		Breadcrumbs       []Breadcrumb
		WatchedDirs       []WatchedDir
		Groups            []WatchedGroup
		Multi             bool
		LiveReloadEnabled bool
		HasRoot           bool
	}{
		Title:             "Settings",
		Breadcrumbs:       breadcrumbs,
		WatchedDirs:       watchedDirs,
		Groups:            groups,
		Multi:             s.multi,
		LiveReloadEnabled: liveReloadEnabled,
		HasRoot:           !s.multi,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// handleShutdown shuts down the server.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html><html><head><title>Shutting Down</title><link rel="stylesheet" href="/_mdserver/style.css"></head><body><div class="container"><h1>Server shutting down...</h1><p>You can close this tab.</p></div></body></html>`))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
}

// handleRemoveWatch removes a watched directory.
func (s *Server) handleRemoveWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dir := r.FormValue("dir")
	if dir == "" {
		http.Error(w, "Missing dir parameter", http.StatusBadRequest)
		return
	}

	// Multi-root requests name the root whose watch is removed; a directory of
	// another root is never touched.
	rt := s.roots[0]
	if s.multi {
		if rt = s.rootNamed(r.FormValue("root")); rt == nil {
			http.Error(w, "Unknown root", http.StatusBadRequest)
			return
		}
		if !rt.contains(dir) {
			http.Error(w, "Directory is not in the root", http.StatusBadRequest)
			return
		}
	}

	// Reject removal of root directory
	absRoot, _ := filepath.Abs(rt.dir)
	if dir == absRoot {
		http.Error(w, "Cannot remove root directory watcher", http.StatusBadRequest)
		return
	}

	if rt.liveReload != nil {
		if err := rt.liveReload.RemoveWatch(dir); err != nil {
			log.Printf("Failed to remove watch on %s: %v", dir, err)
		}
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// extractTitle extracts title from markdown content or uses filename
func extractTitle(content, filename string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return trimMarkdownExt(filename)
}

// trimMarkdownExt removes a .md extension in any case from name.
func trimMarkdownExt(name string) string {
	if ext := filepath.Ext(name); strings.EqualFold(ext, ".md") {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

// createBreadcrumbs generates breadcrumb navigation from a relative path
// relPath should be relative to the root directory (e.g., "docs/subdir" or "docs/subdir/file.md")
// For markdown files, it generates breadcrumbs for the containing directory and includes the filename
func createBreadcrumbs(relPath string) []Breadcrumb {
	return createPrefixedBreadcrumbs("", "", relPath)
}

// breadcrumbs returns the breadcrumbs for relPath, a path relative to this root.
func (rt *root) breadcrumbs(relPath string) []Breadcrumb {
	return createPrefixedBreadcrumbs(rt.name, rt.prefix, relPath)
}

// createPrefixedBreadcrumbs is createBreadcrumbs for a root served under
// prefix ("/<name>", or empty in single-root mode). With a prefix the home icon
// links to /, then comes the root's own "<name>/" crumb, then the path.
func createPrefixedBreadcrumbs(name, prefix, relPath string) []Breadcrumb {
	crumbs := []Breadcrumb{homeBreadcrumb}
	if prefix != "" {
		crumbs = append(crumbs, Breadcrumb{Href: prefix + "/", Text: template.HTML(template.HTMLEscapeString(name) + "/")})
	}

	// If path is empty or just ".", return root breadcrumb only
	if relPath == "" || relPath == "." {
		return crumbs
	}

	// Check if this is a file (has an extension)
	isFile := filepath.Ext(relPath) != ""
	var dirPath string
	var filename string

	if isFile {
		// Extract directory path and filename
		dirPath = filepath.Dir(relPath)
		filename = filepath.Base(relPath)
	} else {
		dirPath = relPath
	}

	// Normalize path separators and clean up
	dirPath = filepath.ToSlash(dirPath)
	dirPath = strings.Trim(dirPath, "/")

	// If directory path is not empty or ".", add directory breadcrumbs
	if dirPath != "" && dirPath != "." {
		// Split path into segments
		parts := strings.Split(dirPath, "/")
		collectPath := prefix + "/"

		// Create breadcrumb for each directory segment
		for _, part := range parts {
			if part == "" {
				continue
			}

			// URL encode the directory name
			encodedPart := url.PathEscape(part)
			fullLink := collectPath + encodedPart + "/"

			crumbs = append(crumbs, Breadcrumb{
				Href: fullLink,
				Text: template.HTML(part + "/"),
			})

			collectPath = fullLink
		}
	}

	// If this is a file, add the filename as the final breadcrumb
	if isFile {
		displayName := trimMarkdownExt(filename)

		// Build the full URL path for the file
		relSlash := filepath.ToSlash(relPath)
		parts := strings.Split(relSlash, "/")
		encodedParts := make([]string, len(parts))
		for i, part := range parts {
			encodedParts[i] = url.PathEscape(part)
		}
		fileURL := prefix + "/" + strings.Join(encodedParts, "/")

		crumbs = append(crumbs, Breadcrumb{
			Href: fileURL,
			Text: template.HTML(displayName),
		})
	}

	return crumbs
}

// injectLiveReloadScript injects the LiveReload client script into HTML templates
func injectLiveReloadScript(html string) string {
	script := `<script>
(function() {
	// data-root names the root this page belongs to; it is empty in single-root
	// mode and absent on pages that belong to no root.
	var root = document.body.getAttribute('data-root');
	if (root === null) {
		return;
	}
	function connect() {
		var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		var host = window.location.host;
		var query = root ? '?root=' + encodeURIComponent(root) : '';
		var ws = new WebSocket(protocol + '//' + host + '/livereload' + query);

		ws.onmessage = function(event) {
			if (event.data === 'reload') {
				window.location.reload();
			}
		};

		ws.onerror = function(error) {
			console.log('LiveReload connection error:', error);
		};

		ws.onclose = function() {
			// Attempt to reconnect after 1 second
			setTimeout(connect, 1000);
		};
	}

	connect();
})();
</script>`

	// Inject script before closing </body> tag
	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", script+"</body>", 1)
	}
	// If no </body> tag, append at the end
	return html + script
}
