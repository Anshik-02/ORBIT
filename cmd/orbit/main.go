package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"orbit/internal/api"
	"orbit/internal/database"
	"orbit/internal/node"
	"orbit/internal/storage"
)

const (
	port    = ":2525"
	walPath = "data/node-a/wal.log"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "", "path to node configuration JSON (e.g. configs/node-a.json)")
	flag.Parse()

	if configFile != "" {
		runClusterNode(configFile)
		return
	}

	runStandalone()
}

// runClusterNode starts an ORBIT node participating in a distributed Raft consensus cluster.
func runClusterNode(configPath string) {
	cfg, err := node.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load node config: %v", err)
	}

	fmt.Printf("Starting ORBIT Cluster Node %q...\n", cfg.ID)
	fmt.Printf("  Client HTTP API: http://%s\n", cfg.Address)
	fmt.Printf("  Raft Transport:  http://%s\n", cfg.RaftAddress)
	fmt.Printf("  Data Directory:  %s\n", cfg.DataDir)
	fmt.Printf("  Peers:           %d remote peers configured\n", len(cfg.Peers))

	n, err := node.NewNode(cfg, nil) // uses production HTTPTransport
	if err != nil {
		log.Fatalf("failed to initialize cluster node: %v", err)
	}

	if err := n.Start(); err != nil {
		log.Fatalf("failed to start cluster node: %v", err)
	}

	fmt.Printf("Node %s online and listening.\n", cfg.ID)
	fmt.Println("\nEndpoints available:")
	fmt.Printf("  POST   http://%s/api/v1/kv\n", cfg.Address)
	fmt.Printf("  GET    http://%s/api/v1/kv/{key}\n", cfg.Address)
	fmt.Printf("  DELETE http://%s/api/v1/kv/{key}\n", cfg.Address)
	fmt.Printf("  GET    http://%s/health\n", cfg.Address)
	fmt.Println("\nPress Ctrl+C to stop.")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	fmt.Printf("\nReceived signal %v. Initiating graceful cluster node shutdown...\n", sig)
	if err := n.Stop(); err != nil {
		log.Printf("warning: node stop error: %v", err)
	} else {
		fmt.Printf("Node %s drained, consensus stopped, and storage closed cleanly.\n", cfg.ID)
	}
	fmt.Println("ORBIT shutdown complete. Goodbye.")
}

// runStandalone runs ORBIT in standalone single-node mode (Phase 1).
func runStandalone() {
	fmt.Println("ORBIT Database Starting (Standalone Mode)...")

	// Ensure the data directory exists with restrictive 0700 permissions.
	if err := os.MkdirAll("data/node-a", 0700); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	// --- Boot the storage engine ---
	// Acquires exclusive process lock, validates sequences, replays WAL records,
	// and restores consensus progress before accepting requests.
	engine, err := storage.NewEngine(walPath)
	if err != nil {
		log.Fatalf("failed to start storage engine: %v", err)
	}

	// --- Wrap engine in the database layer ---
	db, err := database.NewDatabase(engine)
	if err != nil {
		log.Fatalf("failed to initialize database layer: %v", err)
	}

	// --- Wire up HTTP routes ---
	mux := http.NewServeMux()
	h := api.NewHandler(db)
	api.RegisterRoutes(mux, h)

	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to capture critical server listener startup errors
	serverErrCh := make(chan error, 1)

	// Setup OS signal listener for graceful termination
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in a background goroutine
	go func() {
		fmt.Printf("Storage engine ready. WAL: %s\n", walPath)
		fmt.Printf("HTTP API listening on http://localhost%s\n\n", port)
		fmt.Println("  POST   http://localhost:2525/api/v1/kv")
		fmt.Println("  GET    http://localhost:2525/api/v1/kv/{key}")
		fmt.Println("  DELETE http://localhost:2525/api/v1/kv/{key}")
		fmt.Println("  GET    http://localhost:2525/health")
		fmt.Println("\nPress Ctrl+C to stop.")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	// Wait for OS shutdown signal OR unexpected server failure
	select {
	case sig := <-quit:
		fmt.Printf("\nReceived signal %v. Initiating graceful shutdown...\n", sig)
	case err := <-serverErrCh:
		log.Printf("\nHTTP server failed: %v. Initiating emergency shutdown...\n", err)
	}

	// Phase 1: Graceful drain of HTTP server within 5s
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("warning: HTTP graceful drain failed (%v); forcing immediate socket termination", err)
		if closeErr := server.Close(); closeErr != nil {
			log.Printf("warning: server force-close error: %v", closeErr)
		}
	} else {
		fmt.Println("HTTP server stopped accepting connections and drained.")
	}

	// Phase 2: Now that all HTTP handlers have exited, safely flush and close the database
	fmt.Println("Flushing WAL and closing storage engine...")
	if err := db.Close(); err != nil {
		log.Printf("warning: database close error: %v", err)
	} else {
		fmt.Println("WAL synced and storage engine closed cleanly.")
	}

	fmt.Println("ORBIT shutdown complete. Goodbye.")
}
