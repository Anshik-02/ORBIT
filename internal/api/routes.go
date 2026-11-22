package api

import "net/http"

// RegisterRoutes attaches all ORBIT API routes to mux.
//
// Route table:
//
//	POST   /api/v1/kv        → HandleSet    (store a key-value pair)
//	GET    /api/v1/kv/{key}  → HandleGet    (retrieve a value by key)
//	DELETE /api/v1/kv/{key}  → HandleDelete (remove a key)
//
// We use Go 1.22+ enhanced ServeMux patterns which support:
//   - Method prefix matching ("GET ", "POST ", "DELETE ")
//   - Path wildcards ("{key}" captured via r.PathValue("key"))
//
// This means we get method routing for free from the stdlib — no external
// router package needed.
func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /api/v1/kv", h.HandleSet)
	mux.HandleFunc("GET /api/v1/kv/{key}", h.HandleGet)
	mux.HandleFunc("DELETE /api/v1/kv/{key}", h.HandleDelete)
}
