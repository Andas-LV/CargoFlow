// Command auth runs the authentication gRPC service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	authv1 "github.com/Andas-LV/cargoflow/gen/go/auth/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/pkg/health"
	"github.com/Andas-LV/cargoflow/pkg/logger"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
	"github.com/Andas-LV/cargoflow/pkg/postgres"
	"github.com/Andas-LV/cargoflow/pkg/redisx"
	"github.com/Andas-LV/cargoflow/pkg/shutdown"
	"github.com/Andas-LV/cargoflow/services/auth/internal/config"
	"github.com/Andas-LV/cargoflow/services/auth/internal/grpcserver"
	"github.com/Andas-LV/cargoflow/services/auth/internal/repository"
	"github.com/Andas-LV/cargoflow/services/auth/internal/service"
	"github.com/Andas-LV/cargoflow/services/auth/migrations"
)

const serviceName = "auth"

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

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.MigrateOnStart {
		if err := postgres.Migrate(ctx, pool, migrations.FS, log); err != nil {
			return err
		}
	}

	rdb, err := redisx.New(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()

	tokens := service.NewTokens(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	svc := service.New(repository.NewUsers(pool), repository.NewSessions(rdb), tokens)

	grpcSrv, grpcHealth := grpcx.NewServer(log)
	authv1.RegisterAuthServiceServer(grpcSrv, grpcserver.New(svc, log))

	httpSrv := health.Server(cfg.HTTPAddr, health.Checks{
		"postgres": pool.Ping,
		"redis":    redisx.Check(rdb),
	})

	g := shutdown.New(ctx, 10*time.Second)
	g.Go(func(ctx context.Context) error {
		addr := grpcx.ListenAddr(cfg.GRPCAddr)
		log.Info("grpc server listening", "addr", addr)
		return grpcx.Serve(ctx, grpcSrv, addr)
	}, func(ctx context.Context) error {
		grpcHealth.Shutdown()
		return grpcx.Stop(ctx, grpcSrv)
	})
	g.Go(func(ctx context.Context) error {
		log.Info("health server listening", "addr", cfg.HTTPAddr)
		return health.ListenAndServe(ctx, httpSrv)
	}, httpSrv.Shutdown)

	err = g.Wait()
	log.Info("service stopped")
	return err
}
