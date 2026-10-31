package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
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

// newLiveReload creates the file watcher; tests replace it to simulate failure.
var newLiveReload = NewLiveReload

// Config holds server configuration
type Config struct {
	Host             string
	Port             int
	RootDir          string
	File             string
	EnableLiveReload bool
	Verbose          bool
	Version          string
}

// Server represents the HTTP server
type Server struct {
	config     Config
	mux        *http.ServeMux
	roots      []*root
	httpServer *http.Server
	templates  templates

	rootGone     chan struct{}
	rootGoneOnce sync.Once
	shutdownDone chan struct{}

	stopOnce sync.Once
}

// NewServer creates a new server instance serving config.RootDir as its only root.
func NewServer(config Config) *Server {
	s := newServerShell(config)
	s.roots = []*root{s.newRoot(config.RootDir, s.shutdownRootMissing)}
	s.finish()
	return s
}

// newServerShell returns a Server with no roots; callers add roots and call finish.
func newServerShell(config Config) *Server {
	s := &Server{
		config:       config,
		mux:          http.NewServeMux(),
		rootGone:     make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler: noCache(s.requireRoot(s.mux)),
	}
	return s
}

// finish parses the templates and registers the routes once roots is set.
func (s *Server) finish() {
	s.templates = parseTemplates(s.hasLiveReload(), s.config.Version)
	s.setupRoutes()
}

// newRoot creates a root for dir with its own LiveReload when live reload is
// enabled. onRemoved, if set, runs when dir itself is removed or renamed.
func (s *Server) newRoot(dir string, onRemoved func()) *root {
	rt := &root{dir: dir}
	if !s.config.EnableLiveReload {
		return rt
	}
	lr, err := newLiveReload(dir, s.config.Verbose)
	if err != nil {
		log.Printf("Failed to initialize LiveReload: %v", err)
		return rt
	}
	lr.onRootRemoved = onRemoved
	if err := lr.Start(); err != nil {
		log.Printf("Failed to start LiveReload: %v", err)
		return rt
	}
	rt.liveReload = lr
	return rt
}

// hasLiveReload reports whether any root has live reload running.
func (s *Server) hasLiveReload() bool {
	for _, rt := range s.roots {
		if rt.liveReload != nil {
			return true
		}
	}
	return false
}

// rootFor returns the root that serves r. With one root that is the only root.
func (s *Server) rootFor(r *http.Request) *root {
	return s.roots[0]
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
		for _, rt := range s.roots {
			if rt.liveReload != nil {
				rt.liveReload.Stop()
			}
		}
	})
}

// noCache tells browsers not to cache any response, so a tab always shows the
// current content.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setNoCache(w.Header())
		next.ServeHTTP(noCacheWriter{w}, r)
	})
}

func setNoCache(h http.Header) {
	h.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}

// noCacheWriter sets the no-cache headers again as the status is written:
// http.ServeContent and http.ServeFile delete Cache-Control before writing an
// error such as 416.
type noCacheWriter struct {
	http.ResponseWriter
}

func (w noCacheWriter) WriteHeader(code int) {
	setNoCache(w.Header())
	w.ResponseWriter.WriteHeader(code)
}

func (w noCacheWriter) Flush() {
	http.NewResponseController(w.ResponseWriter).Flush()
}

// Hijack lets the live reload WebSocket take over the connection.
func (w noCacheWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// ReadFrom keeps http.ServeFile's sendfile path for images.
func (w noCacheWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(w.ResponseWriter, r)
}

func (w noCacheWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// requireRoot answers every request with a shutdown notice once the root
// directory is gone, then shuts the server down. It watches roots[0] only,
// which is the single root.
func (s *Server) requireRoot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.roots[0].exists() {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusGone)
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Folder Missing</title></head><body style="font-family: sans-serif; margin: 3em;"><h1>Folder missing</h1><p><code>%s</code> no longer exists. The server is shutting down.</p><p>You can close this tab.</p></body></html>`, html.EscapeString(s.roots[0].dir))
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

	// File picker script and search, under the same prefix.
	s.mux.HandleFunc("/_mdserver/picker.js", s.servePickerJS)
	s.mux.HandleFunc("/_mdserver/files", s.handleFiles)

	// LiveReload WebSocket endpoint
	if lr := s.roots[0].liveReload; lr != nil {
		s.mux.HandleFunc("/livereload", lr.HandleWebSocket)
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
	serveEmbedded(w, r, "favicon.svg", faviconSVG)
}

// serveCSS serves mdserver's stylesheet
func (s *Server) serveCSS(w http.ResponseWriter, r *http.Request) {
	log.Printf("file: style.css (embedded)")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	serveEmbedded(w, r, "style.css", styleCSS)
}

// servePickerJS serves the file picker script
func (s *Server) servePickerJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	serveEmbedded(w, r, "picker.js", pickerJS)
}

// handleRequest serves paths under the root: directory listings, rendered Markdown and images.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	rt := s.rootFor(r)
	path, kind, status := rt.resolve(r.URL.Path)

	switch status {
	case http.StatusMovedPermanently:
		// Ensure directory paths end with / for consistency
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	case http.StatusForbidden:
		http.Error(w, "Invalid path", http.StatusForbidden)
		return
	case http.StatusOK:
	default:
		http.NotFound(w, r)
		return
	}

	switch kind {
	case kindDirectory:
		s.handleIndex(w, r, rt, path)
	case kindPage:
		s.handleMarkdown(w, r, rt, path)
	case kindImage:
		s.serveImage(w, r, rt, path)
	default:
		http.NotFound(w, r)
	}
}

// isPageFile reports whether a file name is one mdserver renders as a page.
func isPageFile(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".md")
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

// serveImage serves an allowlisted image that root.resolve found under the root.
func (s *Server) serveImage(w http.ResponseWriter, r *http.Request, rt *root, filePath string) {
	contentType, _ := imageContentType(filepath.Ext(filePath))

	log.Printf("file: %s", rt.relPath(filePath))

	w.Header().Set("Content-Type", contentType)
	// An SVG opened directly is a document on mdserver's origin; keep its scripts and forms from
	// running. form-action has no default-src fallback, so it must be set on its own.
	w.Header().Set("Content-Security-Policy", imageCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeFile(w, r, filePath)
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
