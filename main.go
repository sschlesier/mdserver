package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"mdserver/server"
)

func main() {
	var (
		host       = flag.String("host", "localhost", "Host to bind to")
		port       = flag.Int("port", 0, "Port to bind to (0 for auto-selection)")
		file       = flag.String("file", "", "Specific markdown file to serve (optional)")
		dir        = flag.String("dir", ".", "Directory to serve")
		livereload = flag.Bool("livereload", true, "Enable live reload (default: true)")
	)
	flag.Parse()

	// Remove timestamp prefix from log messages
	log.SetFlags(0)

	// Resolve absolute path for directory
	rootDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("Failed to resolve directory path: %v", err)
	}

	// Validate directory exists
	if info, err := os.Stat(rootDir); err != nil || !info.IsDir() {
		log.Fatalf("Directory does not exist: %s", rootDir)
	}

	// Find available port if needed
	actualPort := *port
	if actualPort == 0 {
		actualPort = findAvailablePort(*host)
		if actualPort == 0 {
			log.Fatal("Failed to find an available port")
		}
	}

	// Create server configuration
	config := server.Config{
		Host:             *host,
		Port:             actualPort,
		RootDir:          rootDir,
		File:             *file,
		EnableLiveReload: *livereload,
	}

	// Initialize and start server
	srv := server.NewServer(config)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down server...")
		srv.Stop()
		os.Exit(0)
	}()

	// Print startup message
	url := fmt.Sprintf("http://%s:%d", *host, actualPort)
	log.Printf("Serving %s", rootDir)
	if *file != "" {
		log.Printf("Entry file: %s", *file)
	}
	log.Printf("Server running at %s", url)
	log.Println("Press Ctrl+C to stop")

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// findAvailablePort scans for an available port starting from 8080
func findAvailablePort(host string) int {
	for port := 8080; port < 65535; port++ {
		addr := fmt.Sprintf("%s:%d", host, port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 0
}
