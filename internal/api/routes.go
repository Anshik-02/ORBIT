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

// WithCORS wraps an http.Handler to provide full CORS support for browser clients and dashboards.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-Idempotency-Key, X-Request-ID, Accept")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
