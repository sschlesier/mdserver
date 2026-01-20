package renderer

import (
	"bytes"
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var mdRenderer goldmark.Markdown

func init() {
	// Initialize goldmark with GitHub Flavored Markdown extensions
	mdRenderer = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // GitHub Flavored Markdown (tables, strikethrough, task lists, autolinks)
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}

// RenderMarkdown converts markdown content to HTML
func RenderMarkdown(markdown []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := mdRenderer.Convert(markdown, &buf); err != nil {
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
