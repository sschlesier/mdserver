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

// rootMissingShutdownTimeout is how long a missing-root shutdown waits for
// in-flight requests before closing their connections.
var rootMissingShutdownTimeout = 2 * time.Second

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
	templates  templates

	rootGone     chan struct{}
	rootGoneOnce sync.Once
	shutdownDone chan struct{}

	stopOnce sync.Once
}

// NewServer creates a new server instance
func NewServer(config Config) *Server {
	s := &Server{
		config:       config,
		templates:    parseTemplates(config.EnableLiveReload),
		mux:          http.NewServeMux(),
		rootGone:     make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler: s.requireRoot(s.mux),
	}

	// Initialize LiveReload if enabled
	if config.EnableLiveReload {
		var err error
		s.liveReload, err = NewLiveReload(config.RootDir, config.Verbose)
		if err != nil {
			log.Printf("Failed to initialize LiveReload: %v", err)
		} else {
			s.liveReload.onRootRemoved = s.shutdownRootMissing
			if err := s.liveReload.Start(); err != nil {
				log.Printf("Failed to start LiveReload: %v", err)
				s.liveReload = nil
			}
		}
	}

	s.setupRoutes()
	return s
}

// Start starts the HTTP server. It returns ErrRootMissing if the server shut
// itself down because the root directory disappeared.
func (s *Server) Start() error {
	log.Printf("Listening on %s", s.httpServer.Addr)
	err := s.httpServer.ListenAndServe()
	select {
	case <-s.rootGone:
		<-s.shutdownDone
		return ErrRootMissing
	default:
		return err
	}
}

// Stop stops the server and cleans up resources. It is safe to call more
// than once, including concurrently: main calls it both on a missing root
// and on SIGINT/SIGTERM.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		if s.liveReload != nil {
			s.liveReload.Stop()
		}
	})
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
// responses (including the shutdown notice) finish. Requests still running
// after rootMissingShutdownTimeout are cut off.
func (s *Server) shutdownRootMissing() {
	s.rootGoneOnce.Do(func() {
		close(s.rootGone)
		go func() {
			defer close(s.shutdownDone)
			ctx, cancel := context.WithTimeout(context.Background(), rootMissingShutdownTimeout)
			defer cancel()
			if err := s.httpServer.Shutdown(ctx); errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Shutdown timed out after %s; closing remaining connections", rootMissingShutdownTimeout)
				s.httpServer.Close()
			}
		}()
	})
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Favicon handlers - serves the markdown icon SVG
	s.mux.HandleFunc("/favicon.ico", s.handleFavicon)
	s.mux.HandleFunc("/favicon.svg", s.handleFavicon)

	// mdserver's stylesheet lives under a prefix no served folder is expected to use.
	s.mux.HandleFunc("/_mdserver/style.css", s.serveCSS)

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
	w.Header().Set("Content-Type", "image/svg+xml")
	// Use shorter cache for initial requests to help Safari pick it up
	w.Header().Set("Cache-Control", "public, max-age=3600")
	serveEmbedded(w, r, "favicon.svg", faviconSVG)
}

// serveCSS serves mdserver's stylesheet
func (s *Server) serveCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	serveEmbedded(w, r, "style.css", styleCSS)
}

// handleRequest serves paths under the root: directory listings, rendered Markdown and images.
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

	if s.isHiddenPath(filePath) {
		http.NotFound(w, r)
		return
	}

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
	if strings.EqualFold(ext, ".md") {
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

	// Not Markdown: serve it if it's an allowlisted image
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
	return rel != ".." && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !hasDotSegment(rel)
}

// hasDotSegment reports whether any segment of a root-relative path starts with ".".
func hasDotSegment(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == "." || seg == ".." {
			continue
		}
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// isHiddenPath reports whether filePath is inside the root and has a dot segment
// relative to the root. Paths outside the root are not hidden; isValidPath rejects them.
func (s *Server) isHiddenPath(filePath string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(s.config.RootDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return hasDotSegment(rel)
}

// handleStaticFile serves an allowlisted image from the root directory; anything else gets 404.
func (s *Server) handleStaticFile(w http.ResponseWriter, r *http.Request, filePath string) {
	if s.isHiddenPath(filePath) {
		http.NotFound(w, r)
		return
	}

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

	contentType, ok := imageContentType(filepath.Ext(filePath))
	if !ok {
		http.NotFound(w, r)
		return
	}

	log.Printf("file: %s", s.relPath(filePath))

	w.Header().Set("Content-Type", contentType)
	// An SVG opened directly is a document on mdserver's origin; keep its scripts and forms from
	// running. form-action has no default-src fallback, so it must be set on its own.
	w.Header().Set("Content-Security-Policy", imageCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")

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

// imageTypes lists the image extensions served from the root, with their MIME types.
// Markdown is rendered separately; any other file under the root gets 404.
var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
}

// imageCSP is sent with every image served from the root. It blocks scripts and forms, and
// allows the inline styles and data: images and fonts that self-contained SVG diagrams use.
const imageCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; form-action 'none'"

// imageContentType returns the MIME type for an image extension, ignoring case.
func imageContentType(ext string) (string, bool) {
	ct, ok := imageTypes[strings.ToLower(ext)]
	return ct, ok
}
