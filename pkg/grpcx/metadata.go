package grpcx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UserIDKey is the metadata key carrying the authenticated user ID.
// Only the gateway sets it (after validating the JWT); it strips the same key from client input.
const UserIDKey = "x-user-id"

// UserID returns the authenticated user ID from incoming metadata.
func UserID(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	vals := md.Get(UserIDKey)
	if len(vals) == 0 || vals[0] == "" {
		return "", false
	}
	return vals[0], true
}

// RequireUserID is UserID that returns codes.Unauthenticated when the ID is missing.
func RequireUserID(ctx context.Context) (string, error) {
	id, ok := UserID(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing user identity")
	}
	return id, nil
}

// WithUserID forwards the user ID to an outgoing service-to-service call.
func WithUserID(ctx context.Context, userID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, UserIDKey, userID)
}
