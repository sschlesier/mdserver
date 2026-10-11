package server

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"os"
	"time"
)

// templateFS holds mdserver's stylesheet, page templates and favicon. They are
// never read from disk, so pages look the same wherever the binary runs.
//
//go:embed template
var templateFS embed.FS

var (
	//go:embed template/style.css
	styleCSS []byte

	//go:embed template/favicon.svg
	faviconSVG []byte

	// embeddedModTime is the Last-Modified time for the embedded files.
	embeddedModTime = binaryModTime()
)

// binaryModTime returns the modification time of the running binary, which holds the
// embedded files, or the current time if it can't be found.
func binaryModTime() time.Time {
	exe, err := os.Executable()
	if err != nil {
		return time.Now()
	}
	info, err := os.Stat(exe)
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

// templates holds a server's parsed page templates.
type templates struct {
	page      *template.Template
	directory *template.Template
	settings  *template.Template
}

// parseTemplates parses the embedded page templates, adding the live reload
// client script when liveReload is set.
func parseTemplates(liveReload bool) templates {
	parse := func(name string) *template.Template {
		src := string(mustReadTemplateFile(name + ".html"))
		if liveReload {
			src = injectLiveReloadScript(src)
		}
		return template.Must(template.New(name).Parse(src))
	}
	return templates{
		page:      parse("page"),
		directory: parse("directory"),
		settings:  parse("settings"),
	}
}

// serveEmbedded writes an embedded file, answering conditional and range
// requests as http.ServeFile would.
func serveEmbedded(w http.ResponseWriter, r *http.Request, name string, content []byte) {
	http.ServeContent(w, r, name, embeddedModTime, bytes.NewReader(content))
}

func mustReadTemplateFile(name string) []byte {
	b, err := templateFS.ReadFile("template/" + name)
	if err != nil {
		panic(err)
	}
	return b
}
