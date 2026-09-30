package server

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a log destination that is safe to read while the server
// is still logging.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(io.MultiWriter(prev, buf))
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

const shutdownTimeoutLine = "Shutdown timed out after 2s; closing remaining connections"

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

func TestRootRenamedShutsDownViaWatcher(t *testing.T) {
	tmpDir, _, done := startRootTestServer(t, true)

	moved := tmpDir + ".old"
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.Rename(tmpDir, moved); err != nil {
		t.Fatalf("Failed to rename root: %v", err)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestRootRenamedAndReplacedShutsDown(t *testing.T) {
	tmpDir, _, done := startRootTestServer(t, true)

	moved := tmpDir + ".old"
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.Rename(tmpDir, moved); err != nil {
		t.Fatalf("Failed to rename root: %v", err)
	}
	if err := os.Mkdir(tmpDir, 0o755); err != nil {
		t.Fatalf("Failed to recreate root: %v", err)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestRootMissingShutdownTimesOutOnStuckRequest(t *testing.T) {
	logs := captureLog(t)

	tmpDir, err := os.MkdirTemp("", "mdserver-root-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	srv := NewServer(Config{Host: "localhost", Port: port, RootDir: tmpDir, EnableLiveReload: true})
	t.Cleanup(srv.Stop)

	entered := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	srv.mux.HandleFunc("/stuck", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
	})

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()
	time.Sleep(100 * time.Millisecond)

	clientErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://localhost:" + strconv.Itoa(port) + "/stuck")
		if err == nil {
			resp.Body.Close()
		}
		clientErr <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Stuck request never reached the handler")
	}

	start := time.Now()
	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 3*time.Second {
		t.Errorf("Start took %v to return, want under 3s", elapsed)
	}
	if !strings.Contains(logs.String(), shutdownTimeoutLine) {
		t.Errorf("Expected log line %q, got:\n%s", shutdownTimeoutLine, logs.String())
	}
	select {
	case err := <-clientErr:
		if err == nil {
			t.Error("Expected the stuck request's connection to be cut off, but it got a response")
		}
	case <-time.After(time.Second):
		t.Error("Stuck request's connection was not closed after the shutdown timeout")
	}
}

func TestRootMissingShutdownWithoutStuckRequestLogsNoTimeout(t *testing.T) {
	logs := captureLog(t)
	tmpDir, _, done := startRootTestServer(t, true)

	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
	if strings.Contains(logs.String(), "timed out") {
		t.Errorf("Unexpected timeout log line:\n%s", logs.String())
	}
}

func assertLiveReloadStopped(t *testing.T, srv *Server) {
	t.Helper()
	select {
	case <-srv.liveReload.stopChan:
	default:
		t.Error("Expected live reload to be stopped")
	}
}

func TestStopTwiceSequentially(t *testing.T) {
	srv := NewServer(Config{Host: "localhost", RootDir: t.TempDir(), EnableLiveReload: true})
	if srv.liveReload == nil {
		t.Fatal("Expected live reload to be enabled")
	}

	srv.Stop()
	assertLiveReloadStopped(t, srv)
	srv.Stop()
}

func TestStopConcurrently(t *testing.T) {
	srv := NewServer(Config{Host: "localhost", RootDir: t.TempDir(), EnableLiveReload: true})
	if srv.liveReload == nil {
		t.Fatal("Expected live reload to be enabled")
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.Stop()
		}()
	}
	wg.Wait()
	assertLiveReloadStopped(t, srv)
}
