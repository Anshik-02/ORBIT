package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"orbit/internal/raft"
	"orbit/internal/sharding"
)

var (
	// ErrNoRoute indicates that no replica or leader could be identified for the key.
	ErrNoRoute = errors.New("router: no route found for key")

	// ErrPeerAddressNotFound is returned when forwarding is needed but the target node's address is unknown.
	ErrPeerAddressNotFound = errors.New("router: target peer address not found")

	// ErrForwardingLoop indicates that a request exceeded the maximum allowed forwarding hops.
	ErrForwardingLoop = errors.New("router: forwarding loop detected")
)

const (
	// MaxForwardHops defines the maximum number of inter-node forwarding attempts before halting.
	MaxForwardHops = 3

	// HeaderForwardHops is the HTTP header tracking forwarding hop count.
	HeaderForwardHops = "X-Orbit-Forward-Hops"

	// HeaderForwardedFrom is the HTTP header identifying the origin node that forwarded the request.
	HeaderForwardedFrom = "X-Orbit-Forwarded-From"
)

// RouteResult describes how a request for a specific key should be handled.
type RouteResult struct {
	ShardID          sharding.ShardID
	GroupID          raft.GroupID
	IsLocal          bool
	Replica          ShardInstance
	LeaderID         string // empty if unknown
	CandidateReplica string // replica node ID to query/forward to when leader is unknown
	Forward          bool
}

// TargetNode returns the leader node ID if known, or candidate replica to contact.
func (r *RouteResult) TargetNode() string {
	if r.LeaderID != "" {
		return r.LeaderID
	}
	return r.CandidateReplica
}

// HasKnownLeader returns true if a confirmed leader is identified.
func (r *RouteResult) HasKnownLeader() bool {
	return r.LeaderID != ""
}

// RequestForwarder routes operations to local shard replicas or forwards them
// to the leader of the corresponding consensus group.
type RequestForwarder struct {
	router      *ShardRouter
	resolver    *LeaderResolver
	localNodeID string
	peerAddrs   map[string]string // nodeID -> HTTP address
	httpClient  *http.Client
}

// NewRequestForwarder creates a new RequestForwarder instance.
func NewRequestForwarder(
	r *ShardRouter,
	resolver *LeaderResolver,
	localNodeID string,
	peerAddrs map[string]string,
) *RequestForwarder {
	return &RequestForwarder{
		router:      r,
		resolver:    resolver,
		localNodeID: localNodeID,
		peerAddrs:   peerAddrs,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Router returns the underlying ShardRouter.
func (f *RequestForwarder) Router() *ShardRouter {
	return f.router
}

// Resolver returns the underlying LeaderResolver.
func (f *RequestForwarder) Resolver() *LeaderResolver {
	return f.resolver
}

// SetTimeout overrides the HTTP client timeout for forwarded requests.
func (f *RequestForwarder) SetTimeout(d time.Duration) {
	f.httpClient.Timeout = d
}

// Route computes the target shard for a key and determines whether the current node
// can serve the request locally or needs to forward to a remote leader.
func (f *RequestForwarder) Route(key string, localShards map[sharding.ShardID]ShardInstance) (*RouteResult, error) {
	shardID, err := f.router.GetShardID(key)
	if err != nil {
		return nil, fmt.Errorf("route key %q: %w", key, err)
	}

	groupID := raft.GroupID(shardID.GroupID())
	res := &RouteResult{
		ShardID: shardID,
		GroupID: groupID,
	}

	replica, isLocal := localShards[shardID]
	if isLocal && replica != nil {
		res.Replica = replica

		// If local replica is the cluster leader for this shard, handle locally
		if replica.Raft().Role() == raft.Leader {
			res.IsLocal = true
			res.LeaderID = f.localNodeID
			return res, nil
		}

		// If local replica knows the remote leader, route forward to it
		if leaderID := replica.Raft().LeaderID(); leaderID != "" {
			res.LeaderID = leaderID
			f.resolver.Tracker().SetLeader(groupID, leaderID)
			if leaderID == f.localNodeID {
				res.IsLocal = true
				return res, nil
			}
			res.Forward = true
			return res, nil
		}
	}

	// Consult LeaderTracker for cached leader
	if cachedLeader, ok := f.resolver.Tracker().GetLeader(groupID); ok && cachedLeader != "" {
		res.LeaderID = cachedLeader
		if cachedLeader == f.localNodeID && isLocal {
			res.IsLocal = true
		} else {
			res.Forward = true
		}
		return res, nil
	}

	// If hosted locally but leader is currently not established, allow local handler
	// to return ErrNotLeader or forward to a candidate replica in the shard
	if isLocal && replica != nil {
		res.IsLocal = (replica.Raft().Role() == raft.Leader)
		res.Forward = !res.IsLocal
		if res.Forward {
			if shardInfo, err := f.router.GetShardInfo(key); err == nil {
				for _, rep := range shardInfo.Replicas {
					if rep != f.localNodeID {
						res.CandidateReplica = rep
						break
					}
				}
				if res.CandidateReplica == "" && len(shardInfo.Replicas) > 0 {
					res.CandidateReplica = shardInfo.Replicas[0]
				}
			}
		}
		return res, nil
	}

	// Shard is not hosted locally and leader is unknown:
	// Set CandidateReplica to first configured replica so caller can query it
	// without falsely claiming it is the Raft leader.
	shardInfo, err := f.router.GetShardInfo(key)
	if err == nil && len(shardInfo.Replicas) > 0 {
		res.CandidateReplica = shardInfo.Replicas[0]
		res.Forward = true
		return res, nil
	}

	return nil, fmt.Errorf("%w for shard %s", ErrNoRoute, shardID)
}

// Forward sends an HTTP request to the designated target node with loop detection.
func (f *RequestForwarder) Forward(ctx context.Context, targetNodeID, method, path string, body []byte, hops int) (*http.Response, error) {
	if hops >= MaxForwardHops {
		return nil, fmt.Errorf("%w: exceeded %d hops", ErrForwardingLoop, MaxForwardHops)
	}

	addr, ok := f.peerAddrs[targetNodeID]
	if !ok || addr == "" {
		return nil, fmt.Errorf("%w: node %q", ErrPeerAddressNotFound, targetNodeID)
	}

	url := addr
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + addr
	}
	url = fmt.Sprintf("%s%s", url, path)

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create forward request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderForwardedFrom, f.localNodeID)
	req.Header.Set(HeaderForwardHops, strconv.Itoa(hops+1))

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward request to node %s (%s): %w", targetNodeID, url, err)
	}
	return resp, nil
}
