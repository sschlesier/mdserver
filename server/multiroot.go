package server

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
)

// RootEntry is one root on the multi-root landing page.
type RootEntry struct {
	Name        string
	Href        string
	Unavailable bool
}

// homeBreadcrumb is the home icon link to /.
var homeBreadcrumb = Breadcrumb{
	Href: "/",
	Text: template.HTML(`<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style="vertical-align: middle; display: inline-block;"><path d="M8 0L0 7h2v9h5v-6h2v6h5V7h2L8 0z"/></svg>/`),
}

// available reports whether the root can still serve. The first time its
// directory is seen missing the root becomes unavailable for good: it stays so
// until restart, even if the directory comes back.
func (rt *root) available() bool {
	if rt.gone.Load() {
		return false
	}
	if !rt.exists() {
		rt.gone.Store(true)
		return false
	}
	return true
}

// serveRootMissing answers a request under an unavailable root with 410.
func serveRootMissing(w http.ResponseWriter, rt *root) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusGone)
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Folder Missing</title></head><body style="font-family: sans-serif; margin: 3em;"><h1>Folder missing</h1><p><code>%s</code> (<code>%s</code>) no longer exists. The other roots keep serving.</p><p><a href="/">Back to the list of roots</a></p></body></html>`,
		html.EscapeString(rt.dir), html.EscapeString(rt.name))
}

// handleRoots serves the multi-root landing page: every root, in config order.
func (s *Server) handleRoots(w http.ResponseWriter, r *http.Request) {
	entries := make([]RootEntry, len(s.roots))
	for i, rt := range s.roots {
		entries[i] = RootEntry{
			Name:        rt.name,
			Href:        rt.prefix + "/",
			Unavailable: !rt.available(),
		}
	}

	data := struct {
		Title       string
		Breadcrumbs []Breadcrumb
		Roots       []RootEntry
	}{
		Title:       "Roots",
		Breadcrumbs: []Breadcrumb{homeBreadcrumb},
		Roots:       entries,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.roots.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// handleLiveReload upgrades a multi-root live reload connection to the
// LiveReload of the root named by the "root" query parameter. An unknown or
// missing root, or one without live reload, is a 404.
func (s *Server) handleLiveReload(w http.ResponseWriter, r *http.Request) {
	rt := s.rootNamed(r.URL.Query().Get("root"))
	if rt == nil || rt.liveReload == nil {
		http.NotFound(w, r)
		return
	}
	rt.liveReload.HandleWebSocket(w, r)
}
