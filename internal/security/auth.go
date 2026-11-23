package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
)

// Role defines the authorization privilege tier for an identity.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleWriter Role = "writer"
	RoleReader Role = "reader"
)

// Permission defines a specific operational entitlement.
type Permission string

const (
	PermRead  Permission = "read"
	PermWrite Permission = "write"
	PermAdmin Permission = "admin"
)

var (
	ErrUnauthorized = errors.New("security: invalid or missing authentication credentials")
	ErrForbidden    = errors.New("security: insufficient permissions for requested operation")
)

// User encapsulates an authenticated principal and their granted role.
type User struct {
	Username string `json:"username"`
	Role     Role   `json:"role"`
	Token    string `json:"token"`
}

// Authorizer manages credentials, token validation, and RBAC permission checks.
type Authorizer struct {
	mu          sync.RWMutex
	usersByTok  map[string]User
	clusterSecret string
}

// NewAuthorizer initializes an Authorizer with an optional cluster-wide shared secret for node-to-node auth.
func NewAuthorizer(clusterSecret string) *Authorizer {
	return &Authorizer{
		usersByTok:    make(map[string]User),
		clusterSecret: clusterSecret,
	}
}

// AddUser registers or updates a user identity with their associated API token and role.
func (a *Authorizer) AddUser(username, token string, role Role) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usersByTok[token] = User{
		Username: username,
		Role:     role,
		Token:    token,
	}
}

// Authenticate verifies the token and returns the corresponding User.
func (a *Authorizer) Authenticate(token string) (User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if token == "" {
		return User{}, ErrUnauthorized
	}

	for storedTok, user := range a.usersByTok {
		if subtle.ConstantTimeCompare([]byte(storedTok), []byte(token)) == 1 {
			return user, nil
		}
	}

	return User{}, ErrUnauthorized
}

// Authorize verifies that the user possesses the required operational permission.
func (a *Authorizer) Authorize(user User, required Permission) error {
	switch user.Role {
	case RoleAdmin:
		return nil // Admin has all permissions
	case RoleWriter:
		if required == PermRead || required == PermWrite {
			return nil
		}
	case RoleReader:
		if required == PermRead {
			return nil
		}
	}
	return ErrForbidden
}

// ValidateClusterToken verifies node-to-node inter-cluster communication tokens.
func (a *Authorizer) ValidateClusterToken(token string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.clusterSecret == "" {
		return true // No cluster secret configured, open cluster
	}
	return subtle.ConstantTimeCompare([]byte(a.clusterSecret), []byte(token)) == 1
}

// ExtractToken retrieves authentication token from Authorization: Bearer, X-API-Key, or ?token= query parameter.
func ExtractToken(r *http.Request) string {
	// 1. Authorization header: Bearer <token>
	authHdr := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHdr), "bearer ") {
		return strings.TrimSpace(authHdr[7:])
	}

	// 2. X-API-Key or X-Cluster-Token header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return strings.TrimSpace(apiKey)
	}
	if clusterTok := r.Header.Get("X-Cluster-Token"); clusterTok != "" {
		return strings.TrimSpace(clusterTok)
	}

	// 3. URL query parameter (useful for WebSocket handshakes)
	if qTok := r.URL.Query().Get("token"); qTok != "" {
		return strings.TrimSpace(qTok)
	}

	return ""
}

// HashPassword generates a salted SHA-256 hash for secure credential storage.
func HashPassword(secret string) (hashHex string, saltHex string, err error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}

	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(secret))

	return hex.EncodeToString(h.Sum(nil)), hex.EncodeToString(salt), nil
}

// VerifyPassword verifies a secret against a stored hash and salt in constant time.
func VerifyPassword(secret, hashHex, saltHex string) bool {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false
	}
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}

	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(secret))
	actualHash := h.Sum(nil)

	return subtle.ConstantTimeCompare(expectedHash, actualHash) == 1
}
