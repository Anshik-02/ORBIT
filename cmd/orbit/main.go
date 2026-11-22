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
	// This opens the WAL, replays any existing records, and reconstructs
	// the Memtable. The database is in a consistent state before we accept
	// any HTTP requests.
	engine, err := storage.NewEngine(walPath)
	if err != nil {
		log.Fatalf("failed to start storage engine: %v", err)
	}

	// --- Wrap engine in the database layer ---
	// The database layer adds validation and clean error types.
	// The API layer talks to the database, never to the engine directly.
	db := database.NewDatabase(engine)

	defer func() {
		fmt.Println("\nShutting down ORBIT...")
		if err := db.Close(); err != nil {
			log.Printf("warning: db close error: %v", err)
		}
		fmt.Println("WAL flushed. Goodbye.")
	}()

	// --- Wire up HTTP routes ---
	mux := http.NewServeMux()
	h := api.NewHandler(db)
	api.RegisterRoutes(mux, h)

	// Health check endpoint — useful for load balancers and Section 2.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok","node":"node-a"}`)
	})

	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// --- Graceful shutdown on Ctrl+C / SIGTERM ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	fmt.Printf("Storage engine ready. WAL: %s\n", walPath)
	fmt.Printf("HTTP API listening on http://localhost%s\n", port)
	fmt.Println()
	fmt.Println("  POST   http://localhost:2525/api/v1/kv")
	fmt.Println("  GET    http://localhost:2525/api/v1/kv/{key}")
	fmt.Println("  DELETE http://localhost:2525/api/v1/kv/{key}")
	fmt.Println("  GET    http://localhost:2525/health")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
