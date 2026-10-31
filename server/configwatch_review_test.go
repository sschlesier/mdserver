package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRevivalStopsTheReplacedRoot(t *testing.T) {
	f := newReloadFixture(t, 0)
	old := f.srv.rootNamed("b")
	if err := os.RemoveAll(f.b); err != nil {
		t.Fatal(err)
	}
	f.get("/b/x.md")
	writeTestFile(t, filepath.Join(f.b, "x.md"), "# B again\n")
	if !f.serves("/b/x.md") {
		t.Fatal("b was not revived")
	}
	select {
	case <-old.liveReload.stopChan:
	default:
		t.Error("the replaced root's watcher was not stopped")
	}
}

func TestPickerRequestRevivesAVanishedRoot(t *testing.T) {
	f := newReloadFixture(t, 0)
	if err := os.RemoveAll(f.b); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.get("/_mdserver/files?root=b"); code != http.StatusGone {
		t.Fatalf("picker on a vanished root = %d, want 410", code)
	}
	writeTestFile(t, filepath.Join(f.b, "x.md"), "# B again\n")
	if code, _ := f.get("/_mdserver/files?root=b"); code != http.StatusOK {
		t.Errorf("picker after the folder returned = %d, want 200", code)
	}
}

func TestReviveRootBranches(t *testing.T) {
	f := newReloadFixture(t, 0)
	oldB := f.srv.rootNamed("b")
	if err := os.RemoveAll(f.b); err != nil {
		t.Fatal(err)
	}
	f.get("/b/")
	writeTestFile(t, filepath.Join(f.b, "x.md"), "# B again\n")

	first := f.srv.reviveRoot(oldB)
	if first == nil || first == oldB {
		t.Fatal("first revival did not produce a fresh root")
	}
	// A second request still holding the old pointer gets the root already revived.
	if second := f.srv.reviveRoot(oldB); second != first {
		t.Error("a lost revival race did not return the winner's root")
	}

	// A name that left the set is not revived.
	f.write(t, f.valid("a"))
	eventually(t, "b is removed", func() bool { return f.srv.rootNamed("b") == nil })
	if f.srv.reviveRoot(first) != nil {
		t.Error("a removed name was revived")
	}

	f.srv.Stop()
	if f.srv.reviveRoot(f.srv.rootNamed("a")) != nil {
		t.Error("a stopped server revived a root")
	}
}

func TestConfigReloadIgnoresOtherFilesInTheDirectory(t *testing.T) {
	f := newReloadFixture(t, 0)
	before := f.srv.reloadCount.Load()
	other := filepath.Join(filepath.Dir(f.cfg), "other")
	writeTestFile(t, other, "x")
	writeTestFile(t, other, "y")
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * configDebounce) // a reload caused by those events would have fired by now
	if n := f.srv.reloadCount.Load() - before; n != 0 {
		t.Fatalf("%d reloads from unrelated files, want 0", n)
	}
	// The watcher is still alive: a config edit reloads.
	f.write(t, f.valid("a", "b", "c"))
	eventually(t, "the config edit is applied", func() bool { return f.serves("/c/x.md") })
}

func TestSameErrorIsLoggedAgainAfterItClears(t *testing.T) {
	lc := captureReloadLog(t)
	f := newReloadFixture(t, 0)
	bad := f.valid("a", "b") + "d ../x\n"
	for round := 1; round <= 2; round++ {
		f.write(t, bad)
		f.srv.reloadConfig(f.cfg)
		f.write(t, f.valid("a", "b"))
		f.srv.reloadConfig(f.cfg)
		if n := lc.count("path must start with / or ~/"); n != round {
			t.Fatalf("after round %d the error was logged %d times, want %d", round, n, round)
		}
	}
}

func TestConfigReloadSurvivesAnUnreadableFile(t *testing.T) {
	f := newReloadFixture(t, 0)
	rtA := f.srv.rootNamed("a")
	if err := os.Chmod(f.cfg, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(f.cfg); err == nil {
		t.Skip("file is readable despite mode 0 (running as root?)")
	}
	f.srv.reloadConfig(f.cfg)
	if _, body := f.get("/"); !strings.Contains(body, "permission denied") {
		t.Errorf("banner lacks the read error: %s", body)
	}
	if f.srv.rootNamed("a") != rtA || !f.serves("/b/x.md") {
		t.Error("an unreadable config changed the served roots")
	}
	if err := os.Chmod(f.cfg, 0644); err != nil {
		t.Fatal(err)
	}
	f.write(t, f.valid("a", "b", "c"))
	eventually(t, "the readable file is loaded", func() bool { return f.serves("/c/x.md") })
}

func TestConcurrentReloadsLeakNoLiveReload(t *testing.T) {
	var mu sync.Mutex
	var made []*LiveReload
	orig := newLiveReload
	newLiveReload = func(dir string, verbose bool) (*LiveReload, error) {
		lr, err := orig(dir, verbose)
		if err == nil {
			mu.Lock()
			made = append(made, lr)
			mu.Unlock()
		}
		return lr, err
	}
	t.Cleanup(func() { newLiveReload = orig })

	f := newReloadFixture(t, 0)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				f.srv.reloadConfig(f.cfg)
			}
		}()
	}
	for j := 0; j < 10; j++ {
		if j%2 == 0 {
			f.write(t, f.valid("a", "c"))
		} else {
			f.write(t, f.valid("a", "b"))
		}
	}
	wg.Wait()
	f.srv.reloadConfig(f.cfg)

	live := map[*LiveReload]bool{}
	for _, rt := range f.srv.rootList() {
		live[rt.liveReload] = true
	}
	mu.Lock()
	defer mu.Unlock()
	for i, lr := range made {
		stopped := false
		select {
		case <-lr.stopChan:
			stopped = true
		default:
		}
		if live[lr] == stopped {
			t.Errorf("live reload %d: serving=%v stopped=%v; a serving one must run and any other must be stopped", i, live[lr], stopped)
		}
	}
}

func TestConcurrentReloadsAreSerialised(t *testing.T) {
	f := newReloadFixture(t, 0)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				f.srv.reloadConfig(f.cfg)
			}
		}(i)
	}
	for j := 0; j < 10; j++ {
		if j%2 == 0 {
			f.write(t, f.valid("a", "c"))
		} else {
			f.write(t, f.valid("a", "b"))
		}
	}
	wg.Wait()
	f.write(t, f.valid("a", "b", "c"))
	f.srv.reloadConfig(f.cfg)
	if got := len(f.srv.rootList()); got != 3 {
		t.Errorf("%d roots after the last reload, want 3", got)
	}
}
