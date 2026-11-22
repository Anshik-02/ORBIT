package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orbit/internal/api"
)

func newTestAPI(t *testing.T) *http.ServeMux {
	t.Helper()
	db := newTestDB(t)
	mux := http.NewServeMux()
	handler := api.NewHandler(db)
	api.RegisterRoutes(mux, handler)
	return mux
}

// TestAPIHappyPath tests standard REST operations.
func TestAPIHappyPath(t *testing.T) {
	mux := newTestAPI(t)

	// POST /api/v1/kv
	setBody := `{"key":"user:1","value":"Anshik"}`
	req := httptest.NewRequest("POST", "/api/v1/kv", bytes.NewBufferString(setBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST expected status 201, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	// GET /api/v1/kv/user:1
	req = httptest.NewRequest("GET", "/api/v1/kv/user:1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if res["value"] != "Anshik" {
		t.Errorf("expected value Anshik, got %q", res["value"])
	}

	// DELETE /api/v1/kv/user:1
	req = httptest.NewRequest("DELETE", "/api/v1/kv/user:1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE expected status 200, got %d", rec.Code)
	}

	// GET again -> 404
	req = httptest.NewRequest("GET", "/api/v1/kv/user:1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET expected status 404 after delete, got %d", rec.Code)
	}
}

// TestAPIRejectsUnknownFields verifies that unexpected JSON fields are rejected.
func TestAPIRejectsUnknownFields(t *testing.T) {
	mux := newTestAPI(t)

	badBody := `{"key":"user:1","value":"Anshik","extra_field":"malicious"}`
	req := httptest.NewRequest("POST", "/api/v1/kv", bytes.NewBufferString(badBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown fields, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown field") {
		t.Errorf("expected unknown field error message, got %s", rec.Body.String())
	}
}

// TestAPIRejectsTrailingContent verifies that extra content after the JSON object is rejected.
func TestAPIRejectsTrailingContent(t *testing.T) {
	mux := newTestAPI(t)

	bodyWithTrailing := `{"key":"user:1","value":"Anshik"}` + `{"extra":"object"}`
	req := httptest.NewRequest("POST", "/api/v1/kv", bytes.NewBufferString(bodyWithTrailing))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for trailing content, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unexpected trailing content") {
		t.Errorf("expected trailing content error, got %s", rec.Body.String())
	}
}

// TestAPIRejectsOversizedBody verifies request payloads larger than 1MB are rejected.
func TestAPIRejectsOversizedBody(t *testing.T) {
	mux := newTestAPI(t)

	// Create payload larger than 1MB
	oversizedValue := strings.Repeat("A", 1024*1024+10)
	payload := `{"key":"big","value":"` + oversizedValue + `"}`

	req := httptest.NewRequest("POST", "/api/v1/kv", bytes.NewBufferString(payload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for oversized payload, got %d", rec.Code)
	}
}

// TestAPIHealthReporting verifies health endpoint returns detailed diagnostic information.
func TestAPIHealthReporting(t *testing.T) {
	db := newTestDB(t)
	mux := http.NewServeMux()
	handler := api.NewHandler(db)
	api.RegisterRoutes(mux, handler)

	// Healthy check
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if res["status"] != "ok" || res["writable"] != true || res["storage"] != "healthy" {
		t.Errorf("unexpected healthy payload: %+v", res)
	}

	// Close database and verify health reports 503 Service Unavailable
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	req = httptest.NewRequest("GET", "/health", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 on closed db, got %d", rec.Code)
	}

	var closedRes map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&closedRes); err != nil {
		t.Fatalf("Decode closed health response: %v", err)
	}
	if closedRes["writable"] != false || closedRes["status"] != "unhealthy" {
		t.Errorf("unexpected closed payload: %+v", closedRes)
	}
}
