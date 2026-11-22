package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"orbit/internal/api"
	"orbit/internal/database"
	"orbit/internal/storage"
)

const (
	port    = ":2525"
	walPath = "data/node-a/wal.log"
)

func main() {
	fmt.Println("ORBIT Database Starting...")

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
		// Force close policy: terminate all connections immediately when graceful shutdown fails
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
