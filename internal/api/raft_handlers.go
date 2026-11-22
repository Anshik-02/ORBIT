package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"orbit/internal/database"
	"orbit/internal/raft"
)

// NotLeaderResponse is returned when a write or consistent read is received by a non-leader node.
type NotLeaderResponse struct {
	Error         string `json:"error"`
	LeaderID      string `json:"leader_id,omitempty"`
	LeaderAddress string `json:"leader_address,omitempty"`
}

// RaftHealthResponse reports consensus and node health status.
type RaftHealthResponse struct {
	Status      string `json:"status"`       // "ok" or "unhealthy"
	Node        string `json:"node"`         // local node id
	Role        string `json:"role"`         // "leader", "follower", "candidate"
	LeaderID    string `json:"leader_id"`    // current known leader
	Term        uint64 `json:"term"`         // current election term
	CommitIndex uint64 `json:"commit_index"` // highest committed log index
	LastApplied uint64 `json:"last_applied"` // highest applied state-machine index
	Storage     string `json:"storage"`      // "healthy" or "unhealthy: ..."
	Writable    bool   `json:"writable"`     // true only if local node is Leader
	Cluster     string `json:"cluster"`      // "raft"
}

// RaftHandler coordinates client requests through the Raft consensus engine.
type RaftHandler struct {
	raft       *raft.Raft
	db         *database.Database
	nodeID     string
	selfAddr   string
	peerAddrs  map[string]string // peerID -> client HTTP address (e.g. "127.0.0.1:2525")
	reqTimeout time.Duration
}

// NewRaftHandler creates an HTTP handler wired to Raft consensus.
func NewRaftHandler(r *raft.Raft, db *database.Database, nodeID, selfAddr string, peerAddrs map[string]string) *RaftHandler {
	return &RaftHandler{
		raft:       r,
		db:         db,
		nodeID:     nodeID,
		selfAddr:   selfAddr,
		peerAddrs:  peerAddrs,
		reqTimeout: 5 * time.Second,
	}
}

// RegisterRaftRoutes attaches Raft-aware HTTP handlers to mux.
func RegisterRaftRoutes(mux *http.ServeMux, h *RaftHandler) {
	mux.HandleFunc("POST /api/v1/kv", h.HandleSet)
	mux.HandleFunc("GET /api/v1/kv/{key}", h.HandleGet)
	mux.HandleFunc("DELETE /api/v1/kv/{key}", h.HandleDelete)
	mux.HandleFunc("GET /health", h.HandleHealth)
}

// redirectToLeader sends a 307 Temporary Redirect pointing to the current leader's endpoint.
func (h *RaftHandler) redirectToLeader(w http.ResponseWriter, path string) {
	leaderID := h.raft.LeaderID()
	if leaderID == "" || leaderID == h.nodeID {
		writeJSON(w, http.StatusServiceUnavailable, NotLeaderResponse{
			Error: "no leader currently elected",
		})
		return
	}

	leaderAddr, ok := h.peerAddrs[leaderID]
	if !ok || leaderAddr == "" {
		writeJSON(w, http.StatusServiceUnavailable, NotLeaderResponse{
			Error:    "leader address unknown",
			LeaderID: leaderID,
		})
		return
	}

	targetURL := fmt.Sprintf("http://%s%s", leaderAddr, path)
	w.Header().Set("Location", targetURL)
	writeJSON(w, http.StatusTemporaryRedirect, NotLeaderResponse{
		Error:         "not_leader",
		LeaderID:      leaderID,
		LeaderAddress: leaderAddr,
	})
}

// HandleSet handles POST /api/v1/kv via Raft proposal.
func (h *RaftHandler) HandleSet(w http.ResponseWriter, r *http.Request) {
	// If not the leader, redirect client to the leader
	if h.raft.Role() != raft.Leader {
		h.redirectToLeader(w, "/api/v1/kv")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req setRequest
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 1MB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "unexpected trailing content after JSON body")
		return
	}

	if strings.TrimSpace(req.Key) == "" {
		writeError(w, http.StatusBadRequest, "key must not be empty")
		return
	}
	if strings.TrimSpace(req.Value) == "" {
		writeError(w, http.StatusBadRequest, "value must not be empty")
		return
	}

	// Propose mutation to Raft log
	logIndex, _, err := h.raft.Propose("SET", req.Key, req.Value)
	if err != nil {
		if errors.Is(err, raft.ErrNotLeader) {
			h.redirectToLeader(w, "/api/v1/kv")
			return
		}
		if errors.Is(err, raft.ErrStopped) {
			writeError(w, http.StatusServiceUnavailable, "node is shutting down")
			return
		}
		log.Printf("[ERROR] raft propose failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to propose command to raft")
		return
	}

	// Wait until entry has reached majority quorum and is applied to state machine
	ctx, cancel := context.WithTimeout(r.Context(), h.reqTimeout)
	defer cancel()

	if err := h.raft.WaitForApply(ctx, logIndex); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusGatewayTimeout, "timeout waiting for entry to be committed")
			return
		}
		writeError(w, http.StatusInternalServerError, "consensus apply error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, kvResponse{Key: req.Key, Value: req.Value})
}

// HandleGet handles GET /api/v1/kv/{key}.
// Linearizable reads are quorum-confirmed through the leader using the ReadIndex protocol (Raft §6.4).
func (h *RaftHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if strings.TrimSpace(key) == "" {
		writeError(w, http.StatusBadRequest, "key must not be empty")
		return
	}

	if h.raft.Role() != raft.Leader {
		h.redirectToLeader(w, fmt.Sprintf("/api/v1/kv/%s", key))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.reqTimeout)
	defer cancel()

	// Execute ReadIndex protocol (confirming leader term, commit in term, and quorum)
	var readIndex uint64
	var err error
	for {
		readIndex, err = h.raft.ReadIndex(ctx)
		if err == nil {
			break
		}
		if errors.Is(err, raft.ErrLeaderNotReady) && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}
		break
	}

	if err != nil {
		if errors.Is(err, raft.ErrNotLeader) {
			h.redirectToLeader(w, fmt.Sprintf("/api/v1/kv/%s", key))
			return
		}
		log.Printf("[WARN] ReadIndex linearizable read failed: %v", err)
		writeError(w, http.StatusServiceUnavailable, "unable to confirm leadership quorum for linearizable read: "+err.Error())
		return
	}

	// Wait until state machine catches up with readIndex
	if readIndex > 0 {
		if err := h.raft.WaitForApply(ctx, readIndex); err != nil {
			writeError(w, http.StatusInternalServerError, "consensus apply error: "+err.Error())
			return
		}
	}

	value, err := h.db.Get(key)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrKeyNotFound):
			writeError(w, http.StatusNotFound, "key not found")
		case errors.Is(err, database.ErrClosed):
			writeError(w, http.StatusServiceUnavailable, "database is closed")
		default:
			log.Printf("[ERROR] storage failure on GET (%q): %v", key, err)
			writeError(w, http.StatusInternalServerError, "internal storage error")
		}
		return
	}

	writeJSON(w, http.StatusOK, kvResponse{Key: key, Value: value})
}

// HandleDelete handles DELETE /api/v1/kv/{key} via Raft proposal.
func (h *RaftHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if strings.TrimSpace(key) == "" {
		writeError(w, http.StatusBadRequest, "key must not be empty")
		return
	}

	if h.raft.Role() != raft.Leader {
		h.redirectToLeader(w, fmt.Sprintf("/api/v1/kv/%s", key))
		return
	}

	logIndex, _, err := h.raft.Propose("DELETE", key, "")
	if err != nil {
		if errors.Is(err, raft.ErrNotLeader) {
			h.redirectToLeader(w, fmt.Sprintf("/api/v1/kv/%s", key))
			return
		}
		if errors.Is(err, raft.ErrStopped) {
			writeError(w, http.StatusServiceUnavailable, "node is shutting down")
			return
		}
		log.Printf("[ERROR] raft propose delete failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to propose delete to raft")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.reqTimeout)
	defer cancel()

	if err := h.raft.WaitForApply(ctx, logIndex); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusGatewayTimeout, "timeout waiting for entry to be committed")
			return
		}
		writeError(w, http.StatusInternalServerError, "consensus apply error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, messageResponse{Message: "deleted"})
}

// HandleHealth handles GET /health, reporting cluster and consensus status.
func (h *RaftHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	role := h.raft.Role()
	isLeader := (role == raft.Leader)

	storageStatus := "healthy"
	if err := h.db.Health(); err != nil {
		storageStatus = "unhealthy: " + err.Error()
		writeJSON(w, http.StatusServiceUnavailable, RaftHealthResponse{
			Status:      "unhealthy",
			Node:        h.nodeID,
			Role:        strings.ToLower(role.String()),
			LeaderID:    h.raft.LeaderID(),
			Term:        h.raft.CurrentTerm(),
			CommitIndex: h.raft.CommitIndex(),
			LastApplied: h.raft.LastApplied(),
			Storage:     storageStatus,
			Writable:    false,
			Cluster:     "raft",
		})
		return
	}

	if storageErr := h.raft.StorageErr(); storageErr != nil {
		storageStatus = "unhealthy: fatal consensus storage error: " + storageErr.Error()
		writeJSON(w, http.StatusServiceUnavailable, RaftHealthResponse{
			Status:      "unhealthy",
			Node:        h.nodeID,
			Role:        strings.ToLower(role.String()),
			LeaderID:    h.raft.LeaderID(),
			Term:        h.raft.CurrentTerm(),
			CommitIndex: h.raft.CommitIndex(),
			LastApplied: h.raft.LastApplied(),
			Storage:     storageStatus,
			Writable:    false,
			Cluster:     "raft",
		})
		return
	}

	writeJSON(w, http.StatusOK, RaftHealthResponse{
		Status:      "ok",
		Node:        h.nodeID,
		Role:        strings.ToLower(role.String()),
		LeaderID:    h.raft.LeaderID(),
		Term:        h.raft.CurrentTerm(),
		CommitIndex: h.raft.CommitIndex(),
		LastApplied: h.raft.LastApplied(),
		Storage:     storageStatus,
		Writable:    isLeader,
		Cluster:     "raft",
	})
}
