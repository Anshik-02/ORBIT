package security

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenBucket tracks request allowances for an IP address.
type TokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// RateLimiter implements an in-memory token-bucket rate limiter per IP address.
type RateLimiter struct {
	mu         sync.Mutex
	rate       float64 // tokens per second
	capacity   float64 // maximum burst size
	buckets    map[string]*TokenBucket
	cleanupInt time.Duration
}

// NewRateLimiter creates a token bucket limiter.
func NewRateLimiter(rate, capacity float64) *RateLimiter {
	rl := &RateLimiter{
		rate:       rate,
		capacity:   capacity,
		buckets:    make(map[string]*TokenBucket),
		cleanupInt: 5 * time.Minute,
	}
	return rl
}

// Allow checks if an IP is allowed to proceed and deducts a token.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[ip]
	if !exists {
		rl.buckets[ip] = &TokenBucket{
			tokens:     rl.capacity - 1,
			lastUpdate: now,
		}
		return true
	}

	// Replenish tokens based on elapsed time
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastUpdate = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

// ExtractClientIP extracts the host portion of the request's RemoteAddr.
func ExtractClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// SecurityConfig bundles security parameters for HTTP middleware.
type SecurityConfig struct {
	Authorizer     *Authorizer
	RateLimiter    *RateLimiter
	MaxBodyBytes   int64
	AllowedOrigins []string
	EnforceHSTS    bool
}

// Wrap applies rate limiting, request size limits, CORS/Origin validation, and security headers.
func (sc *SecurityConfig) Wrap(next http.Handler, requiredPerm Permission) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Security Headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if sc.EnforceHSTS {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// 2. Origin Header Validation (if configured)
		if len(sc.AllowedOrigins) > 0 {
			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := false
				for _, ao := range sc.AllowedOrigins {
					if ao == "*" || strings.EqualFold(ao, origin) {
						allowed = true
						break
					}
				}
				if !allowed {
					http.Error(w, `{"error":"forbidden origin"}`, http.StatusForbidden)
					return
				}
			}
		}

		// 3. Rate Limiting
		if sc.RateLimiter != nil {
			clientIP := ExtractClientIP(r)
			if !sc.RateLimiter.Allow(clientIP) {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "rate limit exceeded",
					"code":  "RATE_LIMITED",
				})
				return
			}
		}

		// 4. Request Body Size Limit
		if sc.MaxBodyBytes > 0 && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, sc.MaxBodyBytes)
		}

		// 5. Authentication & Authorization (if Authorizer configured and permission required)
		if sc.Authorizer != nil && requiredPerm != "" {
			tok := ExtractToken(r)
			user, err := sc.Authorizer.Authenticate(tok)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "unauthorized",
					"code":  "UNAUTHORIZED",
				})
				return
			}

			if err := sc.Authorizer.Authorize(user, requiredPerm); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "forbidden: insufficient permissions",
					"code":  "FORBIDDEN",
				})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
