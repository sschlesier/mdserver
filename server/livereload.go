package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin
		return true
	},
}

// skipDirs are directories that should never be watched (heavy or irrelevant)
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
}

// LiveReload manages file watching and WebSocket connections for live reload
type LiveReload struct {
	rootDir   string
	watcher   *fsnotify.Watcher
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	watched   map[string]bool
	watchedMu sync.Mutex
	broadcast chan []byte
	stopChan  chan struct{}
}

// NewLiveReload creates a new LiveReload instance
func NewLiveReload(rootDir string) (*LiveReload, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	lr := &LiveReload{
		rootDir:   rootDir,
		watcher:   watcher,
		clients:   make(map[*websocket.Conn]bool),
		watched:   make(map[string]bool),
		broadcast: make(chan []byte, 256),
		stopChan:  make(chan struct{}),
	}

	return lr, nil
}

// Start begins watching for file changes
func (lr *LiveReload) Start() error {
	// Watch the root directory and immediate children only;
	// deeper directories are watched on-demand via EnsureWatching.
	err := lr.watchDirectory(lr.rootDir, 1)
	if err != nil {
		return err
	}

	// Start goroutines for handling events
	go lr.watchFiles()
	go lr.broadcastMessages()

	return nil
}

// watchDirectory watches a directory and its subdirectories up to the given depth.
// A depth of 0 means watch only the given directory itself (no children).
// Returns an error only if the directory itself cannot be watched; errors on
// child directories are logged and silently absorbed so that fd exhaustion
// does not crash the process.
func (lr *LiveReload) watchDirectory(dir string, depth int) error {
	lr.watchedMu.Lock()
	if lr.watched[dir] {
		lr.watchedMu.Unlock()
		return nil
	}
	lr.watched[dir] = true
	lr.watchedMu.Unlock()

	// Add the directory to the watcher
	if err := lr.watcher.Add(dir); err != nil {
		return err
	}

	if depth <= 0 {
		return nil
	}

	// Watch subdirectories up to remaining depth
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return nil // non-fatal: we already watch the parent
	}

	for _, entry := range entries {
		stat, err := os.Stat(entry)
		if err != nil {
			continue
		}

		if stat.IsDir() {
			base := filepath.Base(entry)
			// Skip hidden directories
			if base[0] == '.' {
				continue
			}
			// Skip known-heavy directories
			if skipDirs[base] {
				continue
			}
			if err := lr.watchDirectory(entry, depth-1); err != nil {
				log.Printf("LiveReload: cannot watch %s: %v", base, err)
			}
		}
	}

	return nil
}

// EnsureWatching ensures the given directory (and a shallow subtree) is being watched.
// Called on-demand when the user navigates to a directory or views a file.
func (lr *LiveReload) EnsureWatching(dir string) {
	lr.watchedMu.Lock()
	already := lr.watched[dir]
	lr.watchedMu.Unlock()
	if already {
		return
	}

	if err := lr.watchDirectory(dir, 1); err != nil {
		log.Printf("LiveReload: Error expanding watch to %s: %v", dir, err)
	}
}

// watchFiles monitors file system events and triggers reloads
func (lr *LiveReload) watchFiles() {
	for {
		select {
		case event, ok := <-lr.watcher.Events:
			if !ok {
				return
			}
			// Only reload on write events for .md files
			if event.Op&fsnotify.Write == fsnotify.Write {
				if filepath.Ext(event.Name) == ".md" {
					// log.Printf("LiveReload: File changed: %s", event.Name)
					lr.broadcast <- []byte("reload")
				}
			}
			// Handle new directories being created
			if event.Op&fsnotify.Create == fsnotify.Create {
				// Check if it's a directory
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					base := filepath.Base(event.Name)
					// Skip hidden and heavy directories
					if base[0] != '.' && !skipDirs[base] {
						lr.watchDirectory(event.Name, 1)
					}
				}
			}
		case err, ok := <-lr.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("LiveReload: Watcher error: %v", err)
		case <-lr.stopChan:
			return
		}
	}
}

// broadcastMessages sends messages to all connected clients
func (lr *LiveReload) broadcastMessages() {
	for {
		select {
		case message := <-lr.broadcast:
			lr.clientsMu.RLock()
			for client := range lr.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("LiveReload: Error writing to client: %v", err)
					lr.clientsMu.RUnlock()
					lr.clientsMu.Lock()
					delete(lr.clients, client)
					client.Close()
					lr.clientsMu.Unlock()
					lr.clientsMu.RLock()
				}
			}
			lr.clientsMu.RUnlock()
		case <-lr.stopChan:
			return
		}
	}
}

// HandleWebSocket handles WebSocket connections for live reload
func (lr *LiveReload) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// log.Printf("LiveReload: WebSocket upgrade error: %v", err)
		return
	}

	// Add client to the map
	lr.clientsMu.Lock()
	lr.clients[conn] = true
	lr.clientsMu.Unlock()

	// log.Printf("LiveReload: Client connected (total: %d)", len(lr.clients))

	// Handle client disconnection
	go func() {
		defer func() {
			lr.clientsMu.Lock()
			delete(lr.clients, conn)
			lr.clientsMu.Unlock()
			conn.Close()
			// log.Printf("LiveReload: Client disconnected (total: %d)", len(lr.clients))
		}()

		// Read loop to detect disconnection
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

// WatchedDirs returns a sorted list of all currently watched directories.
func (lr *LiveReload) WatchedDirs() []string {
	lr.watchedMu.Lock()
	defer lr.watchedMu.Unlock()

	dirs := make([]string, 0, len(lr.watched))
	for dir := range lr.watched {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

// RemoveWatch stops watching the given directory.
func (lr *LiveReload) RemoveWatch(dir string) error {
	lr.watchedMu.Lock()
	defer lr.watchedMu.Unlock()

	if !lr.watched[dir] {
		return fmt.Errorf("directory not watched: %s", dir)
	}

	if err := lr.watcher.Remove(dir); err != nil {
		return err
	}
	delete(lr.watched, dir)
	return nil
}

// Stop stops the file watcher and closes all connections
func (lr *LiveReload) Stop() {
	close(lr.stopChan)
	lr.watcher.Close()

	lr.clientsMu.Lock()
	for client := range lr.clients {
		client.Close()
	}
	lr.clients = make(map[*websocket.Conn]bool)
	lr.clientsMu.Unlock()

	log.Println("LiveReload: Stopped")
}
