package middleware

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TokenValidator validates an access token and returns its user ID.
type TokenValidator interface {
	ValidateToken(ctx context.Context, token string) (string, error)
}

type userIDKey struct{}

// UserID returns the user authenticated by Auth.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok && id != ""
}

// Auth requires "Authorization: Bearer <access token>" on every request except those for
// which public returns true. The user ID is stored in the request context.
func Auth(v TokenValidator, public func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if public(r) {
				next.ServeHTTP(w, r)
				return
			}

			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				writeError(w, http.StatusUnauthorized, codes.Unauthenticated, "missing bearer token")
				return
			}

			userID, err := v.ValidateToken(r.Context(), token)
			switch status.Code(err) {
			case codes.OK:
			case codes.Unauthenticated:
				writeError(w, http.StatusUnauthorized, codes.Unauthenticated, "invalid or expired token")
				return
			default:
				writeError(w, http.StatusServiceUnavailable, codes.Unavailable, "auth service unavailable")
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
		})
	}
}
