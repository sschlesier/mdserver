package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestServeMarkdownContent(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "mdserver-content-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test markdown file with various markdown features
	testFile := filepath.Join(tmpDir, "test.md")
	markdownContent := "# Test Page\n\n" +
		"This is a test page with **bold text** and *italic text*.\n\n" +
		"## Section 1\n\n" +
		"Some content here with a [link](https://example.com).\n\n" +
		"### Code Example\n\n" +
		"Here's some `inline code` and a code block:\n\n" +
		"```go\n" +
		"package main\n" +
		"func main() {\n" +
		"    println(\"Hello, World!\")\n" +
		"}\n" +
		"```\n\n" +
		"## Lists\n\n" +
		"- Item 1\n" +
		"- Item 2\n" +
		"- Item 3\n\n" +
		"1. First item\n" +
		"2. Second item\n" +
		"3. Third item\n"
	if err := os.WriteFile(testFile, []byte(markdownContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Find an available port
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	// Create and configure server
	config := Config{
		Host:             "localhost",
		Port:             port,
		RootDir:          tmpDir,
		EnableLiveReload: false, // Not needed for content test
	}

	srv := NewServer(config)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Ensure server is stopped at the end
	defer func() {
		srv.Stop()
		time.Sleep(50 * time.Millisecond)
	}()

	baseURL := "http://localhost:" + strconv.Itoa(port)

	// Test 1: Fetch the markdown file and verify HTML structure
	t.Run("HTML structure and content", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/test.md")
		if err != nil {
			t.Fatalf("Failed to fetch test file: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		// Verify Content-Type
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			t.Errorf("Expected text/html Content-Type, got %s", contentType)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		html := string(body)

		// Verify HTML structure
		if !strings.Contains(html, "<!DOCTYPE html>") {
			t.Error("Response should contain DOCTYPE declaration")
		}

		if !strings.Contains(html, "<html") {
			t.Error("Response should contain html tag")
		}

		if !strings.Contains(html, "<body") {
			t.Error("Response should contain body tag")
		}

		// Verify container div exists
		if !strings.Contains(html, `class="container"`) {
			t.Error("Response should contain container div")
		}

		// Verify title
		if !strings.Contains(html, "<title>Test Page</title>") {
			t.Error("Response should contain correct title")
		}

		// Verify markdown content is rendered
		if !strings.Contains(html, "<h1") || !strings.Contains(html, "Test Page") {
			t.Error("Response should contain rendered h1 heading")
		}

		if !strings.Contains(html, "<h2") || !strings.Contains(html, "Section 1") {
			t.Error("Response should contain rendered h2 heading")
		}

		// Verify bold text is rendered
		if !strings.Contains(html, "<strong>") && !strings.Contains(html, "<b>") {
			t.Error("Response should contain rendered bold text")
		}

		// Verify italic text is rendered
		if !strings.Contains(html, "<em>") && !strings.Contains(html, "<i>") {
			t.Error("Response should contain rendered italic text")
		}

		// Verify links are rendered
		if !strings.Contains(html, `<a href="https://example.com">`) {
			t.Error("Response should contain rendered link")
		}

		// Verify code blocks are rendered
		if !strings.Contains(html, `<pre><code`) || !strings.Contains(html, `package main`) {
			t.Error("Response should contain rendered code block")
		}

		// Verify lists are rendered
		if !strings.Contains(html, "<ul>") || !strings.Contains(html, "<li>") {
			t.Error("Response should contain rendered unordered list")
		}

		if !strings.Contains(html, "<ol>") {
			t.Error("Response should contain rendered ordered list")
		}
	})

	// Test 2: Verify CSS is linked in HTML
	t.Run("CSS link in HTML", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/test.md")
		if err != nil {
			t.Fatalf("Failed to fetch test file: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		html := string(body)

		// Verify CSS link is present
		if !strings.Contains(html, `<link rel="stylesheet" href="/assets/style.css">`) {
			t.Error("HTML should contain link to stylesheet")
		}
	})

	// Test 3: Verify CSS file is served and contains expected styles
	t.Run("CSS file is served", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/assets/style.css")
		if err != nil {
			t.Fatalf("Failed to fetch CSS file: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		// Verify Content-Type
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/css") {
			t.Errorf("Expected text/css Content-Type, got %s", contentType)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read CSS body: %v", err)
		}

		css := string(body)

		// Verify CSS contains basic expected rules (works for both full CSS and default CSS)
		expectedCSSRules := []string{
			"body",
			"code",
			"pre",
		}

		for _, rule := range expectedCSSRules {
			if !strings.Contains(css, rule) {
				t.Errorf("CSS should contain rule for %s", rule)
			}
		}

		// Check if this is the full CSS (from template/style.css) or default CSS
		isDefaultCSS := strings.Contains(css, "Default CSS")
		
		if isDefaultCSS {
			// For default CSS, verify it has basic styling
			if !strings.Contains(css, "max-width") || !strings.Contains(css, "padding") {
				t.Error("Default CSS should contain basic styling")
			}
		} else {
			// For full CSS, verify advanced features
			advancedRules := []string{
				".container",
				"h1",
				"h2",
				".breadcrumbs",
			}
			for _, rule := range advancedRules {
				if !strings.Contains(css, rule) {
					t.Errorf("Full CSS should contain rule for %s", rule)
				}
			}

			// Verify CSS variables are present (for theming)
			if !strings.Contains(css, ":root") || !strings.Contains(css, "--bg-color") {
				t.Error("Full CSS should contain CSS variables for theming")
			}
		}
	})

	// Test 4: Verify breadcrumbs are rendered
	t.Run("Breadcrumbs navigation", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/test.md")
		if err != nil {
			t.Fatalf("Failed to fetch test file: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		html := string(body)

		// Verify breadcrumbs structure
		if !strings.Contains(html, `class="breadcrumbs"`) {
			t.Error("HTML should contain breadcrumbs navigation")
		}

		// Verify breadcrumb links
		if !strings.Contains(html, `<a href="/">`) {
			t.Error("HTML should contain root breadcrumb link")
		}

		if !strings.Contains(html, `<a href="/test.md">`) {
			t.Error("HTML should contain file breadcrumb link")
		}
	})
}

func TestServeMarkdownWithMermaid(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "mdserver-mermaid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test markdown file with mermaid diagram
	testFile := filepath.Join(tmpDir, "diagram.md")
	markdownContent := "# Mermaid Test\n\n" +
		"Here's a mermaid diagram:\n\n" +
		"```mermaid\n" +
		"graph TD\n" +
		"    A --> B\n" +
		"    B --> C\n" +
		"```\n"
	if err := os.WriteFile(testFile, []byte(markdownContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Find an available port
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	// Create and configure server
	config := Config{
		Host:             "localhost",
		Port:             port,
		RootDir:          tmpDir,
		EnableLiveReload: false,
	}

	srv := NewServer(config)

	// Start server in a goroutine
	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)
	defer func() {
		srv.Stop()
		time.Sleep(50 * time.Millisecond)
	}()

	baseURL := "http://localhost:" + strconv.Itoa(port)

	resp, err := http.Get(baseURL + "/diagram.md")
	if err != nil {
		t.Fatalf("Failed to fetch test file: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	html := string(body)

	// Verify mermaid script is included
	if !strings.Contains(html, "mermaid.min.js") {
		t.Error("HTML should include mermaid.js script")
	}

	// Verify mermaid diagram is converted (not a code block)
	if strings.Contains(html, `<pre><code class="language-mermaid">`) {
		t.Error("Mermaid diagram should be converted to div, not remain as code block")
	}

	// Verify mermaid div exists
	if !strings.Contains(html, `<div class="mermaid">`) {
		t.Error("HTML should contain mermaid div")
	}

	// Verify mermaid initialization script
	if !strings.Contains(html, "mermaid.initialize") {
		t.Error("HTML should contain mermaid initialization script")
	}
}

func TestServeDirectoryIndex(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "mdserver-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectory and files
	subDir := filepath.Join(tmpDir, "docs")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	file1 := filepath.Join(tmpDir, "readme.md")
	file2 := filepath.Join(subDir, "guide.md")

	os.WriteFile(file1, []byte("# Readme\n"), 0644)
	os.WriteFile(file2, []byte("# Guide\n"), 0644)

	// Find an available port
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	// Create and configure server
	config := Config{
		Host:             "localhost",
		Port:             port,
		RootDir:          tmpDir,
		EnableLiveReload: false,
	}

	srv := NewServer(config)

	// Start server in a goroutine
	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)
	defer func() {
		srv.Stop()
		time.Sleep(50 * time.Millisecond)
	}()

	baseURL := "http://localhost:" + strconv.Itoa(port)

	// Test root directory listing
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Failed to fetch root directory: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	html := string(body)

	// Verify directory listing structure
	if !strings.Contains(html, `class="directory-listing"`) {
		t.Error("HTML should contain directory listing")
	}

	// Verify files are listed
	if !strings.Contains(html, "readme.md") {
		t.Error("Directory listing should contain readme.md")
	}

	// Verify subdirectories are listed
	if !strings.Contains(html, "docs") {
		t.Error("Directory listing should contain docs subdirectory")
	}

	// Verify CSS is linked
	if !strings.Contains(html, `<link rel="stylesheet" href="/assets/style.css">`) {
		t.Error("Directory listing should include CSS link")
	}
}
