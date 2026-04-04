package publicverify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
)

func TestVerifyRejectsUnknownFields(t *testing.T) {
	handler := NewHTTPHandler(&usecase.Verify{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/verify", strings.NewReader(`{"diplomaNumber":"123","universityCode":"UNI","extra":"x"}`))
	recorder := httptest.NewRecorder()

	handler.Verify(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestVerifyRejectsEmptyRequiredFields(t *testing.T) {
	handler := NewHTTPHandler(&usecase.Verify{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/verify", strings.NewReader(`{"diplomaNumber":" ","universityCode":""}`))
	recorder := httptest.NewRecorder()

	handler.Verify(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestVerifyByTokenRejectsOversizedToken(t *testing.T) {
	handler := NewHTTPHandler(&usecase.Verify{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/verify/"+strings.Repeat("a", 300), nil)
	req.SetPathValue("verificationToken", strings.Repeat("a", 300))
	recorder := httptest.NewRecorder()

	handler.VerifyByToken(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
