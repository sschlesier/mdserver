package server

import (
	"net/http"
	"os"
	"path/filepath"
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
