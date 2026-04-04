package studentsharelink

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
)

func TestResolvePageRejectsOversizedToken(t *testing.T) {
	handler := NewHTTPHandler(&usecase.ResolveShareLink{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/s/"+strings.Repeat("a", 300), nil)
	req.SetPathValue("shareToken", strings.Repeat("a", 300))
	recorder := httptest.NewRecorder()

	handler.ResolvePage(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestResolveRejectsOversizedToken(t *testing.T) {
	handler := NewHTTPHandler(&usecase.ResolveShareLink{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/share-links/"+strings.Repeat("a", 300), nil)
	req.SetPathValue("shareToken", strings.Repeat("a", 300))
	recorder := httptest.NewRecorder()

	handler.Resolve(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
