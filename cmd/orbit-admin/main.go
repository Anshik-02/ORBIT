package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"orbit/internal/storage"
)

func main() {
	cmd := flag.String("cmd", "status", "Admin command: status, health, shards, nodes, backup, restore")
	url := flag.String("url", "http://localhost:2525", "Cluster HTTP API address")
	dataDir := flag.String("data", "", "Data directory for backup/restore")
	backupDir := flag.String("backup", "", "Backup target/source directory")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}

	switch *cmd {
	case "status", "health", "shards", "nodes":
		reqURL := fmt.Sprintf("%s/cluster/%s", *url, *cmd)
		resp, err := client.Get(reqURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to cluster: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var pretty json.RawMessage
		if err := json.Unmarshal(body, &pretty); err == nil {
			out, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(body))
		}

	case "backup":
		if *dataDir == "" || *backupDir == "" {
			fmt.Fprintf(os.Stderr, "backup requires -data and -backup directory flags\n")
			os.Exit(1)
		}
		outPath, err := storage.CreateBackup(*dataDir, *backupDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "backup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully created backup at %s\n", outPath)

	case "restore":
		if *backupDir == "" || *dataDir == "" {
			fmt.Fprintf(os.Stderr, "restore requires -backup and -data directory flags\n")
			os.Exit(1)
		}
		if err := storage.RestoreBackup(*backupDir, *dataDir); err != nil {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully restored backup from %s to %s\n", *backupDir, *dataDir)

	default:
		fmt.Fprintf(os.Stderr, "unknown admin command: %s\n", *cmd)
		os.Exit(1)
	}
}
