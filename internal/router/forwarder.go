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

	// HeaderForwardPath is the HTTP header identifying the complete sequence of nodes that forwarded the request.
	HeaderForwardPath = "X-Orbit-Forward-Path"

	// HeaderRequestID is the HTTP header carrying end-to-end tracing and idempotency request IDs.
	HeaderRequestID = "X-Request-ID"
)

// RouteResult describes how a request for a specific key should be handled.
type RouteResult struct {
	ShardID           sharding.ShardID
	GroupID           raft.GroupID
	IsLocal           bool
	Replica           ShardInstance
	LeaderID          string   // empty if unknown
	CandidateReplica  string   // replica node ID to query/forward to when leader is unknown
	CandidateReplicas []string // all known replicas configured for this shard
	Forward           bool
}

// TargetNode returns the leader node ID if known, or candidate replica to contact.
func (r *RouteResult) TargetNode() string {
	if r.LeaderID != "" {
		return r.LeaderID
	}
	return r.CandidateReplica
}

// Candidates returns the ordered sequence of candidate target node IDs to attempt forwarding to.
// The known leader (if any) is placed first, followed by remaining shard replicas.
// Any node matching excludeNodeID (usually the local node) is omitted.
func (r *RouteResult) Candidates(excludeNodeID string) []string {
	return r.CandidatesWithExcluded(excludeNodeID)
}

// CandidatesWithExcluded returns candidate target node IDs to attempt forwarding to,
// omitting any node in the excluded list (such as the local node and visited forward path nodes).
// The visited path check guarantees acyclic forwarding across shard replicas.
func (r *RouteResult) CandidatesWithExcluded(excluded ...string) []string {
	seen := make(map[string]bool)
	for _, ex := range excluded {
		if ex != "" {
			seen[ex] = true
		}
	}
	var result []string

	if r.LeaderID != "" && !seen[r.LeaderID] {
		result = append(result, r.LeaderID)
		seen[r.LeaderID] = true
	}

	if r.CandidateReplica != "" && !seen[r.CandidateReplica] {
		result = append(result, r.CandidateReplica)
		seen[r.CandidateReplica] = true
	}
	for _, rep := range r.CandidateReplicas {
		if !seen[rep] {
			result = append(result, rep)
			seen[rep] = true
		}
	}
	return result
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

// PeerAddress returns the HTTP address for the target node ID.
func (f *RequestForwarder) PeerAddress(nodeID string) (string, bool) {
	addr, ok := f.peerAddrs[nodeID]
	return addr, ok
}

// PeerAddresses returns a copy of configured peer HTTP addresses.
func (f *RequestForwarder) PeerAddresses() map[string]string {
	res := make(map[string]string, len(f.peerAddrs))
	for k, v := range f.peerAddrs {
		res[k] = v
	}
	return res
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

	shardInfo, _ := f.router.GetShardInfo(key)
	if shardInfo != nil {
		res.CandidateReplicas = append([]string(nil), shardInfo.Replicas...)
	}

	replica, isLocal := localShards[shardID]
	if isLocal && replica != nil {
		res.Replica = replica

		// If local replica is the cluster leader for this shard, handle locally
		if replica.Raft().Role() == raft.Leader {
			res.IsLocal = true
			res.LeaderID = f.localNodeID
			f.resolver.Tracker().SetLeaderWithTerm(groupID, f.localNodeID, replica.Raft().CurrentTerm())
			return res, nil
		}

		// If local replica knows the remote leader, route forward to it
		if leaderID := replica.Raft().LeaderID(); leaderID != "" {
			res.LeaderID = leaderID
			f.resolver.Tracker().SetLeaderWithTerm(groupID, leaderID, replica.Raft().CurrentTerm())
			if leaderID == f.localNodeID {
				res.IsLocal = true
				return res, nil
			}
			res.Forward = true
			return res, nil
		}

		// Local replica hosts this shard but is NOT leader and DOES NOT know who the leader is.
		// Ground truth: leader is currently unknown (election in progress or leader stepped down).
		// Invalidate any stale cached leader in Tracker so followers never ping-pong writes.
		f.resolver.Tracker().Invalidate(groupID)
		res.IsLocal = false
		res.Forward = false
		res.LeaderID = ""
		res.CandidateReplica = ""
		return res, nil
	}

	// Shard is not hosted locally: consult LeaderTracker for cached leader
	if cachedLeader, ok := f.resolver.Tracker().GetLeader(groupID); ok && cachedLeader != "" {
		res.LeaderID = cachedLeader
		res.Forward = true
		return res, nil
	}

	// Shard is not hosted locally and leader is unknown:
	// Set CandidateReplica to first configured replica so external caller can query it
	// to discover the active leader.
	shardInfo, err = f.router.GetShardInfo(key)
	if err == nil && len(shardInfo.Replicas) > 0 {
		res.CandidateReplica = shardInfo.Replicas[0]
		res.Forward = true
		return res, nil
	}

	return nil, fmt.Errorf("%w for shard %s", ErrNoRoute, shardID)
}

// Forward sends an HTTP request to the designated target node with loop detection.
// Optional extraArgs: [0] = reqID, [1] = forwardPath (comma-separated list of visited node IDs).
func (f *RequestForwarder) Forward(ctx context.Context, targetNodeID, method, path string, body []byte, hops int, extraArgs ...string) (*http.Response, error) {
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

	newPath := f.localNodeID
	if len(extraArgs) > 1 && extraArgs[1] != "" {
		newPath = extraArgs[1] + "," + f.localNodeID
	}
	req.Header.Set(HeaderForwardPath, newPath)

	if len(extraArgs) > 0 && extraArgs[0] != "" {
		req.Header.Set(HeaderRequestID, extraArgs[0])
	}
	if len(extraArgs) > 2 && extraArgs[2] != "" {
		req.Header.Set("X-Idempotency-Key", extraArgs[2])
	}
	if len(extraArgs) > 3 && extraArgs[3] != "" {
		req.Header.Set("Authorization", extraArgs[3])
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward request to node %s (%s): %w", targetNodeID, url, err)
	}
	return resp, nil
}
