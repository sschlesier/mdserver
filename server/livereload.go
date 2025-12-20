package server

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// LiveReload manages file watching and WebSocket connections for live reload
type LiveReload struct {
	rootDir   string
	watcher   *fsnotify.Watcher
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
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
		broadcast: make(chan []byte, 256),
		stopChan:  make(chan struct{}),
	}

	return lr, nil
}

// Start begins watching for file changes
func (lr *LiveReload) Start() error {
	// Watch the root directory recursively
	err := lr.watchDirectory(lr.rootDir)
	if err != nil {
		return err
	}

	// Start goroutines for handling events
	go lr.watchFiles()
	go lr.broadcastMessages()

	return nil
}

// watchDirectory recursively watches a directory and its subdirectories
func (lr *LiveReload) watchDirectory(dir string) error {
	// Add the directory to the watcher
	err := lr.watcher.Add(dir)
	if err != nil {
		return err
	}

	// Recursively watch subdirectories
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return err
	}

	for _, entry := range entries {
		stat, err := os.Stat(entry)
		if err != nil {
			continue
		}

		if stat.IsDir() {
			// Skip hidden directories
			if filepath.Base(entry)[0] == '.' {
				continue
			}
			// Recursively watch subdirectories
			if err := lr.watchDirectory(entry); err != nil {
				// log.Printf("LiveReload: Error watching directory %s: %v", entry, err)
			}
		}
	}

	return nil
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
					// Skip hidden directories
					if filepath.Base(event.Name)[0] != '.' {
						lr.watchDirectory(event.Name)
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
