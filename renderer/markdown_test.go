package renderer

import (
	"strings"
	"testing"
)

func TestProcessMermaidBlocks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:  "single mermaid block",
			input: `<pre><code class="language-mermaid">graph TD
    A --> B
</code></pre>`,
			expected: `<div class="mermaid">graph TD
    A --> B</div>`,
		},
		{
			name:  "mermaid block with case-insensitive class",
			input: `<pre><code class="language-Mermaid">graph TD
    A --> B
</code></pre>`,
			expected: `<div class="mermaid">graph TD
    A --> B</div>`,
		},
		{
			name:  "empty mermaid block",
			input: `<pre><code class="language-mermaid"></code></pre>`,
			expected: `<div class="mermaid"></div>`,
		},
		{
			name:  "mermaid block with special characters",
			input: `<pre><code class="language-mermaid">graph TD
    A["Node &lt;test&gt;"] --> B
</code></pre>`,
			expected: `<div class="mermaid">graph TD
    A["Node &lt;test&gt;"] --> B</div>`,
		},
		{
			name:  "mermaid block with leading/trailing whitespace",
			input: `<pre><code class="language-mermaid">    graph TD
    A --> B
</code></pre>`,
			expected: `<div class="mermaid">graph TD
    A --> B</div>`,
		},
		{
			name:  "multiple mermaid blocks",
			input: `<pre><code class="language-mermaid">graph TD
    A --> B
</code></pre>
<p>Some text</p>
<pre><code class="language-mermaid">sequenceDiagram
    A->>B: Hello
</code></pre>`,
			expected: `<div class="mermaid">graph TD
    A --> B</div>
<p>Some text</p>
<div class="mermaid">sequenceDiagram
    A->>B: Hello</div>`,
		},
		{
			name:  "non-mermaid code block unchanged",
			input: `<pre><code class="language-go">package main
func main() {}
</code></pre>`,
			expected: `<pre><code class="language-go">package main
func main() {}
</code></pre>`,
		},
		{
			name:  "mixed content",
			input: `<h1>Title</h1>
<pre><code class="language-python">print("hello")
</code></pre>
<pre><code class="language-mermaid">graph TD
    A --> B
</code></pre>
<p>More text</p>`,
			expected: `<h1>Title</h1>
<pre><code class="language-python">print("hello")
</code></pre>
<div class="mermaid">graph TD
    A --> B</div>
<p>More text</p>`,
		},
		{
			name:  "mermaid block with HTML entities",
			input: `<pre><code class="language-mermaid">graph TD
    A["Test &amp; More"] --> B
</code></pre>`,
			expected: `<div class="mermaid">graph TD
    A["Test &amp; More"] --> B</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processMermaidBlocks([]byte(tt.input))
			resultStr := string(result)

			// Normalize whitespace for comparison
			resultStr = normalizeWhitespace(resultStr)
			expectedStr := normalizeWhitespace(tt.expected)

			if resultStr != expectedStr {
				t.Errorf("processMermaidBlocks() = %q, want %q", resultStr, expectedStr)
			}
		})
	}
}

func TestRenderMarkdownWithMermaid(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		check    func(t *testing.T, html string)
	}{
		{
			name: "mermaid block in markdown",
			markdown: "# Test\n\n" +
				"Some text before.\n\n" +
				"```mermaid\n" +
				"graph TD\n" +
				"    A --> B\n" +
				"```\n\n" +
				"Some text after.",
			check: func(t *testing.T, html string) {
				if !strings.Contains(html, `<div class="mermaid">`) {
					t.Error("Expected to find mermaid div in output")
				}
				if !strings.Contains(html, "graph TD") {
					t.Error("Expected to find mermaid diagram content")
				}
				if strings.Contains(html, `<pre><code class="language-mermaid">`) {
					t.Error("Found unconverted mermaid code block")
				}
					// Verify other markdown still renders
					if !strings.Contains(html, "<h1") {
						t.Error("Expected h1 tag from markdown")
					}
				if !strings.Contains(html, "<p>") {
					t.Error("Expected p tags from markdown")
				}
			},
		},
		{
			name: "multiple mermaid blocks",
			markdown: "# Test\n\n" +
				"```mermaid\n" +
				"graph TD\n" +
				"    A --> B\n" +
				"```\n\n" +
				"```mermaid\n" +
				"sequenceDiagram\n" +
				"    A->>B: Hello\n" +
				"```",
			check: func(t *testing.T, html string) {
				// Count occurrences of mermaid divs
				count := strings.Count(html, `<div class="mermaid">`)
				if count != 2 {
					t.Errorf("Expected 2 mermaid divs, found %d", count)
				}
			},
		},
		{
			name: "mixed code blocks",
			markdown: "# Test\n\n" +
				"```go\n" +
				"package main\n" +
				"```\n\n" +
				"```mermaid\n" +
				"graph TD\n" +
				"    A --> B\n" +
				"```\n\n" +
				"```python\n" +
				"print(\"hello\")\n" +
				"```",
			check: func(t *testing.T, html string) {
				// Should have one mermaid div
				mermaidCount := strings.Count(html, `<div class="mermaid">`)
				if mermaidCount != 1 {
					t.Errorf("Expected 1 mermaid div, found %d", mermaidCount)
				}
				// Should have code blocks for go and python
				if !strings.Contains(html, `language-go`) {
					t.Error("Expected go code block")
				}
				if !strings.Contains(html, `language-python`) {
					t.Error("Expected python code block")
				}
			},
		},
		{
			name: "mermaid with GFM features",
			markdown: "# Test\n\n" +
				"| Column 1 | Column 2 |\n" +
				"|----------|---------|\n" +
				"| Value 1  | Value 2 |\n\n" +
				"```mermaid\n" +
				"graph TD\n" +
				"    A --> B\n" +
				"```",
			check: func(t *testing.T, html string) {
				// Verify table is rendered
				if !strings.Contains(html, "<table>") {
					t.Error("Expected table from GFM")
				}
				// Verify mermaid is converted
				if !strings.Contains(html, `<div class="mermaid">`) {
					t.Error("Expected mermaid div")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderMarkdown([]byte(tt.markdown))
			if err != nil {
				t.Fatalf("RenderMarkdown() error = %v", err)
			}

			html := string(result)
			tt.check(t, html)
		})
	}
}

// normalizeWhitespace normalizes whitespace in HTML for comparison
func normalizeWhitespace(s string) string {
	// Replace multiple spaces with single space
	s = strings.ReplaceAll(s, "  ", " ")
	// Replace newlines with spaces
	s = strings.ReplaceAll(s, "\n", " ")
	// Trim spaces
	s = strings.TrimSpace(s)
	return s
}

// Helper function to extract mermaid divs from HTML (for potential future use)
func extractMermaidDivs(html string) []string {
	var divs []string
	start := 0
	for {
		idx := strings.Index(html[start:], `<div class="mermaid">`)
		if idx == -1 {
			break
		}
		start += idx
		end := strings.Index(html[start:], `</div>`)
		if end == -1 {
			break
		}
		divs = append(divs, html[start:start+end+6])
		start += end + 6
	}
	return divs
}
