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

	"mdserver/renderer"
)

// Breadcrumb represents a single breadcrumb navigation item
type Breadcrumb struct {
	Href string
	Text string
}

// handleMarkdown serves a markdown file as HTML
func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request, filePath string) {
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
	tmpl, err := loadTemplate()
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
	tmpl, err := loadDirectoryTemplate()
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
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write([]byte(getDefaultCSS()))
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	http.ServeFile(w, r, cssPath)
}

// loadTemplate loads the HTML template
func loadTemplate() (*template.Template, error) {
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
		return getDefaultTemplate()
	}

	tmpl, parseErr := template.New("page").Parse(string(tmplContent))
	if parseErr != nil {
		return nil, parseErr
	}

	return tmpl, nil
}

// loadDirectoryTemplate loads the directory listing template
func loadDirectoryTemplate() (*template.Template, error) {
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
		return getDefaultDirectoryTemplate()
	}

	tmpl, parseErr := template.New("directory").Parse(string(tmplContent))
	if parseErr != nil {
		return nil, parseErr
	}

	return tmpl, nil
}

// getDefaultTemplate returns a default HTML template
func getDefaultTemplate() (*template.Template, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<link rel="stylesheet" href="/assets/style.css">
</head>
<body>
	<div class="container">
		{{if .Breadcrumbs}}
		<nav class="breadcrumbs">
			{{range $index, $crumb := .Breadcrumbs}}{{if $index}}    {{end}}<a href="{{$crumb.Href}}">{{$crumb.Text}}</a>{{end}}
		</nav>
		{{end}}
		{{.Content}}
	</div>
</body>
</html>`
	return template.New("page").Parse(tmpl)
}

// getDefaultDirectoryTemplate returns a default directory listing template
func getDefaultDirectoryTemplate() (*template.Template, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<link rel="stylesheet" href="/assets/style.css">
</head>
<body>
	<div class="container">
		<nav class="breadcrumbs">
			{{range $index, $crumb := .Breadcrumbs}}{{if $index}}    {{end}}<a href="{{$crumb.Href}}">{{$crumb.Text}}</a>{{end}}
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
	return template.New("directory").Parse(tmpl)
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
		{Href: "/", Text: "./"},
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
				Text: part + "/",
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
			Text: displayName,
		})
	}

	return crumbs
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
