package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mdserver/server"
)

// multiRootConflict names the first flag given together with --multi-root that
// it cannot be combined with, or returns "" when there is none.
func multiRootConflict(setFlags map[string]bool, render bool) string {
	switch {
	case setFlags["dir"]:
		return "--dir"
	case setFlags["file"]:
		return "--file"
	case render:
		return "--render"
	}
	return ""
}

// listenPort picks the port: --port if given, else the config's port, else 0
// for an automatically chosen free port.
func listenPort(flagPort, configPort int) int {
	if flagPort != 0 {
		return flagPort
	}
	return configPort
}

// runMultiRoot serves the roots named by the config file until interrupted.
// Config problems are fatal and exit 1 before anything listens.
func runMultiRoot(host string, flagPort int, liveReload, verbose, noOpen bool, configArg string) {
	log.SetFlags(0)

	path, err := server.ConfigPath(configArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	cfg, err := server.LoadRootsConfig(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	port := listenPort(flagPort, cfg.Port)
	if port == 0 {
		port = findAvailablePort(host)
		if port == 0 {
			log.Fatal("Failed to find an available port")
		}
	}

	srv := server.NewServer(server.Config{
		Host:             host,
		Port:             port,
		Roots:            cfg.Roots,
		EnableLiveReload: liveReload,
		Verbose:          verbose,
		Version:          version,
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\nShutting down server...")
		srv.Stop()
		os.Exit(0)
	}()

	url := fmt.Sprintf("http://%s:%d/", host, port)
	for _, r := range cfg.Roots {
		log.Printf("Serving %s: %s", r.Name, r.Dir)
	}
	log.Printf("Server running at %s", url)
	log.Println("Press Ctrl+C to stop")

	if !noOpen {
		go openBrowser(url)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
