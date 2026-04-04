package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPResolverIgnoresForwardedHeadersFromUntrustedPeer(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("NewClientIPResolver() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.10")

	if got := resolver.Resolve(req); got != "203.0.113.5" {
		t.Fatalf("Resolve() = %s, want 203.0.113.5", got)
	}
}

func TestClientIPResolverUsesForwardedChainFromTrustedProxy(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("NewClientIPResolver() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.1.2.3:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 10.2.3.4")

	if got := resolver.Resolve(req); got != "198.51.100.10" {
		t.Fatalf("Resolve() = %s, want 198.51.100.10", got)
	}
}
