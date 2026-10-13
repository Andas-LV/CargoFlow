// Package grpcx builds gRPC servers and clients with the project's standard
// interceptors: OpenTelemetry tracing, panic recovery and structured logging.
package grpcx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// NewServer returns a gRPC server with tracing, recovery and logging interceptors,
// the standard grpc.health.v1 service and server reflection (handy for grpcurl).
// The returned health server starts in SERVING state.
func NewServer(log *slog.Logger, opts ...grpc.ServerOption) (*grpc.Server, *health.Server) {
	recoveryOpt := recovery.WithRecoveryHandlerContext(func(ctx context.Context, p any) error {
		log.ErrorContext(ctx, "grpc panic", "panic", p, "stack", string(debug.Stack()))
		return status.Error(codes.Internal, "internal error")
	})
	logOpt := logging.WithLogOnEvents(logging.FinishCall)

	opts = append([]grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			recovery.UnaryServerInterceptor(recoveryOpt),
			logging.UnaryServerInterceptor(SlogAdapter(log), logOpt),
		),
		grpc.ChainStreamInterceptor(
			recovery.StreamServerInterceptor(recoveryOpt),
			logging.StreamServerInterceptor(SlogAdapter(log), logOpt),
		),
	}, opts...)

	srv := grpc.NewServer(opts...)
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	reflection.Register(srv)
	return srv, hs
}

// Serve listens on addr and serves srv until it is stopped.
func Serve(ctx context.Context, srv *grpc.Server, addr string) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

// Stop tries GracefulStop and falls back to Stop when ctx expires first.
func Stop(ctx context.Context, srv *grpc.Server) error {
	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		srv.Stop()
	}
	return nil
}

// ListenAddr turns a service address such as "localhost:50051" or "auth:50051"
// into a listen address on all interfaces (":50051"). The same env var can then be
// used both by the service itself and by its clients.
func ListenAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return ":" + port
}

// SlogAdapter adapts slog to the go-grpc-middleware logging interface.
func SlogAdapter(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}
