package server

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"time"
)

// mdserver's page templates, stylesheet and favicon are embedded and never read
// from disk, so pages look the same wherever the binary runs.
//
//go:embed template/*.html
var templateFS embed.FS

var (
	//go:embed template/style.css
	styleCSS []byte

	//go:embed template/favicon.svg
	faviconSVG []byte
)

// templates holds a server's parsed page templates.
type templates struct {
	page      *template.Template
	directory *template.Template
	settings  *template.Template
}

// parseTemplates parses the embedded page templates, adding the live reload
// client script when liveReload is set. Pages show version in the header.
func parseTemplates(liveReload bool, version string) templates {
	parse := func(name string) *template.Template {
		src := string(mustReadTemplateFile(name + ".html"))
		if liveReload {
			src = injectLiveReloadScript(src)
		}
		funcs := template.FuncMap{"version": func() string { return version }}
		return template.Must(template.New(name).Funcs(funcs).Parse(src))
	}
	return templates{
		page:      parse("page"),
		directory: parse("directory"),
		settings:  parse("settings"),
	}
}

// serveEmbedded writes an embedded file, answering range requests as
// http.ServeFile would. It sends no Last-Modified, since nothing is cached.
func serveEmbedded(w http.ResponseWriter, r *http.Request, name string, content []byte) {
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}

func mustReadTemplateFile(name string) []byte {
	b, err := templateFS.ReadFile("template/" + name)
	if err != nil {
		panic(err)
	}
	return b
}
