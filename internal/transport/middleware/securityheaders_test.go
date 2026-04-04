package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersMiddlewareSetsHeaders(t *testing.T) {
	middleware := NewSecurityHeaders()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	headers := recorder.Result().Header
	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := headers.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := headers.Get("Strict-Transport-Security"); got == "" {
		t.Fatal("Strict-Transport-Security header is empty")
	}
	if got := headers.Get("Content-Security-Policy"); got != "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'" {
		t.Fatalf("Content-Security-Policy = %q", got)
	}
}

func TestSecurityHeadersMiddlewareRelaxesCSPForSwagger(t *testing.T) {
	middleware := NewSecurityHeaders()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	got := recorder.Result().Header.Get("Content-Security-Policy")
	want := "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	if got != want {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, want)
	}
}
