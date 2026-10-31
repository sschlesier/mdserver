package server

import (
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// configDebounce is how long the config watcher waits after the last event
// before it reads the file; tests shorten it.
var configDebounce = 100 * time.Millisecond

// portNoticeText is shown on / when the config's port line no longer matches
// the one the server started with.
const portNoticeText = "port change needs a restart"

// configWatcher watches the directory holding the config file and calls
// Server.reloadConfig after a burst of events that name the file. The
// directory is watched, not the file, so a save that replaces the file by
// rename is still seen.
type configWatcher struct {
	s       *Server
	path    string // as the user gave it; read and named in messages
	base    string // base name of the symlink-resolved path
	watcher *fsnotify.Watcher
	done    chan struct{}
	exited  chan struct{}
}

// WatchConfig starts reloading the multi-root config at path whenever it
// changes. startPort is the port line the server started with (0 for none);
// later changes to it only raise a notice. Stop ends the watch.
func (s *Server) WatchConfig(path string, startPort int) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(filepath.Dir(resolved)); err != nil {
		w.Close()
		return err
	}
	cw := &configWatcher{
		s:       s,
		path:    path,
		base:    filepath.Base(resolved),
		watcher: w,
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
	}
	s.startPort = startPort
	s.cfgWatch = cw
	go cw.run()
	return nil
}

func (cw *configWatcher) stop() {
	close(cw.done)
	cw.watcher.Close()
	<-cw.exited
}

func (cw *configWatcher) run() {
	defer close(cw.exited)
	var timer *time.Timer
	var fire <-chan time.Time
	for {
		select {
		case <-cw.done:
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-cw.watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(ev.Name) != cw.base {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(configDebounce)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(configDebounce)
			}
			fire = timer.C
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Config watcher error: %v", err)
		case <-fire:
			fire = nil
			cw.s.reloadConfig(cw.path)
		}
	}
}

// configState returns the current reload error and port notice, both empty
// when there is nothing to show.
func (s *Server) configState() (errMsg, notice string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.reloadErr, s.portNotice
}

// setReloadError records a failed reload and logs a message once until it
// changes or clears.
func (s *Server) setReloadError(msg string) {
	s.stateMu.Lock()
	s.reloadErr = msg
	fresh := msg != s.loggedErr
	s.loggedErr = msg
	s.stateMu.Unlock()
	if fresh {
		log.Printf("Config reload failed, keeping the current roots: %s", msg)
	}
}

// setReloadOK clears the reload error and sets the port notice for a valid file.
func (s *Server) setReloadOK(configPort int) {
	notice := ""
	if configPort != s.startPort {
		notice = portNoticeText
	}
	s.stateMu.Lock()
	s.reloadErr = ""
	s.loggedErr = ""
	s.portNotice = notice
	fresh := notice != "" && notice != s.loggedNotice
	s.loggedNotice = notice
	s.stateMu.Unlock()
	if fresh {
		log.Printf("Config: %s", portNoticeText)
	}
}

// reloadConfig reads the config file and applies it. It is all or nothing: a
// file that fails to load or validate changes no root and is recorded as the
// reload error.
func (s *Server) reloadConfig(path string) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	s.reloadCount.Add(1)

	cfg, err := LoadRootsConfig(path)
	if err != nil {
		s.setReloadError(err.Error())
		return
	}
	if !s.applyRoots(cfg.Roots) {
		return
	}
	s.setReloadOK(cfg.Port)
}

// sameRootDir reports whether two root directories are the same once
// symlinks are resolved.
func sameRootDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// applyRoots makes the served roots match want, in that order: roots with the
// same name and directory keep their live reload, others are added, removed or
// replaced. It reports false if the server has stopped. Callers hold reloadMu.
func (s *Server) applyRoots(want []Root) bool {
	// Create the roots that differ from the current set first, outside the
	// root-set lock. Only reloads change names and directories, and they are
	// serialised, so these stay the differing ones.
	cur := s.rootList()
	fresh := map[string]*root{}
	for _, w := range want {
		if old := findRoot(cur, w.Name); old != nil && sameRootDir(old.dir, w.Dir) {
			continue
		}
		fresh[w.Name] = s.newMultiRoot(w.Name, w.Dir)
	}

	s.rootsMu.Lock()
	if s.stopped {
		s.rootsMu.Unlock()
		for _, rt := range fresh {
			stopRoot(rt)
		}
		return false
	}
	// Re-read the set: a request may have revived a root since the snapshot.
	cur = s.roots
	next := make([]*root, 0, len(want))
	kept := map[*root]bool{}
	for _, w := range want {
		if old := findRoot(cur, w.Name); old != nil && sameRootDir(old.dir, w.Dir) {
			next = append(next, old)
			kept[old] = true
			continue
		}
		nr := fresh[w.Name]
		if nr == nil {
			nr = s.newMultiRoot(w.Name, w.Dir)
			fresh[w.Name] = nr
		}
		next = append(next, nr)
	}
	s.roots = next
	s.rootsMu.Unlock()

	for _, rt := range cur {
		if !kept[rt] {
			stopRoot(rt)
			log.Printf("Root %s removed: %s", rt.name, rt.dir)
		}
	}
	for _, rt := range fresh {
		log.Printf("Serving %s: %s", rt.name, rt.dir)
	}
	return true
}

func findRoot(roots []*root, name string) *root {
	for _, rt := range roots {
		if rt.name == name {
			return rt
		}
	}
	return nil
}

func stopRoot(rt *root) {
	if rt.liveReload != nil {
		rt.liveReload.Stop()
	}
}
