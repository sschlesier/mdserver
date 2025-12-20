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
	Host    string
	Port    int
	RootDir string
	File    string
}

// Server represents the HTTP server
type Server struct {
	config Config
	mux    *http.ServeMux
}

// NewServer creates a new server instance
func NewServer(config Config) *Server {
	s := &Server{
		config: config,
		mux:    http.NewServeMux(),
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

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Static assets handler (must be registered first for /assets/ path)
	s.mux.HandleFunc("/assets/", s.handleAssets)

	// Root handler - handles all other routes including root and markdown files
	s.mux.HandleFunc("/", s.handleRequest)
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

	// Set appropriate Content-Type based on extension
	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := getContentType(ext)
	w.Header().Set("Content-Type", contentType)

	// Serve file
	http.ServeFile(w, r, filePath)
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
