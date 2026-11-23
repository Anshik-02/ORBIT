package main

import (
	"context"
	"errors"
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
	"orbit/internal/observability"
	"orbit/internal/router"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

const (
	port    = ":2525"
	walPath = "data/node-a/wal.log"
)

func main() {
	var configFile string
	var clusterConfigFile string
	flag.StringVar(&configFile, "config", "", "path to node configuration JSON (e.g. configs/node-a.json)")
	flag.StringVar(&clusterConfigFile, "cluster", "configs/cluster.json", "path to cluster metadata JSON (e.g. configs/cluster.json)")
	flag.Parse()

	if configFile == "" {
		configFile = os.Getenv("ORBIT_CONFIG_PATH")
	}
	if clusterConfigFile == "configs/cluster.json" && os.Getenv("ORBIT_CLUSTER_CONFIG") != "" {
		clusterConfigFile = os.Getenv("ORBIT_CLUSTER_CONFIG")
	}

	if configFile != "" {
		runClusterNode(configFile, clusterConfigFile)
		return
	}

	runStandalone()
}

// runClusterNode starts an ORBIT node participating in a distributed Raft consensus cluster.
func runClusterNode(configPath, clusterConfigPath string) {
	// 1. Load node config
	cfg, err := node.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load node config: %v", err)
	}

	// 2. Load cluster metadata if available
	var meta *sharding.ClusterMetadata
	if clusterConfigPath != "" {
		if _, statErr := os.Stat(clusterConfigPath); statErr == nil {
			m, loadErr := sharding.LoadClusterMetadata(clusterConfigPath)
			if loadErr != nil {
				log.Fatalf("failed to load cluster metadata from %s: %v", clusterConfigPath, loadErr)
			}
			if valErr := m.Validate(); valErr != nil {
				log.Fatalf("cluster metadata invalid: %v", valErr)
			}
			meta = m
		}
	}

	var n *node.Node
	if meta != nil && len(meta.ShardsForNode(cfg.ID)) > 0 {
		// 3. Initialize NewShardedNode
		peerRaftAddrs := make(map[string]string)
		peerHTTPAddrs := make(map[string]string)
		for _, p := range cfg.Peers {
			peerRaftAddrs[p.ID] = p.RaftAddress
			peerHTTPAddrs[p.ID] = p.Address
		}

		shardedNode, err := node.NewShardedNode(cfg, meta, peerRaftAddrs, nil)
		if err != nil {
			log.Fatalf("failed to initialize sharded node: %v", err)
		}

		// 4. Construct consistent hash ring
		ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
		if err != nil {
			log.Fatalf("failed to create hash ring: %v", err)
		}
		for sid := sharding.ShardID(0); sid < sharding.ShardID(meta.ShardCount); sid++ {
			if err := ring.AddShard(sid); err != nil {
				log.Fatalf("failed to add shard %d to ring: %v", sid, err)
			}
		}

		// 5. Initialize shard router
		shardRouter := router.New(meta, ring)

		// 6. Initialize leader tracker and resolver
		tracker := router.NewLeaderTracker()
		resolver := router.NewLeaderResolver(tracker)

		// 7. Initialize request forwarder
		forwarder := router.NewRequestForwarder(shardRouter, resolver, cfg.ID, peerHTTPAddrs)

		// 8. Initialize shard-aware KV handler
		kvHandler := api.NewKVHandler(shardedNode, meta, forwarder, cfg.ID)

		// 8b. Wire DistributedEventBus for cross-node event propagation
		eventBus := observability.NewDistributedEventBus(kvHandler.EventHub(), cfg.ID, peerHTTPAddrs)
		kvHandler.SetEventBus(eventBus)

		// 9. Register the KV, cluster health and WebSocket routes
		shardedNode.SetKVHandler(kvHandler)
		n = shardedNode
	} else {
		// Fallback for single-node legacy configuration
		legacyNode, err := node.NewNode(cfg, nil)
		if err != nil {
			log.Fatalf("failed to initialize cluster node: %v", err)
		}
		n = legacyNode
	}

	// 10. Start the HTTP server and Raft transport
	if err := n.Start(); err != nil {
		log.Fatalf("failed to start cluster node: %v", err)
	}

	fmt.Printf("Starting ORBIT Cluster Node %q...\n", cfg.ID)
	fmt.Printf("  Client HTTP API: http://%s\n", n.ListenerAddr())
	fmt.Printf("  Raft Transport:  http://%s\n", cfg.RaftAddress)
	fmt.Printf("  Data Directory:  %s\n", cfg.DataDir)
	fmt.Printf("  Peers:           %d remote peers configured\n", len(cfg.Peers))
	if meta != nil && len(meta.ShardsForNode(cfg.ID)) > 0 {
		fmt.Printf("  Hosted Shards:   %d shard replica(s) assigned\n", len(n.Shards()))
		for sid := range n.Shards() {
			fmt.Printf("    - Shard %d (Group %s)\n", sid, sid.GroupID())
		}
	}
	fmt.Printf("Node %s online and listening.\n", cfg.ID)
	fmt.Println("\nEndpoints available:")
	fmt.Printf("  PUT    http://%s/kv/{key}\n", n.ListenerAddr())
	fmt.Printf("  GET    http://%s/kv/{key}\n", n.ListenerAddr())
	fmt.Printf("  DELETE http://%s/kv/{key}\n", n.ListenerAddr())
	fmt.Printf("  GET    http://%s/cluster/status\n", n.ListenerAddr())
	fmt.Printf("  GET    http://%s/cluster/health\n", n.ListenerAddr())
	fmt.Printf("  GET    http://%s/cluster/shards\n", n.ListenerAddr())
	fmt.Printf("  GET    http://%s/cluster/nodes\n", n.ListenerAddr())
	fmt.Printf("  WS     ws://%s/ws/events\n", n.ListenerAddr())
	fmt.Println("\nPress Ctrl+C to stop.")

	// 11. Implement graceful shutdown for every component
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

	// --- Boot the HTTP API handler ---
	handler := api.NewHandler(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux, handler)

	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	fmt.Printf("ORBIT Online and Listening on http://localhost%s\n", port)
	fmt.Println("\nEndpoints available:")
	fmt.Println("  POST   http://localhost:2525/api/v1/kv")
	fmt.Println("  GET    http://localhost:2525/api/v1/kv/{key}")
	fmt.Println("  DELETE http://localhost:2525/api/v1/kv/{key}")
	fmt.Println("  GET    http://localhost:2525/health")
	fmt.Println("\nPress Ctrl+C to stop.")

	// Listen for OS signals to coordinate clean shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Run the HTTP listener in a separate goroutine.
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failure: %v", err)
		}
	}()

	sig := <-quit
	fmt.Printf("\nReceived signal %v. Initiating graceful shutdown...\n", sig)

	// Step 1: Drain pending HTTP requests (grace period: 5 seconds).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server forced shutdown: %v", err)
	} else {
		fmt.Println("HTTP server drained and stopped.")
	}

	// Step 2: Flush remaining database entries and unlock files.
	if err := db.Close(); err != nil {
		log.Printf("Database close error: %v", err)
	} else {
		fmt.Println("Storage engine flushed and process lock released.")
	}

	fmt.Println("ORBIT shutdown complete. Goodbye.")
}
