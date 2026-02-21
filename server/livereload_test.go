package server

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// findAvailablePort finds an available port for testing
func findAvailablePort() (int, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func TestLiveReloadIntegration(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "mdserver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test markdown file
	testFile := filepath.Join(tmpDir, "test.md")
	initialContent := "# Test Page\n\nInitial content.\n"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
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
		EnableLiveReload: true,
	}

	srv := NewServer(config)
	if srv.liveReload == nil {
		t.Fatal("LiveReload was not initialized")
	}

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
		// Wait a bit for cleanup
		time.Sleep(50 * time.Millisecond)
	}()

	// Verify server is running by fetching the test file
	baseURL := "http://localhost:" + strconv.Itoa(port)
	resp, err := http.Get(baseURL + "/test.md")
	if err != nil {
		t.Fatalf("Failed to fetch test file: %v", err)
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

	if !strings.Contains(string(body), "Initial content") {
		t.Errorf("Response body doesn't contain initial content. Got: %s", string(body))
	}

	// Connect to WebSocket endpoint
	wsURL := "ws://localhost:" + strconv.Itoa(port) + "/livereload"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()
	defer resp.Body.Close()

	// Set read deadline to avoid hanging
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Modify the markdown file
	updatedContent := initialContent + "\n## New Section\n\nThis was added to test livereload.\n"
	if err := os.WriteFile(testFile, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	// Wait for file system event to be processed
	time.Sleep(200 * time.Millisecond)

	// Read message from WebSocket
	messageType, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	if messageType != websocket.TextMessage {
		t.Errorf("Expected text message, got %d", messageType)
	}

	if string(message) != "reload" {
		t.Errorf("Expected 'reload' message, got %q", string(message))
	}

	// Verify the updated content is served
	resp2, err := http.Get(baseURL + "/test.md")
	if err != nil {
		t.Fatalf("Failed to fetch updated file: %v", err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("Failed to read updated response body: %v", err)
	}

	if !strings.Contains(string(body2), "New Section") {
		t.Errorf("Updated content not found in response. Got: %s", string(body2))
	}
}

func TestEnsureWatchingDeepDirectory(t *testing.T) {
	// Create a temporary directory with a deeply nested structure
	tmpDir, err := os.MkdirTemp("", "mdserver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a directory beyond the initial watch depth of 1
	deepDir := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatalf("Failed to create deep directory: %v", err)
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
		EnableLiveReload: true,
	}

	srv := NewServer(config)
	if srv.liveReload == nil {
		t.Fatal("LiveReload was not initialized")
	}

	// Start server in a goroutine
	go func() {
		_ = srv.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	defer func() {
		srv.Stop()
		time.Sleep(50 * time.Millisecond)
	}()

	// Connect to WebSocket
	wsURL := "ws://localhost:" + strconv.Itoa(port) + "/livereload"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Wait for connection to register
	time.Sleep(100 * time.Millisecond)

	// Write a .md file in the deep directory — should NOT trigger reload
	// because initial watch depth is 1 (covers only tmpDir + tmpDir/a)
	deepFile := filepath.Join(deepDir, "test.md")
	if err := os.WriteFile(deepFile, []byte("# Deep Test\n"), 0644); err != nil {
		t.Fatalf("Failed to write deep file: %v", err)
	}

	// Wait briefly and check no message arrives
	time.Sleep(300 * time.Millisecond)

	// Now call EnsureWatching on the deep directory
	srv.liveReload.EnsureWatching(deepDir)

	// Wait for watcher to settle
	time.Sleep(100 * time.Millisecond)

	// Modify the file — should now trigger a reload
	if err := os.WriteFile(deepFile, []byte("# Deep Test Updated\n"), 0644); err != nil {
		t.Fatalf("Failed to update deep file: %v", err)
	}

	// Read message from WebSocket
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message after EnsureWatching: %v", err)
	}

	if string(message) != "reload" {
		t.Errorf("Expected 'reload' message, got %q", string(message))
	}
}

func TestLiveReloadMultipleClients(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "mdserver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test markdown file
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0644); err != nil {
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
		EnableLiveReload: true,
	}

	srv := NewServer(config)
	if srv.liveReload == nil {
		t.Fatal("LiveReload was not initialized")
	}

	// Start server in a goroutine
	go func() {
		_ = srv.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Ensure server is stopped at the end
	defer func() {
		srv.Stop()
		time.Sleep(50 * time.Millisecond)
	}()

	// Connect multiple WebSocket clients
	wsURL := "ws://localhost:" + strconv.Itoa(port) + "/livereload"
	
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect first client: %v", err)
	}
	defer conn1.Close()
	conn1.SetReadDeadline(time.Now().Add(5 * time.Second))

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect second client: %v", err)
	}
	defer conn2.Close()
	conn2.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Wait a bit for connections to be registered
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(testFile, []byte("# Updated\n"), 0644); err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	// Wait for file system event
	time.Sleep(200 * time.Millisecond)

	// Both clients should receive the reload message
	msg1 := make(chan []byte, 1)
	msg2 := make(chan []byte, 1)
	err1 := make(chan error, 1)
	err2 := make(chan error, 1)

	go func() {
		_, m, err := conn1.ReadMessage()
		if err != nil {
			err1 <- err
		} else {
			msg1 <- m
		}
	}()

	go func() {
		_, m, err := conn2.ReadMessage()
		if err != nil {
			err2 <- err
		} else {
			msg2 <- m
		}
	}()

	// Wait for both messages or timeout
	select {
	case m := <-msg1:
		if string(m) != "reload" {
			t.Errorf("Client 1: Expected 'reload', got %q", string(m))
		}
	case err := <-err1:
		t.Errorf("Client 1 error: %v", err)
	case <-time.After(2 * time.Second):
		t.Error("Client 1: Timeout waiting for reload message")
	}

	select {
	case m := <-msg2:
		if string(m) != "reload" {
			t.Errorf("Client 2: Expected 'reload', got %q", string(m))
		}
	case err := <-err2:
		t.Errorf("Client 2 error: %v", err)
	case <-time.After(2 * time.Second):
		t.Error("Client 2: Timeout waiting for reload message")
	}
}
