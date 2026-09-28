package server

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrRootMissing is returned by Start when the server shut itself down
// because the root directory no longer exists.
var ErrRootMissing = errors.New("root directory no longer exists")

const defaultRootPollInterval = time.Second

// Config holds server configuration
type Config struct {
	Host             string
	Port             int
	RootDir          string
	File             string
	EnableLiveReload bool
	Verbose          bool
}

// Server represents the HTTP server
type Server struct {
	config     Config
	mux        *http.ServeMux
	liveReload *LiveReload
	httpServer *http.Server

	rootPollInterval time.Duration
	rootGone         chan struct{}
	rootGoneOnce     sync.Once
	shutdownDone     chan struct{}
	stopChan         chan struct{}
	stopOnce         sync.Once
}

// NewServer creates a new server instance
func NewServer(config Config) *Server {
	s := &Server{
		config:           config,
		mux:              http.NewServeMux(),
		rootPollInterval: defaultRootPollInterval,
		rootGone:         make(chan struct{}),
		shutdownDone:     make(chan struct{}),
		stopChan:         make(chan struct{}),
	}

	// Initialize LiveReload if enabled
	if config.EnableLiveReload {
		var err error
		s.liveReload, err = NewLiveReload(config.RootDir, config.Verbose)
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
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler: s.requireRoot(s.mux),
	}
	return s
}

// Start starts the HTTP server. It returns ErrRootMissing if the server shut
// itself down because the root directory disappeared.
func (s *Server) Start() error {
	log.Printf("Listening on %s", s.httpServer.Addr)
	go s.watchRoot()
	err := s.httpServer.ListenAndServe()
	select {
	case <-s.rootGone:
		<-s.shutdownDone
		return ErrRootMissing
	default:
		return err
	}
}

// Stop stops the server and cleans up resources
func (s *Server) Stop() {
	s.stopOnce.Do(func() { close(s.stopChan) })
	if s.liveReload != nil {
		s.liveReload.Stop()
	}
}

// rootExists reports whether the root directory is still present. Errors
// other than "not exist" (e.g. permissions) are treated as present.
func (s *Server) rootExists() bool {
	info, err := os.Stat(s.config.RootDir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return info.IsDir()
}

// watchRoot polls for the root directory and shuts the server down if it
// disappears while no request is in flight.
func (s *Server) watchRoot() {
	ticker := time.NewTicker(s.rootPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if !s.rootExists() {
				s.shutdownRootMissing()
				return
			}
		case <-s.rootGone:
			return
		case <-s.stopChan:
			return
		}
	}
}

// requireRoot answers every request with a shutdown notice once the root
// directory is gone, then shuts the server down.
func (s *Server) requireRoot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rootExists() {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusGone)
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Folder Missing</title></head><body style="font-family: sans-serif; margin: 3em;"><h1>Folder missing</h1><p><code>%s</code> no longer exists. The server is shutting down.</p><p>You can close this tab.</p></body></html>`, html.EscapeString(s.config.RootDir))
		s.shutdownRootMissing()
	})
}

// shutdownRootMissing gracefully stops the HTTP server, letting in-flight
// responses (including the shutdown notice) finish.
func (s *Server) shutdownRootMissing() {
	s.rootGoneOnce.Do(func() {
		close(s.rootGone)
		go func() {
			defer close(s.shutdownDone)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.httpServer.Shutdown(ctx)
		}()
	})
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

	// Settings routes
	s.mux.HandleFunc("/settings", s.handleSettings)
	s.mux.HandleFunc("/settings/shutdown", s.handleShutdown)
	s.mux.HandleFunc("/settings/remove-watch", s.handleRemoveWatch)

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
