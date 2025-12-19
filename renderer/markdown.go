package renderer

import (
	"bytes"

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
	return buf.Bytes(), nil
}

// NewRenderer returns a new goldmark instance (for testing or custom configuration)
func NewRenderer() goldmark.Markdown {
	return mdRenderer
}

