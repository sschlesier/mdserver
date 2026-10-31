package renderer

import (
	"bytes"
	_ "embed"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

//go:embed standalone.css
var standaloneCSS string

var mdRenderer goldmark.Markdown

func init() {
	mdRenderer = newGoldmark()
}

// newGoldmark builds a goldmark instance with GitHub Flavored Markdown
// (tables, strikethrough, task lists, autolinks); extra parser options are
// appended to the common ones.
func newGoldmark(extra ...parser.Option) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(append([]parser.Option{parser.WithAutoHeadingID()}, extra...)...),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}

// baseRewriter prefixes root-relative link and image destinations with base.
type baseRewriter struct {
	base []byte
}

func (b baseRewriter) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest *[]byte
		switch v := n.(type) {
		case *ast.Link:
			dest = &v.Destination
		case *ast.Image:
			dest = &v.Destination
		default:
			return ast.WalkContinue, nil
		}
		d := *dest
		if len(d) > 0 && d[0] == '/' && !bytes.HasPrefix(d, []byte("//")) {
			*dest = append(append([]byte{}, b.base...), d...)
		}
		return ast.WalkContinue, nil
	})
}

var (
	baseMu        sync.Mutex
	baseRenderers = map[string]goldmark.Markdown{}
)

// rendererForBase returns the shared renderer for an empty base, or a cached
// renderer that rewrites root-relative destinations under base.
func rendererForBase(base string) goldmark.Markdown {
	if base == "" {
		return mdRenderer
	}
	baseMu.Lock()
	defer baseMu.Unlock()
	if r, ok := baseRenderers[base]; ok {
		return r
	}
	r := newGoldmark(parser.WithASTTransformers(
		util.Prioritized(baseRewriter{base: []byte(base)}, 100),
	))
	baseRenderers[base] = r
	return r
}

// RenderMarkdown converts markdown content to HTML
func RenderMarkdown(markdown []byte) ([]byte, error) {
	return RenderMarkdownWithBase(markdown, "")
}

// RenderMarkdownWithBase converts markdown to HTML, rewriting link and image
// destinations that start with a single "/" to live under base. base is empty
// or a path with a leading slash and no trailing slash, such as "/notes".
func RenderMarkdownWithBase(markdown []byte, base string) ([]byte, error) {
	var buf bytes.Buffer
	if err := rendererForBase(base).Convert(markdown, &buf); err != nil {
		return nil, err
	}
	htmlContent := buf.Bytes()
	// Post-process to convert mermaid code blocks to div.mermaid elements
	htmlContent = processMermaidBlocks(htmlContent)
	return htmlContent, nil
}

// processMermaidBlocks converts mermaid code blocks to div.mermaid elements for Mermaid.js rendering
func processMermaidBlocks(htmlContent []byte) []byte {
	// Pattern to match: <pre><code class="language-mermaid">...</code></pre>
	// Also handles case-insensitive matching and variations in whitespace
	pattern := regexp.MustCompile(`(?i)<pre><code\s+class=["']language-mermaid["']>([\s\S]*?)</code></pre>`)

	result := pattern.ReplaceAllFunc(htmlContent, func(match []byte) []byte {
		// Extract the content between the code tags
		submatches := pattern.FindSubmatch(match)
		if len(submatches) < 2 {
			return match // Return original if pattern doesn't match as expected
		}

		mermaidCode := submatches[1]
		// Trim leading/trailing whitespace but preserve internal formatting
		mermaidCode = bytes.TrimSpace(mermaidCode)

		// Note: We don't HTML-escape the mermaid code because Mermaid.js needs to parse
		// the raw text content. The content is already safe since it was inside a <code>
		// block and will be rendered as text content of the div.

		// Build the replacement div
		var result bytes.Buffer
		result.WriteString(`<div class="mermaid">`)
		result.Write(mermaidCode)
		result.WriteString(`</div>`)

		return result.Bytes()
	})

	return result
}

// NewRenderer returns a new goldmark instance (for testing or custom configuration)
func NewRenderer() goldmark.Markdown {
	return mdRenderer
}

// RenderStandalone converts markdown to a complete standalone HTML document
func RenderStandalone(markdown []byte, filename string) ([]byte, error) {
	// Render markdown content
	content, err := RenderMarkdown(markdown)
	if err != nil {
		return nil, err
	}

	// Extract title from first H1 or use filename
	title := extractTitle(markdown, filename)

	// Build standalone HTML document
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>`)
	buf.WriteString(escapeHTML(title))
	buf.WriteString(`</title>
	<style>
`)
	buf.WriteString(standaloneCSS)
	buf.WriteString(`	</style>
	<script src="https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js"></script>
</head>
<body>
	<div class="container">
		`)
	buf.Write(content)
	buf.WriteString(`
	</div>
	<script>
		mermaid.initialize({ startOnLoad: true, theme: 'default' });
	</script>
</body>
</html>
`)

	return buf.Bytes(), nil
}

// extractTitle extracts the title from the first H1 heading or uses the filename
func extractTitle(markdown []byte, filename string) string {
	// Look for first H1 heading (# Title)
	lines := strings.Split(string(markdown), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}

	// Fall back to filename without extension
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

// escapeHTML escapes special HTML characters in a string
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
