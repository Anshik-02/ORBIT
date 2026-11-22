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

	// Ensure the data directory exists.
	if err := os.MkdirAll("data/node-a", 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	// --- Boot the storage engine ---
	// Opens WAL, validates sequence continuity, replays existing records,
	// and reconstructs the Memtable before accepting any requests.
	engine, err := storage.NewEngine(walPath)
	if err != nil {
		log.Fatalf("failed to start storage engine: %v", err)
	}

	// --- Wrap engine in the database layer ---
	db := database.NewDatabase(engine)

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
			log.Fatalf("server error: %v", err)
		}
	}()

	// Block main thread until OS shutdown signal arrives
	sig := <-quit
	fmt.Printf("\nReceived signal %v. Initiating graceful shutdown...\n", sig)

	// Phase 1: Shutdown HTTP server first.
	// This stops accepting new connections and gives active requests up to 5s to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("warning: HTTP server shutdown error (some requests may have timed out): %v", err)
	} else {
		fmt.Println("HTTP server stopped accepting connections and drained.")
	}

	// Phase 2: Now that all HTTP handlers have exited, safely flush and close the database.
	// This guarantees that no active HTTP handler attempts to write to a closed WAL.
	fmt.Println("Flushing WAL and closing storage engine...")
	if err := db.Close(); err != nil {
		log.Printf("warning: database close error: %v", err)
	} else {
		fmt.Println("WAL synced and storage engine closed cleanly.")
	}

	fmt.Println("ORBIT shutdown complete. Goodbye.")
}
