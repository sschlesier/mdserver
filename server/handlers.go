package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mdserver/renderer"
)

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

	// Load and execute template
	tmpl, err := loadTemplate()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load template: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Title   string
		Content template.HTML
	}{
		Title:   title,
		Content: template.HTML(htmlContent),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

// handleIndex generates a directory index page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.config.RootDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read directory: %v", err), http.StatusInternalServerError)
		return
	}

	var markdownFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			markdownFiles = append(markdownFiles, entry.Name())
		}
	}

	// Generate simple HTML index
	html := "<!DOCTYPE html>\n<html><head><meta charset='utf-8'><title>Index</title>"
	html += "<link rel='stylesheet' href='/assets/style.css'>"
	html += "</head><body><div class='container'><h1>Markdown Files</h1><ul>"

	if len(markdownFiles) == 0 {
		html += "<li>No markdown files found</li>"
	} else {
		for _, file := range markdownFiles {
			html += fmt.Sprintf("<li><a href='/%s'>%s</a></li>", file, file)
		}
	}

	html += "</ul></div></body></html>"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
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

	return template.New("page").Parse(string(tmplContent))
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
		{{.Content}}
	</div>
</body>
</html>`
	return template.New("page").Parse(tmpl)
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

