package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
)

func NewRateLimit(limiter port.RateLimiter, resolver *ClientIPResolver, window time.Duration, metrics *appmetrics.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limiter == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathLabel := normalizePath(r.URL.Path)
			allowed, err := limiter.Allow(r.Context(), buildRateLimitKey(r, resolver, pathLabel))
			if err != nil {
				if metrics != nil {
					metrics.ObserveRateLimit(pathLabel, "error")
				}
				next.ServeHTTP(w, r)
				return
			}
			if allowed {
				if metrics != nil {
					metrics.ObserveRateLimit(pathLabel, "allowed")
				}
				next.ServeHTTP(w, r)
				return
			}

			if metrics != nil {
				metrics.ObserveRateLimit(pathLabel, "blocked")
			}
			retryAfter := int(window.Seconds())
			if retryAfter <= 0 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			http.Error(w, fmt.Sprintf("rate limit exceeded, retry in %d seconds", retryAfter), http.StatusTooManyRequests)
		})
	}
}

func buildRateLimitKey(r *http.Request, resolver *ClientIPResolver, pathLabel string) string {
	clientIP := ClientIP(r)
	if resolver != nil {
		clientIP = resolver.Resolve(r)
	}
	return fmt.Sprintf("%s:%s:%s", clientIP, r.Method, pathLabel)
}

func normalizePath(path string) string {
	switch {
	case path == "/api/v1/public/verify":
		return path
	case strings.HasPrefix(path, "/api/v1/public/verify/"):
		return "/api/v1/public/verify/{verificationToken}"
	case strings.HasPrefix(path, "/v/"):
		return "/v/{verificationToken}"
	case strings.HasPrefix(path, "/s/"):
		return "/s/{shareToken}"
	default:
		return path
	}
}
