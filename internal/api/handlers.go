package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"orbit/internal/database"
)

const maxRequestBodyBytes = 1 << 20 // 1 MB

// Handler holds a reference to the Database.
// All HTTP handlers are methods on this struct so they share one DB instance.
type Handler struct {
	db *database.Database
}

// NewHandler creates a Handler wired to the given Database.
func NewHandler(db *database.Database) *Handler {
	return &Handler{db: db}
}

// --- Request / Response types ---

type setRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type kvResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type messageResponse struct {
	Message string `json:"message"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// --- Helpers ---

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError sends a JSON error body with the appropriate HTTP status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// --- Handlers ---

// HandleSet handles POST /api/v1/kv
func (h *Handler) HandleSet(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
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

	// Verify no unexpected trailing data exists
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "unexpected trailing content after JSON body")
		return
	}

	if err := h.db.Set(req.Key, req.Value); err != nil {
		switch {
		case errors.Is(err, database.ErrEmptyKey):
			writeError(w, http.StatusBadRequest, "key must not be empty")
		case errors.Is(err, database.ErrEmptyValue):
			writeError(w, http.StatusBadRequest, "value must not be empty")
		case errors.Is(err, database.ErrClosed):
			writeError(w, http.StatusServiceUnavailable, "database is closed")
		default:
			log.Printf("[ERROR] storage failure on SET: %v", err)
			writeError(w, http.StatusInternalServerError, "internal storage error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, kvResponse{Key: req.Key, Value: req.Value})
}

// HandleGet handles GET /api/v1/kv/{key}
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	value, err := h.db.Get(key)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrEmptyKey):
			writeError(w, http.StatusBadRequest, "key must not be empty")
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

// HandleDelete handles DELETE /api/v1/kv/{key}
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	if err := h.db.Delete(key); err != nil {
		switch {
		case errors.Is(err, database.ErrEmptyKey):
			writeError(w, http.StatusBadRequest, "key must not be empty")
		case errors.Is(err, database.ErrClosed):
			writeError(w, http.StatusServiceUnavailable, "database is closed")
		default:
			log.Printf("[ERROR] storage failure on DELETE (%q): %v", key, err)
			writeError(w, http.StatusInternalServerError, "internal storage error")
		}
		return
	}

	writeJSON(w, http.StatusOK, messageResponse{Message: "deleted"})
}
