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
	json.NewDecoder(rec.Body).Decode(&res)
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

	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 413 or 400 for oversized payload, got %d", rec.Code)
	}
}
