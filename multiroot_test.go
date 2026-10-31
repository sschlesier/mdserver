package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMultiRootConflict(t *testing.T) {
	tests := []struct {
		set    map[string]bool
		render bool
		want   string
	}{
		{map[string]bool{"multi-root": true}, false, ""},
		{map[string]bool{"multi-root": true, "port": true, "host": true, "no-open": true, "live-reload": true, "verbose": true}, false, ""},
		{map[string]bool{"multi-root": true, "dir": true}, false, "--dir"},
		{map[string]bool{"multi-root": true, "file": true}, false, "--file"},
		{map[string]bool{"multi-root": true, "render": true}, true, "--render"},
		{map[string]bool{"multi-root": true, "r": true}, true, "--render"},
	}
	for _, tt := range tests {
		if got := multiRootConflict(tt.set, tt.render); got != tt.want {
			t.Errorf("multiRootConflict(%v, %v) = %q, want %q", tt.set, tt.render, got, tt.want)
		}
	}
}

func TestListenPort(t *testing.T) {
	tests := []struct{ flag, config, want int }{
		{0, 0, 0},
		{0, 8099, 8099},
		{9000, 0, 9000},
		{9000, 8099, 9000},
	}
	for _, tt := range tests {
		if got := listenPort(tt.flag, tt.config); got != tt.want {
			t.Errorf("listenPort(%d, %d) = %d, want %d", tt.flag, tt.config, got, tt.want)
		}
	}
}

// buildBinary builds mdserver once per test into its own temp directory.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mdserver")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// runBinary runs the binary with an isolated HOME and XDG_CONFIG_HOME.
func runBinary(t *testing.T, bin, home, xdg string, args ...string) (stderr string, exit int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+xdg)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return errBuf.String(), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run: %v", err)
	}
	return errBuf.String(), ee.ExitCode()
}

func TestMultiRootCommandLineErrors(t *testing.T) {
	bin := buildBinary(t)
	home, xdg := t.TempDir(), t.TempDir()
	good := t.TempDir()
	cfgDir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(cfgDir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	for _, flag := range []string{"--dir=.", "--file=x.md", "--render"} {
		stderr, code := runBinary(t, bin, home, xdg, "--multi-root", flag)
		name := "--" + strings.TrimLeft(strings.SplitN(flag, "=", 2)[0], "-")
		if code != 2 || !strings.Contains(stderr, name) {
			t.Errorf("--multi-root %s: exit %d, stderr %q; want exit 2 naming %s", flag, code, stderr, name)
		}
	}

	stderr, code := runBinary(t, bin, home, xdg, "--multi-root")
	if code != 1 || !strings.Contains(stderr, filepath.Join(xdg, "mdserver", "roots")) || !strings.Contains(stderr, filepath.Join(home, ".config", "mdserver", "roots")) {
		t.Errorf("missing default config: exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); err == nil {
		t.Error("a missing config created ~/.config")
	}

	missing := filepath.Join(cfgDir, "nope")
	stderr, code = runBinary(t, bin, home, xdg, "--multi-root", missing)
	if code != 1 || !strings.Contains(stderr, missing) {
		t.Errorf("missing explicit config: exit %d, stderr %q", code, stderr)
	}

	bad := write("bad", "port 8099\na "+good+"\nb "+good+"\nbad ../x\n")
	stderr, code = runBinary(t, bin, home, xdg, "--multi-root", bad)
	if code != 1 || !strings.Contains(stderr, bad+":4: path must start with / or ~/") {
		t.Errorf("invalid line: exit %d, stderr %q", code, stderr)
	}

	empty := write("empty", "# nothing\n")
	stderr, code = runBinary(t, bin, home, xdg, "--multi-root", empty)
	if code != 1 || !strings.Contains(stderr, empty+":") {
		t.Errorf("zero roots: exit %d, stderr %q", code, stderr)
	}
}

// syncBuffer is a bytes.Buffer that can be read while a process writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startBinary starts the binary and waits until url answers.
func startBinary(t *testing.T, bin string, args []string, url string) (stderr *syncBuffer) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	stderr = &syncBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			return stderr
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not answer at %s; stderr: %s", url, stderr)
	return nil
}

func httpGetBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestMultiRootServesFromTheConfigPort(t *testing.T) {
	bin := buildBinary(t)
	a, b := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(a, "x.md"), []byte("# A\n"), 0644)
	os.WriteFile(filepath.Join(b, "x.md"), []byte("# B\n"), 0644)
	port := freePort(t)
	cfg := filepath.Join(t.TempDir(), "roots")
	os.WriteFile(cfg, []byte(fmt.Sprintf("port %d\na %s\nb %s\n", port, a, b)), 0644)

	base := fmt.Sprintf("http://localhost:%d", port)
	stderr := startBinary(t, bin, []string{"--multi-root", "--no-open", cfg}, base+"/")

	landing := httpGetBody(t, base+"/")
	if !strings.Contains(landing, `href="/a/"`) || !strings.Contains(landing, `href="/b/"`) {
		t.Errorf("landing page lacks the roots: %s", landing)
	}
	if !strings.Contains(httpGetBody(t, base+"/b/x.md"), "<h1") {
		t.Error("/b/x.md did not render")
	}
	for _, want := range []string{"Serving a: " + a, "Serving b: " + b, base + "/"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("startup output lacks %q:\n%s", want, stderr)
		}
	}
}

func TestMultiRootFlagPortBeatsConfigPort(t *testing.T) {
	bin := buildBinary(t)
	a := t.TempDir()
	cfgPort, flagPort := freePort(t), freePort(t)
	cfg := filepath.Join(t.TempDir(), "roots")
	os.WriteFile(cfg, []byte(fmt.Sprintf("port %d\na %s\n", cfgPort, a)), 0644)

	startBinary(t, bin, []string{"--multi-root", "--no-open", "--port", fmt.Sprint(flagPort), cfg}, fmt.Sprintf("http://localhost:%d/", flagPort))
	if resp, err := http.Get(fmt.Sprintf("http://localhost:%d/", cfgPort)); err == nil {
		resp.Body.Close()
		t.Errorf("the config port %d is listening although --port was given", cfgPort)
	}
}

func TestMultiRootAutoPort(t *testing.T) {
	bin := buildBinary(t)
	a := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "roots")
	os.WriteFile(cfg, []byte("a "+a+"\n"), 0644)

	cmd := exec.Command(bin, "--multi-root", "--no-open", cfg)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	buf := &syncBuffer{}
	cmd.Stderr = buf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if i := strings.Index(buf.String(), "Server running at "); i >= 0 {
			if line, _, ok := strings.Cut(buf.String()[i:], "\n"); ok {
				url := strings.TrimPrefix(line, "Server running at ")
				if strings.HasPrefix(url, "http://localhost:") && strings.HasSuffix(url, "/") {
					return
				}
				t.Fatalf("unexpected URL line %q", line)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no startup line:\n%s", buf.String())
}
