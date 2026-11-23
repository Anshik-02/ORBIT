package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrEmptyNodeID     = errors.New("node: id must not be empty")
	ErrEmptyDataDir    = errors.New("node: data_dir must not be empty")
	ErrInvalidAddress  = errors.New("node: invalid address")
	ErrInvalidRaftAddr = errors.New("node: invalid raft address")
	ErrDuplicatePeerID = errors.New("node: duplicate peer id detected")
	ErrSelfInPeers     = errors.New("node: peer list must not contain self")
	ErrInvalidTimeouts = errors.New("node: election timeout min must be less than max")
)

// PeerConfig holds connectivity info for a remote peer in the ORBIT cluster.
type PeerConfig struct {
	ID          string `json:"id"`
	Address     string `json:"address"`      // Client HTTP API address (e.g. "127.0.0.1:2526")
	RaftAddress string `json:"raft_address"` // Inter-node Raft RPC address (e.g. "127.0.0.1:3526")
}

// NodeConfig specifies configuration for an independent ORBIT node.
type NodeConfig struct {
	ID                 string        `json:"id"`
	Address            string        `json:"address"`      // e.g. "127.0.0.1:2525"
	RaftAddress        string        `json:"raft_address"` // e.g. "127.0.0.1:3525"
	DataDir            string        `json:"data_dir"`     // e.g. "data/node-a"
	Peers              []PeerConfig  `json:"peers"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval"`   // default: 50ms
	ElectionTimeoutMin time.Duration `json:"election_timeout_min"` // default: 150ms
	ElectionTimeoutMax time.Duration `json:"election_timeout_max"` // default: 300ms
}

// UnmarshalJSON parses a NodeConfig, transparently supporting human-readable duration strings
// (such as "50ms", "150ms") or raw nanosecond integers.
func (c *NodeConfig) UnmarshalJSON(data []byte) error {
	type rawConfig struct {
		ID                 string          `json:"id"`
		Address            string          `json:"address"`
		RaftAddress        string          `json:"raft_address"`
		DataDir            string          `json:"data_dir"`
		Peers              []PeerConfig    `json:"peers"`
		HeartbeatInterval  json.RawMessage `json:"heartbeat_interval"`
		ElectionTimeoutMin json.RawMessage `json:"election_timeout_min"`
		ElectionTimeoutMax json.RawMessage `json:"election_timeout_max"`
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.ID = raw.ID
	c.Address = raw.Address
	c.RaftAddress = raw.RaftAddress
	c.DataDir = raw.DataDir
	c.Peers = raw.Peers

	parseDuration := func(rawMsg json.RawMessage) (time.Duration, error) {
		if len(rawMsg) == 0 || string(rawMsg) == "null" {
			return 0, nil
		}
		var s string
		if err := json.Unmarshal(rawMsg, &s); err == nil {
			return time.ParseDuration(s)
		}
		var num int64
		if err := json.Unmarshal(rawMsg, &num); err == nil {
			return time.Duration(num), nil
		}
		return 0, fmt.Errorf("invalid duration: %s", string(rawMsg))
	}

	var err error
	if c.HeartbeatInterval, err = parseDuration(raw.HeartbeatInterval); err != nil {
		return fmt.Errorf("heartbeat_interval: %w", err)
	}
	if c.ElectionTimeoutMin, err = parseDuration(raw.ElectionTimeoutMin); err != nil {
		return fmt.Errorf("election_timeout_min: %w", err)
	}
	if c.ElectionTimeoutMax, err = parseDuration(raw.ElectionTimeoutMax); err != nil {
		return fmt.Errorf("election_timeout_max: %w", err)
	}

	return nil
}

// LoadConfig reads, deserializes, and validates a NodeConfig from a JSON file.
func LoadConfig(path string) (NodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return NodeConfig{}, fmt.Errorf("node: read config file %q: %w", path, err)
	}
	var cfg NodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return NodeConfig{}, fmt.Errorf("node: parse config JSON %q: %w", path, err)
	}

	// Environment variable overrides
	if envID := os.Getenv("ORBIT_NODE_ID"); envID != "" {
		cfg.ID = envID
	}
	if envAddr := os.Getenv("ORBIT_ADDRESS"); envAddr != "" {
		cfg.Address = envAddr
	} else if envHTTP := os.Getenv("ORBIT_HTTP_ADDR"); envHTTP != "" {
		cfg.Address = envHTTP
	}
	if envRaft := os.Getenv("ORBIT_RAFT_ADDRESS"); envRaft != "" {
		cfg.RaftAddress = envRaft
	} else if envRaftAddr := os.Getenv("ORBIT_RAFT_ADDR"); envRaftAddr != "" {
		cfg.RaftAddress = envRaftAddr
	}
	if envDir := os.Getenv("ORBIT_DATA_DIR"); envDir != "" {
		cfg.DataDir = envDir
	}

	if err := cfg.Validate(); err != nil {
		return NodeConfig{}, fmt.Errorf("node: validate config %q: %w", path, err)
	}
	return cfg, nil
}

// Validate ensures the node configuration is self-consistent and well-formed.
func (c *NodeConfig) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return ErrEmptyNodeID
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return ErrEmptyDataDir
	}
	if strings.TrimSpace(c.Address) == "" {
		return fmt.Errorf("%w: client address cannot be empty", ErrInvalidAddress)
	}
	if strings.TrimSpace(c.RaftAddress) == "" {
		return fmt.Errorf("%w: raft address cannot be empty", ErrInvalidRaftAddr)
	}

	// Validate host:port format
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return fmt.Errorf("%w: address %q: %v", ErrInvalidAddress, c.Address, err)
	}
	if _, _, err := net.SplitHostPort(c.RaftAddress); err != nil {
		return fmt.Errorf("%w: raft address %q: %v", ErrInvalidRaftAddr, c.RaftAddress, err)
	}

	if c.ElectionTimeoutMin == 0 {
		c.ElectionTimeoutMin = 150 * time.Millisecond
	}
	if c.ElectionTimeoutMax == 0 {
		c.ElectionTimeoutMax = 300 * time.Millisecond
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 50 * time.Millisecond
	}

	if c.ElectionTimeoutMin >= c.ElectionTimeoutMax {
		return ErrInvalidTimeouts
	}
	if c.HeartbeatInterval >= c.ElectionTimeoutMin {
		return fmt.Errorf("node: heartbeat interval (%v) must be smaller than election timeout min (%v)",
			c.HeartbeatInterval, c.ElectionTimeoutMin)
	}

	seenPeers := make(map[string]bool)
	for _, p := range c.Peers {
		if p.ID == c.ID {
			return fmt.Errorf("%w: %q", ErrSelfInPeers, p.ID)
		}
		if seenPeers[p.ID] {
			return fmt.Errorf("%w: %q", ErrDuplicatePeerID, p.ID)
		}
		seenPeers[p.ID] = true

		if _, _, err := net.SplitHostPort(p.Address); err != nil {
			return fmt.Errorf("%w: peer %s address %q: %v", ErrInvalidAddress, p.ID, p.Address, err)
		}
		if _, _, err := net.SplitHostPort(p.RaftAddress); err != nil {
			return fmt.Errorf("%w: peer %s raft address %q: %v", ErrInvalidRaftAddr, p.ID, p.RaftAddress, err)
		}
	}

	return nil
}

// ClusterConfig3Nodes returns default configurations for a standard 3-node ORBIT cluster.
func ClusterConfig3Nodes(baseDataDir string) []NodeConfig {
	nodes := []struct {
		id       string
		addr     string
		raftAddr string
	}{
		{"node-a", "127.0.0.1:2525", "127.0.0.1:3525"},
		{"node-b", "127.0.0.1:2526", "127.0.0.1:3526"},
		{"node-c", "127.0.0.1:2527", "127.0.0.1:3527"},
	}

	configs := make([]NodeConfig, len(nodes))
	for i, n := range nodes {
		var peers []PeerConfig
		for j, other := range nodes {
			if i != j {
				peers = append(peers, PeerConfig{
					ID:          other.id,
					Address:     other.addr,
					RaftAddress: other.raftAddr,
				})
			}
		}

		configs[i] = NodeConfig{
			ID:                 n.id,
			Address:            n.addr,
			RaftAddress:        n.raftAddr,
			DataDir:            filepath.Join(baseDataDir, n.id),
			Peers:              peers,
			HeartbeatInterval:  50 * time.Millisecond,
			ElectionTimeoutMin: 150 * time.Millisecond,
			ElectionTimeoutMax: 300 * time.Millisecond,
		}
	}

	return configs
}
