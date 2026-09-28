package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startRootTestServer(t *testing.T, liveReload bool) (string, string, chan error) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mdserver-root-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	srv := NewServer(Config{Host: "localhost", Port: port, RootDir: tmpDir, EnableLiveReload: liveReload})
	t.Cleanup(srv.Stop)

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()
	time.Sleep(100 * time.Millisecond)

	return tmpDir, "http://localhost:" + strconv.Itoa(port), done
}

func waitForStart(t *testing.T, done chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after root directory was removed")
		return nil
	}
}

func TestRequestAfterRootRemovedRepliesAndShutsDown(t *testing.T) {
	tmpDir, baseURL, done := startRootTestServer(t, false)

	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Errorf("Expected status %d, got %d", http.StatusGone, resp.StatusCode)
	}
	if !strings.Contains(string(body), "no longer exists") || !strings.Contains(string(body), "shutting down") {
		t.Errorf("Expected missing-folder shutdown notice, got: %s", body)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestRootRemovedWithoutRequestShutsDownViaWatcher(t *testing.T) {
	tmpDir, _, done := startRootTestServer(t, true)

	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestRootPresentKeepsServing(t *testing.T) {
	_, baseURL, done := startRootTestServer(t, true)

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	select {
	case err := <-done:
		t.Fatalf("Server stopped unexpectedly: %v", err)
	default:
	}
}
