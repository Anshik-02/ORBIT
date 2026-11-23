package chaos

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"orbit/internal/raft"
)

var (
	ErrNetworkPartition = errors.New("chaos: network partition active")
	ErrPacketDropped    = errors.New("chaos: packet dropped by chaos injection")
	ErrRPCTimeout       = errors.New("chaos: RPC timed out due to injected latency")
)

// ChaosTransport decorates any underlying raft.Transport with network fault injection capabilities:
// - Arbitrary node partitions (bidirectional or unidirectional)
// - Configurable transmission latency/jitter
// - Random packet drop rates
// - Simulated RPC timeouts
type ChaosTransport struct {
	underlying raft.Transport
	mu         sync.RWMutex

	// Partition tracking: partitioned[fromNode][toNode] == true means traffic is dropped
	partitions map[string]map[string]bool

	// Latency injection
	latency time.Duration

	// Packet loss rate (0.0 to 1.0)
	packetLossRate float64

	// Forced timeout injection
	forceTimeout bool

	// Local node identifier for this transport instance
	localNodeID string
}

// NewChaosTransport wraps an existing raft.Transport with chaos injection controls.
func NewChaosTransport(localNodeID string, underlying raft.Transport) *ChaosTransport {
	return &ChaosTransport{
		underlying:  underlying,
		localNodeID: localNodeID,
		partitions:  make(map[string]map[string]bool),
	}
}

// Partition isolates localNode from targetNode.
func (c *ChaosTransport) Partition(targetNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.partitions[c.localNodeID] == nil {
		c.partitions[c.localNodeID] = make(map[string]bool)
	}
	c.partitions[c.localNodeID][targetNode] = true
}

// PartitionBidirectional isolates nodeA and nodeB from each other.
func (c *ChaosTransport) PartitionBidirectional(targetNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.partitions[c.localNodeID] == nil {
		c.partitions[c.localNodeID] = make(map[string]bool)
	}
	if c.partitions[targetNode] == nil {
		c.partitions[targetNode] = make(map[string]bool)
	}
	c.partitions[c.localNodeID][targetNode] = true
	c.partitions[targetNode][c.localNodeID] = true
}

// HealPartition restores communication with targetNode.
func (c *ChaosTransport) HealPartition(targetNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.partitions[c.localNodeID] != nil {
		delete(c.partitions[c.localNodeID], targetNode)
	}
	if c.partitions[targetNode] != nil {
		delete(c.partitions[targetNode], c.localNodeID)
	}
}

// HealAll restores full network connectivity.
func (c *ChaosTransport) HealAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partitions = make(map[string]map[string]bool)
	c.latency = 0
	c.packetLossRate = 0
	c.forceTimeout = false
}

// SetLatency sets artificial network delay on outbound RPCs.
func (c *ChaosTransport) SetLatency(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latency = d
}

// SetPacketLoss sets the drop probability (0.0 to 1.0).
func (c *ChaosTransport) SetPacketLoss(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.packetLossRate = rate
}

// SetForceTimeout simulates network blackout where RPCs exceed deadlines.
func (c *ChaosTransport) SetForceTimeout(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forceTimeout = enabled
}

func (c *ChaosTransport) applyChaos(ctx context.Context, target string) error {
	c.mu.RLock()
	isPartitioned := (c.partitions[c.localNodeID] != nil && c.partitions[c.localNodeID][target]) ||
		(c.partitions[target] != nil && c.partitions[target][c.localNodeID])
	lat := c.latency
	loss := c.packetLossRate
	timeout := c.forceTimeout
	c.mu.RUnlock()

	if isPartitioned {
		return ErrNetworkPartition
	}

	if timeout {
		select {
		case <-time.After(50 * time.Millisecond):
			return ErrRPCTimeout
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if loss > 0 && rand.Float64() < loss {
		return ErrPacketDropped
	}

	if lat > 0 {
		select {
		case <-time.After(lat):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (c *ChaosTransport) SendRequestVote(ctx context.Context, target string, groupID raft.GroupID, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	if err := c.applyChaos(ctx, target); err != nil {
		return nil, err
	}
	return c.underlying.SendRequestVote(ctx, target, groupID, req)
}

func (c *ChaosTransport) SendAppendEntries(ctx context.Context, target string, groupID raft.GroupID, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	if err := c.applyChaos(ctx, target); err != nil {
		return nil, err
	}
	return c.underlying.SendAppendEntries(ctx, target, groupID, req)
}

func (c *ChaosTransport) SendInstallSnapshot(ctx context.Context, target string, groupID raft.GroupID, req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
	if err := c.applyChaos(ctx, target); err != nil {
		return nil, err
	}
	return c.underlying.SendInstallSnapshot(ctx, target, groupID, req)
}

func (c *ChaosTransport) RegisterHandler(groupID raft.GroupID, handler raft.RPCHandler) error {
	return c.underlying.RegisterHandler(groupID, handler)
}
