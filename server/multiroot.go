package server

import (
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

// handleRoots serves the multi-root landing page: every root, in config order.
func (s *Server) handleRoots(w http.ResponseWriter, r *http.Request) {
	entries := make([]RootEntry, len(s.roots))
	for i, rt := range s.roots {
		entries[i] = RootEntry{
			Name:        rt.name,
			Href:        rt.prefix + "/",
			Unavailable: rt.unavailable(),
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
