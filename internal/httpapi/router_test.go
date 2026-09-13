package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/profile"
)

func testHandler() http.Handler {
	return New(http.NotFoundHandler(), profile.NewSeedStore(), Options{})
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); !strings.Contains(got, `"status":"ok"`) {
		t.Fatalf("body = %q, want health payload", got)
	}
}

func TestMeta(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"marketing_origin":"https://vutame.com"`) || !strings.Contains(body, `"profile_origin":"https://vuta.me"`) {
		t.Fatalf("body = %q, want domain metadata", body)
	}
}

func TestProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/chrisdontmiss", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"usr_01_chrisdontmiss"`) || !strings.Contains(body, `"handle":"chrisdontmiss"`) {
		t.Fatalf("body = %q, want stable ID and demo profile", body)
	}
}

func TestUnknownProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/nope", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAvailability(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/handles/newcreator/availability", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"available":true`) {
		t.Fatalf("status = %d body = %q, want available handle", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/handles/chrisdontmiss/availability", nil)
	rec = httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("status = %d body = %q, want unavailable handle", rec.Code, rec.Body.String())
	}
}

func TestInvalidHandleAvailability(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/handles/api/availability", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
