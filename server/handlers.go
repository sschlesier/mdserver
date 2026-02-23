package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mdserver/renderer"
)

// settingsGearIcon is the SVG markup for the settings gear icon used in breadcrumbs.
const settingsGearIcon = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>`

// Breadcrumb represents a single breadcrumb navigation item
type Breadcrumb struct {
	Href string
	Text template.HTML
}

// handleMarkdown serves a markdown file as HTML
func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request, filePath string) {
	log.Printf("markdown: %s", s.relPath(filePath))
	if s.liveReload != nil {
		s.liveReload.EnsureWatching(filepath.Dir(filePath))
	}
	// Read markdown file
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read file: %v", err), http.StatusNotFound)
		return
	}

	// Render markdown to HTML
	htmlContent, err := renderer.RenderMarkdown(content)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render markdown: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract title from first h1 or use filename
	title := extractTitle(string(content), filepath.Base(filePath))

	// Calculate relative path from root directory for breadcrumbs
	relPath, err := filepath.Rel(s.config.RootDir, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}

	// Generate breadcrumbs
	breadcrumbs := createBreadcrumbs(relPath)

	// Load and execute template
	tmpl, err := s.loadTemplate()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load template: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Title       string
		Content     template.HTML
		Breadcrumbs []Breadcrumb
	}{
		Title:       title,
		Content:     template.HTML(htmlContent),
		Breadcrumbs: breadcrumbs,
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
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request, dirPath string) {
	log.Printf("dir: %s", s.relPath(dirPath))
	if s.liveReload != nil {
		s.liveReload.EnsureWatching(dirPath)
	}
	// Read directory entries
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate relative path from root directory for breadcrumbs
	relPath, err := filepath.Rel(s.config.RootDir, dirPath)
	if err != nil {
		relPath = "."
	}

	// Generate breadcrumbs
	breadcrumbs := createBreadcrumbs(relPath)

	// Build list of entries
	var dirEntries []DirectoryEntry

	// Check if we're at root directory (for parent directory link)
	absDirPath, err := filepath.Abs(dirPath)
	if err != nil {
		absDirPath = dirPath
	}
	absRootDir, err := filepath.Abs(s.config.RootDir)
	if err != nil {
		absRootDir = s.config.RootDir
	}
	isAtRoot := absDirPath == absRootDir

	for _, entry := range entries {
		// Skip hidden files/directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Only include directories or markdown files
		isMarkdown := !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md")
		if !entry.IsDir() && !isMarkdown {
			continue
		}

		entryPath := filepath.Join(dirPath, entry.Name())
		relEntryPath, err := filepath.Rel(s.config.RootDir, entryPath)
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
		urlPath := "/" + strings.Join(encodedParts, "/")
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
		relParentPath, err := filepath.Rel(s.config.RootDir, parentDir)
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
				parentURLPath = "/"
			} else {
				parentURLPath = "/" + strings.Join(encodedParts, "/") + "/"
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
	}

	// Load directory template
	tmpl, err := s.loadDirectoryTemplate()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load template: %v", err), http.StatusInternalServerError)
		return
	}

	// Determine title
	title := "Index"
	if relPath != "." && relPath != "" {
		title = filepath.Base(dirPath)
	}

	data := struct {
		Title       string
		Breadcrumbs []Breadcrumb
		Entries     []DirectoryEntry
	}{
		Title:       title,
		Breadcrumbs: breadcrumbs,
		Entries:     dirEntries,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// handleAssets serves static files (images, CSS, JS)
func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	// Extract file path from request
	requestPath := r.URL.Path

	// Remove /assets/ prefix if present
	if strings.HasPrefix(requestPath, "/assets/") {
		requestPath = strings.TrimPrefix(requestPath, "/assets/")
	} else if requestPath == "/assets" {
		requestPath = ""
	}

	// If no path specified, try to serve CSS
	if requestPath == "" || requestPath == "/" {
		if r.URL.Path == "/assets/style.css" || strings.HasSuffix(r.URL.Path, "style.css") {
			s.serveCSS(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}

	// Check if this is a request for style.css
	if requestPath == "style.css" {
		s.serveCSS(w, r)
		return
	}

	// Construct full file path
	filePath := filepath.Join(s.config.RootDir, requestPath)

	// Validate path is within root directory
	if !s.isValidPath(filePath) {
		http.Error(w, "Invalid path", http.StatusForbidden)
		return
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	log.Printf("file: %s", s.relPath(filePath))

	// Set appropriate Content-Type
	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := getContentType(ext)
	w.Header().Set("Content-Type", contentType)

	// Serve file
	http.ServeFile(w, r, filePath)
}

// serveCSS serves the CSS file from template directory
func (s *Server) serveCSS(w http.ResponseWriter, r *http.Request) {
	// Try to find template directory relative to executable or current directory
	exePath, err := os.Executable()
	var cssPath string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		cssPath = filepath.Join(exeDir, "template", "style.css")
		if _, err := os.Stat(cssPath); os.IsNotExist(err) {
			// Try relative to current working directory
			cssPath = "template/style.css"
		}
	} else {
		cssPath = "template/style.css"
	}

	// Check if file exists
	if _, err := os.Stat(cssPath); os.IsNotExist(err) {
		// Serve default CSS inline
		log.Printf("file: %s (default CSS)", cssPath)
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write([]byte(getDefaultCSS()))
		return
	}

	log.Printf("file: %s", cssPath)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, cssPath)
}

// loadTemplate loads the HTML template
func (s *Server) loadTemplate() (*template.Template, error) {
	// Try to find template directory relative to executable or current directory
	exePath, err := os.Executable()
	var templatePath string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		templatePath = filepath.Join(exeDir, "template", "page.html")
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			// Try relative to current working directory
			templatePath = "template/page.html"
		}
	} else {
		templatePath = "template/page.html"
	}

	// Try to load template file
	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		// Use default template
		return s.getDefaultTemplate()
	}

	tmplContentStr := string(tmplContent)
	// Inject LiveReload script if enabled
	if s.config.EnableLiveReload {
		tmplContentStr = s.injectLiveReloadScript(tmplContentStr)
	}

	tmpl, parseErr := template.New("page").Parse(tmplContentStr)
	if parseErr != nil {
		return nil, parseErr
	}

	return tmpl, nil
}

// loadDirectoryTemplate loads the directory listing template
func (s *Server) loadDirectoryTemplate() (*template.Template, error) {
	// Try to find template directory relative to executable or current directory
	exePath, err := os.Executable()
	var templatePath string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		templatePath = filepath.Join(exeDir, "template", "directory.html")
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			// Try relative to current working directory
			templatePath = "template/directory.html"
		}
	} else {
		templatePath = "template/directory.html"
	}

	// Try to load template file
	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		// Use default directory template
		return s.getDefaultDirectoryTemplate()
	}

	tmplContentStr := string(tmplContent)
	// Inject LiveReload script if enabled
	if s.config.EnableLiveReload {
		tmplContentStr = s.injectLiveReloadScript(tmplContentStr)
	}

	tmpl, parseErr := template.New("directory").Parse(tmplContentStr)
	if parseErr != nil {
		return nil, parseErr
	}

	return tmpl, nil
}

// getDefaultTemplate returns a default HTML template
func (s *Server) getDefaultTemplate() (*template.Template, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<link rel="icon" type="image/svg+xml" href="/favicon.ico">
	<link rel="icon" type="image/svg+xml" href="/favicon.svg">
	<link rel="apple-touch-icon" href="/favicon.ico">
	<link rel="stylesheet" href="/assets/style.css">
	<script src="https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js"></script>
</head>
<body>
	<div class="container">
		{{if .Breadcrumbs}}
		<nav class="breadcrumbs">
			<span class="breadcrumb-links">
				{{range $index, $crumb := .Breadcrumbs}}{{if $index}}    {{end}}<a href="{{$crumb.Href}}">{{$crumb.Text}}</a>{{end}}
			</span>
			<a href="/settings" class="settings-icon" title="Settings">` + settingsGearIcon + `</a>
		</nav>
		{{end}}
		{{.Content}}
	</div>
	<script>
		mermaid.initialize({ startOnLoad: true, theme: 'default' });
	</script>
</body>
</html>`

	tmplStr := tmpl
	// Inject LiveReload script if enabled
	if s.config.EnableLiveReload {
		tmplStr = s.injectLiveReloadScript(tmplStr)
	}

	return template.New("page").Parse(tmplStr)
}

// getDefaultDirectoryTemplate returns a default directory listing template
func (s *Server) getDefaultDirectoryTemplate() (*template.Template, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<link rel="icon" type="image/svg+xml" href="/favicon.ico">
	<link rel="icon" type="image/svg+xml" href="/favicon.svg">
	<link rel="apple-touch-icon" href="/favicon.ico">
	<link rel="stylesheet" href="/assets/style.css">
</head>
<body>
	<div class="container">
		<nav class="breadcrumbs">
			<span class="breadcrumb-links">
				{{range $index, $crumb := .Breadcrumbs}}{{if $index}}    {{end}}<a href="{{$crumb.Href}}">{{$crumb.Text}}</a>{{end}}
			</span>
			<a href="/settings" class="settings-icon" title="Settings">` + settingsGearIcon + `</a>
		</nav>
		<h1>{{.Title}}</h1>
		<ul class="directory-listing">
			{{range .Entries}}
			<li class="{{if .IsDir}}directory{{else if .IsMarkdown}}markdown{{else}}file{{end}}">
				<a href="{{.Path}}">{{.Name}}{{if .IsDir}}/{{end}}</a>
			</li>
			{{end}}
		</ul>
	</div>
</body>
</html>`

	tmplStr := tmpl
	// Inject LiveReload script if enabled
	if s.config.EnableLiveReload {
		tmplStr = s.injectLiveReloadScript(tmplStr)
	}

	return template.New("directory").Parse(tmplStr)
}

// WatchedDir represents a watched directory for the settings page.
type WatchedDir struct {
	Path    string
	Display string
	IsRoot  bool
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
	liveReloadEnabled := s.liveReload != nil

	if liveReloadEnabled {
		absRoot, _ := filepath.Abs(s.config.RootDir)
		for _, dir := range s.liveReload.WatchedDirs() {
			rel, err := filepath.Rel(absRoot, dir)
			if err != nil {
				rel = dir
			}
			isRoot := dir == absRoot
			display := rel
			if isRoot {
				display = "."
			}
			watchedDirs = append(watchedDirs, WatchedDir{
				Path:    dir,
				Display: display,
				IsRoot:  isRoot,
			})
		}
	}

	tmpl, err := s.loadSettingsTemplate()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load template: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Title              string
		Breadcrumbs        []Breadcrumb
		WatchedDirs        []WatchedDir
		LiveReloadEnabled  bool
	}{
		Title:             "Settings",
		Breadcrumbs:       breadcrumbs,
		WatchedDirs:       watchedDirs,
		LiveReloadEnabled: liveReloadEnabled,
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
	w.Write([]byte(`<!DOCTYPE html><html><head><title>Shutting Down</title><link rel="stylesheet" href="/assets/style.css"></head><body><div class="container"><h1>Server shutting down...</h1><p>You can close this tab.</p></div></body></html>`))
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

	// Reject removal of root directory
	absRoot, _ := filepath.Abs(s.config.RootDir)
	if dir == absRoot {
		http.Error(w, "Cannot remove root directory watcher", http.StatusBadRequest)
		return
	}

	if s.liveReload != nil {
		if err := s.liveReload.RemoveWatch(dir); err != nil {
			log.Printf("Failed to remove watch on %s: %v", dir, err)
		}
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// loadSettingsTemplate loads the settings page template.
func (s *Server) loadSettingsTemplate() (*template.Template, error) {
	exePath, err := os.Executable()
	var templatePath string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		templatePath = filepath.Join(exeDir, "template", "settings.html")
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			templatePath = "template/settings.html"
		}
	} else {
		templatePath = "template/settings.html"
	}

	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return s.getDefaultSettingsTemplate()
	}

	tmplContentStr := string(tmplContent)
	if s.config.EnableLiveReload {
		tmplContentStr = s.injectLiveReloadScript(tmplContentStr)
	}

	return template.New("settings").Parse(tmplContentStr)
}

// getDefaultSettingsTemplate returns a default settings page template.
func (s *Server) getDefaultSettingsTemplate() (*template.Template, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<link rel="icon" type="image/svg+xml" href="/favicon.ico">
	<link rel="icon" type="image/svg+xml" href="/favicon.svg">
	<link rel="apple-touch-icon" href="/favicon.ico">
	<link rel="stylesheet" href="/assets/style.css">
</head>
<body>
	<div class="container">
		<nav class="breadcrumbs">
			<span class="breadcrumb-links">
				{{range $index, $crumb := .Breadcrumbs}}{{if $index}}    {{end}}<a href="{{$crumb.Href}}">{{$crumb.Text}}</a>{{end}}
			</span>
		</nav>
		<h1>Settings</h1>
		<div class="settings-section">
			<h2>Server</h2>
			<form method="POST" action="/settings/shutdown" onsubmit="return confirm('Are you sure you want to shut down the server?');">
				<button type="submit" class="shutdown-btn">` + `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path><line x1="12" y1="2" x2="12" y2="12"></line></svg>` + ` Shut Down Server</button>
			</form>
		</div>
		{{if .LiveReloadEnabled}}
		<div class="settings-section">
			<h2>Watched Directories</h2>
			<div class="watched-dirs-list">
				{{range .WatchedDirs}}
				<div class="watched-dir-row">
					<span class="watched-dir-path">{{.Display}}</span>
					{{if .IsRoot}}<span class="root-badge">root</span>{{else}}
					<form method="POST" action="/settings/remove-watch" style="display:inline;">
						<input type="hidden" name="dir" value="{{.Path}}">
						<button type="submit" class="remove-watch-btn">Remove</button>
					</form>
					{{end}}
				</div>
				{{end}}
			</div>
		</div>
		{{end}}
	</div>
</body>
</html>`

	tmplStr := tmpl
	if s.config.EnableLiveReload {
		tmplStr = s.injectLiveReloadScript(tmplStr)
	}

	return template.New("settings").Parse(tmplStr)
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
	// Remove .md extension from filename
	return strings.TrimSuffix(filename, ".md")
}

// createBreadcrumbs generates breadcrumb navigation from a relative path
// relPath should be relative to the root directory (e.g., "docs/subdir" or "docs/subdir/file.md")
// For markdown files, it generates breadcrumbs for the containing directory and includes the filename
func createBreadcrumbs(relPath string) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Href: "/", Text: template.HTML(`<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style="vertical-align: middle; display: inline-block;"><path d="M8 0L0 7h2v9h5v-6h2v6h5V7h2L8 0z"/></svg>/`)},
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
		collectPath := "/"

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
		// Remove .md extension from display
		displayName := strings.TrimSuffix(filename, ".md")

		// Build the full URL path for the file
		relSlash := filepath.ToSlash(relPath)
		parts := strings.Split(relSlash, "/")
		encodedParts := make([]string, len(parts))
		for i, part := range parts {
			encodedParts[i] = url.PathEscape(part)
		}
		fileURL := "/" + strings.Join(encodedParts, "/")

		crumbs = append(crumbs, Breadcrumb{
			Href: fileURL,
			Text: template.HTML(displayName),
		})
	}

	return crumbs
}

// injectLiveReloadScript injects the LiveReload client script into HTML templates
func (s *Server) injectLiveReloadScript(html string) string {
	script := `<script>
(function() {
	function connect() {
		var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		var host = window.location.host;
		var ws = new WebSocket(protocol + '//' + host + '/livereload');

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

// getDefaultCSS returns default CSS content
func getDefaultCSS() string {
	return `/* Default CSS - template/style.css not found */
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 900px; margin: 0 auto; padding: 20px; line-height: 1.6; }
code { background: #f5f5f5; padding: 2px 6px; border-radius: 3px; }
pre { background: #f5f5f5; padding: 16px; border-radius: 5px; overflow-x: auto; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #ddd; padding: 8px 12px; }
th { background: #f8f8f8; }`
}
