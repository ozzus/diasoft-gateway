package registryprivate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListUniversityDiplomasWithNilBodyDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"total":0}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", time.Second)

	result, err := client.ListUniversityDiplomas(context.Background(), "11111111-1111-1111-1111-111111111111", "", "", 1)
	if err != nil {
		t.Fatalf("ListUniversityDiplomas() error = %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("total = %d, want 0", result.Total)
	}
	if len(result.Items) != 0 {
		t.Fatalf("items len = %d, want 0", len(result.Items))
	}
}
