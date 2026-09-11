package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()

	New(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); !strings.Contains(got, `"status":"ok"`) {
		t.Fatalf("body = %q, want health payload", got)
	}
}

func TestProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/chrisdontmiss", nil)
	rec := httptest.NewRecorder()

	New(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); !strings.Contains(got, `"handle":"chrisdontmiss"`) {
		t.Fatalf("body = %q, want demo profile", got)
	}
}

func TestUnknownProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/nope", nil)
	rec := httptest.NewRecorder()

	New(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
