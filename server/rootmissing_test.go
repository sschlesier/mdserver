package server

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

	_, baseURL, done := startServerAt(t, tmpDir, liveReload)
	return tmpDir, baseURL, done
}

func startServerAt(t *testing.T, root string, liveReload bool) (*Server, string, chan error) {
	t.Helper()
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	srv := NewServer(Config{Host: "localhost", Port: port, RootDir: root, EnableLiveReload: liveReload})
	t.Cleanup(srv.Stop)

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()
	time.Sleep(100 * time.Millisecond)

	return srv, "http://localhost:" + strconv.Itoa(port), done
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

func TestMissingRootNoticeEscapesRootPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("'<' is not valid in Windows file names")
	}
	root := filepath.Join(t.TempDir(), "a<b&c")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}
	_, baseURL, done := startServerAt(t, root, false)

	if err := os.RemoveAll(root); err != nil {
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
	want := strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(root)
	if !strings.Contains(string(body), want) {
		t.Errorf("Expected body to contain escaped root %q, got: %s", want, body)
	}
	if strings.Contains(string(body), "a<b&c") {
		t.Errorf("Body contains the unescaped root name: %s", body)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestMissingRootNoticeIsPromptAndClosesConnection(t *testing.T) {
	tmpDir, baseURL, done := startRootTestServer(t, false)

	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}
	start := time.Now()
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	elapsed := time.Since(start)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Errorf("Expected status %d, got %d", http.StatusGone, resp.StatusCode)
	}
	if elapsed > time.Second {
		t.Errorf("410 took %v to arrive, want under 1s", elapsed)
	}
	// The client drops the Connection header and reports it as Close.
	if !resp.Close {
		t.Error("Expected the 410 response to carry Connection: close")
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestUnreadableRootParentIsNotTreatedAsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not make a directory unsearchable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}
	_, baseURL, done := startServerAt(t, root, false)

	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatalf("Failed to chmod parent: %v", err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusGone {
		t.Errorf("Got %d for a root that is unreadable, not missing", resp.StatusCode)
	}

	select {
	case err := <-done:
		t.Fatalf("Server stopped for an unreadable root: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRootReplacedByFileIsMissing(t *testing.T) {
	tmpDir, baseURL, done := startRootTestServer(t, false)

	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}
	if err := os.WriteFile(tmpDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("Failed to create file at root path: %v", err)
	}

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Errorf("Expected status %d, got %d", http.StatusGone, resp.StatusCode)
	}

	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
}

func TestStartWaitsForInFlightRequestOnRootMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mdserver-root-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	srv := NewServer(Config{Host: "localhost", Port: port, RootDir: tmpDir})
	t.Cleanup(srv.Stop)

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRequest := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRequest)
	srv.mux.HandleFunc("/inflight", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		io.WriteString(w, "finished")
	})

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()
	time.Sleep(100 * time.Millisecond)
	baseURL := "http://localhost:" + strconv.Itoa(port)

	type result struct {
		status int
		body   string
		err    error
	}
	inflight := make(chan result, 1)
	go func() {
		resp, err := http.Get(baseURL + "/inflight")
		if err != nil {
			inflight <- result{err: err}
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		inflight <- result{status: resp.StatusCode, body: string(body)}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("In-flight request never reached the handler")
	}

	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("Failed to remove root: %v", err)
	}
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("Expected status %d, got %d", http.StatusGone, resp.StatusCode)
	}

	select {
	case err := <-done:
		t.Fatalf("Start returned (%v) while a request was still in flight", err)
	case <-time.After(300 * time.Millisecond):
	}

	releaseRequest()
	if err := waitForStart(t, done); !errors.Is(err, ErrRootMissing) {
		t.Errorf("Expected ErrRootMissing, got %v", err)
	}
	select {
	case r := <-inflight:
		if r.err != nil || r.status != http.StatusOK || r.body != "finished" {
			t.Errorf("Expected the in-flight request to finish normally, got status %d body %q err %v", r.status, r.body, r.err)
		}
	case <-time.After(time.Second):
		t.Error("In-flight request never completed")
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
