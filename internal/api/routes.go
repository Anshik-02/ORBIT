package api

import "net/http"

// RegisterRoutes attaches all ORBIT API routes to mux.
//
// Route table:
//
//	POST   /api/v1/kv        -> HandleSet    (store a key-value pair)
//	GET    /api/v1/kv/{key}  -> HandleGet    (retrieve a value by key)
//	DELETE /api/v1/kv/{key}  -> HandleDelete (remove a key)
//	GET    /health           -> HandleHealth (detailed health & readiness check)
func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /api/v1/kv", h.HandleSet)
	mux.HandleFunc("GET /api/v1/kv/{key}", h.HandleGet)
	mux.HandleFunc("DELETE /api/v1/kv/{key}", h.HandleDelete)
	mux.HandleFunc("GET /health", h.HandleHealth)
}
