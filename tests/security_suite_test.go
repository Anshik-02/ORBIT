package tests

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"orbit/internal/security"
)

func TestSecurity_SelfSignedCertAndTLS(t *testing.T) {
	certPEM, keyPEM, err := security.GenerateSelfSignedCert("127.0.0.1")
	if err != nil {
		t.Fatalf("failed to generate cert: %v", err)
	}

	tempDir := t.TempDir()
	certFile := filepath.Join(tempDir, "server.crt")
	keyFile := filepath.Join(tempDir, "server.key")
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	serverTLS, err := security.LoadServerTLSConfig(certFile, keyFile, "", false)
	if err != nil {
		t.Fatalf("failed to load server TLS config: %v", err)
	}
	if serverTLS.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected min version TLS 1.2, got %x", serverTLS.MinVersion)
	}

	clientTLS, err := security.LoadClientTLSConfig("", "", certFile, false)
	if err != nil {
		t.Fatalf("failed to load client TLS config: %v", err)
	}

	// Test real HTTPS communication
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("secure-orbit"))
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLS,
		},
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("HTTPS GET failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "secure-orbit" {
		t.Fatalf("expected secure-orbit, got %s", string(body))
	}
}

func TestSecurity_RBACAuthenticationAndAuthorization(t *testing.T) {
	auth := security.NewAuthorizer("cluster-secret-123")

	auth.AddUser("alice", "token-admin-1", security.RoleAdmin)
	auth.AddUser("bob", "token-writer-2", security.RoleWriter)
	auth.AddUser("charlie", "token-reader-3", security.RoleReader)

	// Test authentication
	uAlice, err := auth.Authenticate("token-admin-1")
	if err != nil || uAlice.Username != "alice" {
		t.Fatalf("expected alice authenticated, got %v (err: %v)", uAlice, err)
	}

	_, err = auth.Authenticate("invalid-token")
	if err != security.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}

	// Test RBAC permissions
	uBob, _ := auth.Authenticate("token-writer-2")
	uCharlie, _ := auth.Authenticate("token-reader-3")

	// Admin can do all
	if err := auth.Authorize(uAlice, security.PermAdmin); err != nil {
		t.Fatalf("admin should have admin perm: %v", err)
	}
	// Writer can read and write, but not admin
	if err := auth.Authorize(uBob, security.PermWrite); err != nil {
		t.Fatalf("writer should have write perm: %v", err)
	}
	if err := auth.Authorize(uBob, security.PermAdmin); err != security.ErrForbidden {
		t.Fatalf("writer should be forbidden admin perm, got %v", err)
	}
	// Reader can read, but not write
	if err := auth.Authorize(uCharlie, security.PermRead); err != nil {
		t.Fatalf("reader should have read perm: %v", err)
	}
	if err := auth.Authorize(uCharlie, security.PermWrite); err != security.ErrForbidden {
		t.Fatalf("reader should be forbidden write perm, got %v", err)
	}

	// Test cluster secret
	if !auth.ValidateClusterToken("cluster-secret-123") {
		t.Fatalf("expected cluster secret to match")
	}
	if auth.ValidateClusterToken("wrong-secret") {
		t.Fatalf("expected wrong cluster secret to be rejected")
	}
}

func TestSecurity_TokenExtractionAndHashing(t *testing.T) {
	// 1. Bearer header
	r1, _ := http.NewRequest(http.MethodGet, "/kv/test", nil)
	r1.Header.Set("Authorization", "Bearer my-secret-token")
	if tok := security.ExtractToken(r1); tok != "my-secret-token" {
		t.Fatalf("expected my-secret-token, got %s", tok)
	}

	// 2. X-API-Key header
	r2, _ := http.NewRequest(http.MethodGet, "/kv/test", nil)
	r2.Header.Set("X-API-Key", "api-key-999")
	if tok := security.ExtractToken(r2); tok != "api-key-999" {
		t.Fatalf("expected api-key-999, got %s", tok)
	}

	// 3. Query param
	r3, _ := http.NewRequest(http.MethodGet, "/ws/events?token=query-tok-777", nil)
	if tok := security.ExtractToken(r3); tok != "query-tok-777" {
		t.Fatalf("expected query-tok-777, got %s", tok)
	}

	// 4. Salted password hashing
	hash, salt, err := security.HashPassword("super-secret-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if !security.VerifyPassword("super-secret-password", hash, salt) {
		t.Fatalf("password verification should succeed")
	}
	if security.VerifyPassword("wrong-password", hash, salt) {
		t.Fatalf("wrong password verification should fail")
	}
}

func TestSecurity_RateLimiterMiddleware(t *testing.T) {
	// Allow 2 requests burst, rate of 1 token/sec
	limiter := security.NewRateLimiter(1.0, 2.0)

	secConfig := &security.SecurityConfig{
		RateLimiter: limiter,
	}

	handler := secConfig.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "")

	// 1st request -> OK
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/kv/key1", nil)
	req1.RemoteAddr = "192.168.1.100:1234"
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec1.Code)
	}

	// 2nd request -> OK
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/kv/key2", nil)
	req2.RemoteAddr = "192.168.1.100:1234"
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}

	// 3rd request -> Rate limited (429)
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/kv/key3", nil)
	req3.RemoteAddr = "192.168.1.100:1234"
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 Too Many Requests, got %d", rec3.Code)
	}

	// Different IP address should NOT be blocked
	recOther := httptest.NewRecorder()
	reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/kv/key1", nil)
	reqOther.RemoteAddr = "10.0.0.1:5678"
	handler.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusOK {
		t.Fatalf("different IP should succeed, got %d", recOther.Code)
	}
}

func TestSecurity_RequestSizeAndOriginValidation(t *testing.T) {
	secConfig := &security.SecurityConfig{
		MaxBodyBytes:   128, // max 128 bytes
		AllowedOrigins: []string{"https://app.orbit-db.internal"},
	}

	handler := secConfig.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}), "")

	// 1. Origin validation: unauthorized origin -> 403 Forbidden
	recBadOrigin := httptest.NewRecorder()
	reqBadOrigin := httptest.NewRequest(http.MethodPost, "/api/v1/kv/key", bytes.NewReader([]byte("ok")))
	reqBadOrigin.Header.Set("Origin", "https://malicious-site.com")
	handler.ServeHTTP(recBadOrigin, reqBadOrigin)
	if recBadOrigin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized origin, got %d", recBadOrigin.Code)
	}

	// 2. Allowed origin -> OK
	recGoodOrigin := httptest.NewRecorder()
	reqGoodOrigin := httptest.NewRequest(http.MethodPost, "/api/v1/kv/key", bytes.NewReader([]byte("ok")))
	reqGoodOrigin.Header.Set("Origin", "https://app.orbit-db.internal")
	handler.ServeHTTP(recGoodOrigin, reqGoodOrigin)
	if recGoodOrigin.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed origin, got %d", recGoodOrigin.Code)
	}

	// 3. Oversized payload -> 413
	oversized := bytes.Repeat([]byte("A"), 512) // exceeds 128 bytes limit
	recOversized := httptest.NewRecorder()
	reqOversized := httptest.NewRequest(http.MethodPost, "/api/v1/kv/key", bytes.NewReader(oversized))
	handler.ServeHTTP(recOversized, reqOversized)
	if recOversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized body, got %d", recOversized.Code)
	}
}
