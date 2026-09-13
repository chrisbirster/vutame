package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/profile"
)

func TestProfileQRPNG(t *testing.T) {
	profiles := profile.NewMemoryStore([]profile.Profile{{ID: "usr_qr", Handle: "qrcode"}})
	handler := New(http.NotFoundHandler(), profiles, Options{ProfileOrigin: "https://vuta.me"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/profiles/qrcode/qr.png", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("body is not PNG: %x", rec.Body.Bytes()[:min(8, rec.Body.Len())])
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "vutame-qrcode-qr.png") {
		t.Fatalf("content disposition = %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestProfileQRUnknownHandle(t *testing.T) {
	handler := New(http.NotFoundHandler(), profile.NewMemoryStore(nil), Options{})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/profiles/missing/qr.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}
