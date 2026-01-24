package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Config holds server configuration
type Config struct {
	Host             string
	Port             int
	RootDir          string
	File             string
	EnableLiveReload bool
}

// Server represents the HTTP server
type Server struct {
	config     Config
	mux        *http.ServeMux
	liveReload *LiveReload
}

// NewServer creates a new server instance
func NewServer(config Config) *Server {
	s := &Server{
		config: config,
		mux:    http.NewServeMux(),
	}

	// Initialize LiveReload if enabled
	if config.EnableLiveReload {
		var err error
		s.liveReload, err = NewLiveReload(config.RootDir)
		if err != nil {
			log.Printf("Failed to initialize LiveReload: %v", err)
		} else {
			if err := s.liveReload.Start(); err != nil {
				log.Printf("Failed to start LiveReload: %v", err)
				s.liveReload = nil
			}
		}
	}

	s.setupRoutes()
	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	log.Printf("Listening on %s", addr)
	return http.ListenAndServe(addr, s.mux)
}

// Stop stops the server and cleans up resources
func (s *Server) Stop() {
	if s.liveReload != nil {
		s.liveReload.Stop()
	}
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Favicon handlers - serves the markdown icon SVG
	s.mux.HandleFunc("/favicon.ico", s.handleFavicon)
	s.mux.HandleFunc("/favicon.svg", s.handleFavicon)

	// Static assets handler (must be registered first for /assets/ path)
	s.mux.HandleFunc("/assets/", s.handleAssets)

	// LiveReload WebSocket endpoint
	if s.liveReload != nil {
		s.mux.HandleFunc("/livereload", s.liveReload.HandleWebSocket)
	}

	// Root handler - handles all other routes including root and markdown files
	s.mux.HandleFunc("/", s.handleRequest)
}

// handleFavicon serves the markdown favicon
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	// Try to find favicon.svg in template directory
	exePath, err := os.Executable()
	var faviconPath string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		faviconPath = filepath.Join(exeDir, "template", "favicon.svg")
		if _, err := os.Stat(faviconPath); os.IsNotExist(err) {
			// Try relative to current working directory
			faviconPath = "template/favicon.svg"
		}
	} else {
		faviconPath = "template/favicon.svg"
	}

	// Check if file exists
	if _, err := os.Stat(faviconPath); os.IsNotExist(err) {
		// Serve default markdown icon as SVG
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect width="100" height="100" rx="20" fill="#000"/><text x="50" y="70" font-family="Arial, sans-serif" font-size="60" font-weight="bold" fill="#fff" text-anchor="middle">M</text></svg>`))
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	// Use shorter cache for initial requests to help Safari pick it up
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, faviconPath)
}

// handleRequest handles all non-asset requests (root, markdown files, etc.)
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	requestPath := r.URL.Path

	// Handle root path
	if requestPath == "/" {
		// Always serve directory index at root
		s.handleIndex(w, r, s.config.RootDir)
		return
	}

	// Handle markdown file requests
	// Remove leading slash
	if len(requestPath) > 0 && requestPath[0] == '/' {
		requestPath = requestPath[1:]
	}

	filePath := filepath.Join(s.config.RootDir, requestPath)

	// Check if path exists and is a directory
	if info, err := os.Stat(filePath); err == nil && info.IsDir() {
		// Ensure directory paths end with / for consistency
		if !strings.HasSuffix(requestPath, "/") && !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		s.handleIndex(w, r, filePath)
		return
	}

	// Check if it's a markdown file (has .md extension or no extension)
	ext := filepath.Ext(filePath)
	if ext == ".md" {
		if s.isValidPath(filePath) {
			s.handleMarkdown(w, r, filePath)
			return
		}
	} else if ext == "" {
		// Try adding .md extension
		filePathWithExt := filePath + ".md"
		if s.isValidPath(filePathWithExt) {
			s.handleMarkdown(w, r, filePathWithExt)
			return
		}
	}

	// Not a markdown file, try to serve as static asset from root directory
	s.handleStaticFile(w, r, filePath)
}

// isValidPath checks if a file path is within the root directory (security)
func (s *Server) isValidPath(filePath string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	absRoot, err := filepath.Abs(s.config.RootDir)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}

	// Prevent directory traversal
	return rel != ".." && rel != "." && len(rel) > 0 && rel[0] != '.'
}

// handleStaticFile serves a static file from the root directory
func (s *Server) handleStaticFile(w http.ResponseWriter, r *http.Request, filePath string) {
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

	// Set appropriate Content-Type based on extension
	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := getContentType(ext)
	w.Header().Set("Content-Type", contentType)

	// Serve file
	http.ServeFile(w, r, filePath)
}

// relPath returns a path relative to the root directory, or the original path if it's outside the root
func (s *Server) relPath(path string) string {
	rel, err := filepath.Rel(s.config.RootDir, path)
	if err != nil {
		return path
	}
	// If path is outside root directory, return original
	if strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// getContentType returns the MIME type for a file extension
func getContentType(ext string) string {
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
