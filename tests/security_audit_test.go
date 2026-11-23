package tests

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"orbit/internal/observability"
	"orbit/internal/raft"
	"orbit/internal/security"
)

type mockRPCHandler struct{}

func (m *mockRPCHandler) HandleRequestVote(ctx context.Context, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	return &raft.RequestVoteResponse{Term: req.Term, VoteGranted: true}, nil
}
func (m *mockRPCHandler) HandleAppendEntries(ctx context.Context, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	return &raft.AppendEntriesResponse{Term: req.Term, Success: true}, nil
}
func (m *mockRPCHandler) HandleInstallSnapshot(ctx context.Context, req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
	return &raft.InstallSnapshotResponse{Term: req.Term, Success: true}, nil
}

// TestSecurity_MutualTLSEnforcementOnRaftRPCs tests that HTTPTransport with mTLS
// enforces client certificate verification and rejects unauthenticated RPCs.
func TestSecurity_MutualTLSEnforcementOnRaftRPCs(t *testing.T) {
	// Generate self-signed CA / server cert
	certPEM, keyPEM, err := security.GenerateSelfSignedCert("127.0.0.1")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEM)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}

	serverTr := raft.NewHTTPTransport("127.0.0.1:0")
	serverTr.SetTLSConfig(serverTLS)
	serverTr.SetClusterSecret("super-secret-cluster-key")

	handler := &mockRPCHandler{}
	_ = serverTr.RegisterHandler(raft.GroupID("0"), handler)

	if err := serverTr.Start(); err != nil {
		t.Fatalf("serverTr Start failed: %v", err)
	}
	defer serverTr.Close()

	serverAddr := serverTr.Addr()

	// 1. Client with valid mTLS certificate and matching cluster secret succeeds
	validClientTr := raft.NewHTTPTransport("127.0.0.1:0")
	validClientTr.SetTLSConfig(clientTLS)
	validClientTr.SetClusterSecret("super-secret-cluster-key")
	defer validClientTr.Close()

	req := &raft.RequestVoteRequest{
		Term:        1,
		CandidateID: "client-node",
	}
	resp, err := validClientTr.SendRequestVote(context.Background(), serverAddr, raft.GroupID("0"), req)
	if err != nil {
		t.Fatalf("SendRequestVote failed over valid mTLS: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected valid response over mTLS")
	}

	// 2. Client with bad cluster secret is rejected with error
	badSecretClientTr := raft.NewHTTPTransport("127.0.0.1:0")
	badSecretClientTr.SetTLSConfig(clientTLS)
	badSecretClientTr.SetClusterSecret("wrong-secret")
	defer badSecretClientTr.Close()

	_, err = badSecretClientTr.SendRequestVote(context.Background(), serverAddr, raft.GroupID("0"), req)
	if err == nil {
		t.Fatalf("expected rejection with bad cluster secret, got success")
	}

	// 3. Client without client certificate is rejected by TLS handshake
	noCertClientTr := raft.NewHTTPTransport("127.0.0.1:0")
	noCertClientTr.SetTLSConfig(&tls.Config{
		RootCAs:            caPool,
		InsecureSkipVerify: false,
	})
	defer noCertClientTr.Close()

	_, err = noCertClientTr.SendRequestVote(context.Background(), serverAddr, raft.GroupID("0"), req)
	if err == nil {
		t.Fatalf("expected TLS handshake failure without client certificate, got success")
	}
}

// TestSecurity_WebSocketAuthentication tests that WebSocket connections are gated
// by token authentication and rejected with 401 if unauthorized.
func TestSecurity_WebSocketAuthentication(t *testing.T) {
	authorizer := security.NewAuthorizer("cluster-secret")
	authorizer.AddUser("alice", "token-alice-123", security.RoleReader)

	eventHub := observability.NewEventHub()
	wsHub := observability.NewWebSocketHub(eventHub)

	wsHub.SetAuthValidator(func(r *http.Request) error {
		tok := r.Header.Get("Authorization")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok == "" {
			return errors.New("missing token")
		}
		user, err := authorizer.Authenticate(tok)
		if err != nil {
			return err
		}
		return authorizer.Authorize(user, security.PermRead)
	})

	mux := http.NewServeMux()
	wsHub.RegisterRoutes(mux, "/ws/events")

	// 1. Connection without token -> 401 Unauthorized
	req1 := httptest.NewRequest(http.MethodGet, "/ws/events", nil)
	req1.Header.Set("Connection", "Upgrade")
	req1.Header.Set("Upgrade", "websocket")
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated websocket, got %d", rec1.Code)
	}

	// 2. Connection with invalid token -> 401 Unauthorized
	req2 := httptest.NewRequest(http.MethodGet, "/ws/events?token=invalid-tok", nil)
	req2.Header.Set("Connection", "Upgrade")
	req2.Header.Set("Upgrade", "websocket")
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", rec2.Code)
	}

	// 3. Connection with valid token passes auth check (fails later on Sec-WebSocket-Key in test recorder)
	req3 := httptest.NewRequest(http.MethodGet, "/ws/events?token=token-alice-123", nil)
	req3.Header.Set("Connection", "Upgrade")
	req3.Header.Set("Upgrade", "websocket")
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)

	// Since auth passed, it reaches Sec-WebSocket-Version/Key validation (status 400 Bad Request, NOT 401)
	if rec3.Code == http.StatusUnauthorized {
		t.Fatalf("expected auth to pass with valid token, got 401")
	}
}

// TestSecurity_AntiSpoofingClientIPExtraction tests that spoofed X-Forwarded-For headers
// from untrusted connections are discarded, preventing rate-limit bypass.
func TestSecurity_AntiSpoofingClientIPExtraction(t *testing.T) {
	// Request from untrusted external client with spoofed header
	reqUntrusted := httptest.NewRequest(http.MethodGet, "/kv/test", nil)
	reqUntrusted.RemoteAddr = "203.0.113.195:4321"
	reqUntrusted.Header.Set("X-Forwarded-For", "198.51.100.1")

	ip := security.ExtractClientIP(reqUntrusted, "10.0.0.1") // only 10.0.0.1 is trusted proxy
	if ip != "203.0.113.195" {
		t.Fatalf("expected untrusted connection to use true RemoteAddr 203.0.113.195, got %s", ip)
	}

	// Request from trusted internal load balancer / proxy
	reqTrusted := httptest.NewRequest(http.MethodGet, "/kv/test", nil)
	reqTrusted.RemoteAddr = "10.0.0.1:54321"
	reqTrusted.Header.Set("X-Forwarded-For", "198.51.100.1")

	ipTrusted := security.ExtractClientIP(reqTrusted, "10.0.0.1")
	if ipTrusted != "198.51.100.1" {
		t.Fatalf("expected trusted proxy to extract forwarded client IP 198.51.100.1, got %s", ipTrusted)
	}
}

// TestSecurity_CertificateValidation checks certificate validity and key parameters.
func TestSecurity_CertificateValidation(t *testing.T) {
	certPEM, _, err := security.GenerateSelfSignedCert("node-1.orbit.internal")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("failed to decode certificate PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	if cert.Subject.CommonName != "node-1.orbit.internal" {
		t.Fatalf("unexpected CommonName: %s", cert.Subject.CommonName)
	}
	if len(cert.DNSNames) == 0 {
		t.Fatalf("expected DNS SANs in certificate")
	}
}
