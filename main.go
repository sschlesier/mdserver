package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"mdserver/renderer"
	"mdserver/server"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		host        = flag.String("host", "localhost", "Host to bind to")
		port        = flag.Int("port", 0, "Port to bind to (0 for auto-selection)")
		file        = flag.String("file", "", "Specific markdown file to serve (optional)")
		dir         = flag.String("dir", ".", "Directory to serve")
		livereload  = flag.Bool("live-reload", true, "Enable live reload")
		showVersion = flag.Bool("version", false, "Show version information")
		render      = flag.Bool("render", false, "Render markdown to HTML and output to stdout")
		noOpen      = flag.Bool("no-open", false, "Don't open browser on startup")
	)
	flag.BoolVar(render, "r", false, "Render markdown to HTML and output to stdout (shorthand)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdserver [flags] [file]\n\nFlags:\n")
		flag.VisitAll(func(f *flag.Flag) {
			prefix := "--"
			if len(f.Name) == 1 {
				prefix = "-"
			}
			defVal := ""
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
				defVal = fmt.Sprintf(" (default: %s)", f.DefValue)
			}
			fmt.Fprintf(os.Stderr, "  %s%s\t%s%s\n", prefix, f.Name, f.Usage, defVal)
		})
	}
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("mdserver %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", date)
		os.Exit(0)
	}

	// Handle render mode
	if *render {
		// Get input file from --file flag or positional arg
		inputFile := *file
		if inputFile == "" && flag.NArg() > 0 {
			inputFile = flag.Arg(0)
		}
		if inputFile == "" {
			fmt.Fprintln(os.Stderr, "Error: no input file specified")
			os.Exit(1)
		}

		content, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		html, err := renderer.RenderStandalone(content, inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering: %v\n", err)
			os.Exit(1)
		}

		fmt.Print(string(html))
		os.Exit(0)
	}

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

	if !*noOpen {
		go openBrowser(url)
	}

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		log.Printf("Warning: don't know how to open browser on %s", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Warning: failed to open browser: %v", err)
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
