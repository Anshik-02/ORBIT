package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"orbit/internal/database"
)

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
//
// Request body:
//
//	{ "key": "user:1", "value": "Anshik" }
//
// Responses:
//
//	201 Created  — write succeeded and is durable
//	400 Bad Request — missing/blank key or value
//	500 Internal Server Error — storage failure
func (h *Handler) HandleSet(w http.ResponseWriter, r *http.Request) {
	var req setRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := h.db.Set(req.Key, req.Value); err != nil {
		switch {
		case errors.Is(err, database.ErrEmptyKey):
			writeError(w, http.StatusBadRequest, "key must not be empty")
		case errors.Is(err, database.ErrEmptyValue):
			writeError(w, http.StatusBadRequest, "value must not be empty")
		default:
			writeError(w, http.StatusInternalServerError, "storage error: "+err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, kvResponse{Key: req.Key, Value: req.Value})
}

// HandleGet handles GET /api/v1/kv/{key}
//
// Responses:
//
//	200 OK        — { "key": "...", "value": "..." }
//	400 Bad Request — blank key
//	404 Not Found — key does not exist
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	value, err := h.db.Get(key)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrEmptyKey):
			writeError(w, http.StatusBadRequest, "key must not be empty")
		case errors.Is(err, database.ErrKeyNotFound):
			writeError(w, http.StatusNotFound, "key not found")
		default:
			writeError(w, http.StatusInternalServerError, "storage error: "+err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, kvResponse{Key: key, Value: value})
}

// HandleDelete handles DELETE /api/v1/kv/{key}
//
// Responses:
//
//	200 OK        — key deleted (or was already absent — idempotent)
//	400 Bad Request — blank key
//	500 Internal Server Error — storage failure
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	if err := h.db.Delete(key); err != nil {
		switch {
		case errors.Is(err, database.ErrEmptyKey):
			writeError(w, http.StatusBadRequest, "key must not be empty")
		default:
			writeError(w, http.StatusInternalServerError, "storage error: "+err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, messageResponse{Message: "deleted"})
}
