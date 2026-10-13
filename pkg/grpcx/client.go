package grpcx

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// NewClient creates a lazily-connecting client connection with tracing enabled.
// Traffic is plaintext: services talk over a private network (docker / Render private network).
func NewClient(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}, opts...)
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc client %s: %w", target, err)
	}
	return conn, nil
}

// HealthCheck calls grpc.health.v1 on conn; usable as a readiness check.
func HealthCheck(conn *grpc.ClientConn) func(ctx context.Context) error {
	client := healthpb.NewHealthClient(conn)
	return func(ctx context.Context) error {
		resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			return err
		}
		if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			return fmt.Errorf("status %s", resp.GetStatus())
		}
		return nil
	}
}
