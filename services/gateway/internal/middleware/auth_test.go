package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type validatorFunc func(ctx context.Context, token string) (string, error)

func (f validatorFunc) ValidateToken(ctx context.Context, token string) (string, error) {
	return f(ctx, token)
}

func TestAuth(t *testing.T) {
	validator := validatorFunc(func(_ context.Context, token string) (string, error) {
		switch token {
		case "good":
			return "user-1", nil
		case "down":
			return "", status.Error(codes.Unavailable, "down")
		default:
			return "", status.Error(codes.Unauthenticated, "bad")
		}
	})
	public := func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/v1/auth/") }

	var gotUser string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = UserID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	h := Auth(validator, public)(next)

	tests := []struct {
		name, path, header string
		wantStatus         int
		wantUser           string
	}{
		{"public route", "/v1/auth/login", "", http.StatusNoContent, ""},
		{"missing token", "/v1/shipments", "", http.StatusUnauthorized, ""},
		{"invalid token", "/v1/shipments", "Bearer bad", http.StatusUnauthorized, ""},
		{"auth unavailable", "/v1/shipments", "Bearer down", http.StatusServiceUnavailable, ""},
		{"valid token", "/v1/shipments", "Bearer good", http.StatusNoContent, "user-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotUser = ""
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantUser, gotUser)
		})
	}
}
