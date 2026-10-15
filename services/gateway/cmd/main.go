// Command gateway exposes the public REST API and proxies it to the internal gRPC services.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/pkg/health"
	"github.com/Andas-LV/cargoflow/pkg/logger"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
	"github.com/Andas-LV/cargoflow/pkg/redisx"
	"github.com/Andas-LV/cargoflow/pkg/shutdown"
	"github.com/Andas-LV/cargoflow/services/gateway/internal/config"
	"github.com/Andas-LV/cargoflow/services/gateway/internal/server"
)

const serviceName = "gateway"

func main() {
	if err := run(); err != nil {
		slog.Error(serviceName+" stopped with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(serviceName, cfg.LogLevel)

	ctx, stop := shutdown.SignalContext()
	defer stop()

	otelShutdown, err := otelx.Init(ctx, serviceName, cfg.OTel)
	if err != nil {
		return err
	}
	defer func() { _ = otelShutdown(context.Background()) }()

	rdb, err := redisx.New(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()

	// Connections are lazy: the gateway starts even if a backend is not up yet.
	var backends server.Backends
	if backends.Auth, err = grpcx.NewClient(cfg.AuthGRPCAddr); err != nil {
		return err
	}
	defer func() { _ = backends.Auth.Close() }()
	if backends.Shipment, err = grpcx.NewClient(cfg.ShipmentGRPCAddr); err != nil {
		return err
	}
	defer func() { _ = backends.Shipment.Close() }()
	if backends.Tracking, err = grpcx.NewClient(cfg.TrackingGRPCAddr); err != nil {
		return err
	}
	defer func() { _ = backends.Tracking.Close() }()
	if backends.Notification, err = grpcx.NewClient(cfg.NotificationGRPCAddr); err != nil {
		return err
	}
	defer func() { _ = backends.Notification.Close() }()

	srv, err := server.New(ctx, cfg, backends, rdb, log)
	if err != nil {
		return err
	}

	g := shutdown.New(ctx, 10*time.Second)
	g.Go(func(ctx context.Context) error {
		log.Info("http server listening", "addr", cfg.HTTPAddr, "cors_origins", cfg.CORSOrigins)
		return health.ListenAndServe(ctx, srv)
	}, srv.Shutdown)

	err = g.Wait()
	log.Info("service stopped")
	return err
}
