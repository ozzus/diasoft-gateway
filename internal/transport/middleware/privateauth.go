package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/privateapi"
)

type privateClaimsKey struct{}

func NewPrivateAuth(tokenManager *privateapi.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization := strings.TrimSpace(r.Header.Get("Authorization"))
			if !strings.HasPrefix(authorization, "Bearer ") {
				WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
			claims, err := tokenManager.Parse(token)
			if err != nil {
				WritePrivateError(w, http.StatusUnauthorized, "Невалидный токен")
				return
			}

			ctx := context.WithValue(r.Context(), privateClaimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequirePrivateRole(role privateapi.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := PrivateClaimsFromContext(r.Context())
			if !ok {
				WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
				return
			}
			if claims.Role != role {
				WritePrivateError(w, http.StatusForbidden, "Недостаточно прав")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func PrivateClaimsFromContext(ctx context.Context) (privateapi.Claims, bool) {
	claims, ok := ctx.Value(privateClaimsKey{}).(privateapi.Claims)
	return claims, ok
}

func WritePrivateError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
