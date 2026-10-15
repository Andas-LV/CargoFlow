// Command shipment runs the shipment gRPC service and its outbox relay.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	shipmentv1 "github.com/Andas-LV/cargoflow/gen/go/shipment/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/pkg/health"
	"github.com/Andas-LV/cargoflow/pkg/logger"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
	"github.com/Andas-LV/cargoflow/pkg/postgres"
	"github.com/Andas-LV/cargoflow/pkg/redisx"
	"github.com/Andas-LV/cargoflow/pkg/shutdown"
	"github.com/Andas-LV/cargoflow/services/shipment/data"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/config"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/events"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/grpcserver"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/service"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/stations"
	"github.com/Andas-LV/cargoflow/services/shipment/migrations"
)

const serviceName = "shipment"

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

	nc, err := natsx.Connect(cfg.NATS, serviceName, log)
	if err != nil {
		return err
	}
	defer nc.Close()
	if err := nc.EnsureStream(ctx); err != nil {
		return err
	}

	store := repository.NewStore(pool)

	// The station directory ships with the binary: sync it into the DB, then serve it from memory.
	stationRows, err := stations.ParseCSV(data.StationsKZ)
	if err != nil {
		return err
	}
	if err := stations.Sync(ctx, store.Queries, stationRows); err != nil {
		return err
	}
	dir, err := stations.Load(ctx, store.Queries)
	if err != nil {
		return err
	}
	log.Info("station directory loaded", "stations", dir.Len())

	svc := service.New(store, repository.NewCache(rdb, cfg.CacheTTL), dir, log)
	relay := events.NewRelay(store, nc, cfg.OutboxPollInterval, log)

	grpcSrv, grpcHealth := grpcx.NewServer(log)
	shipmentv1.RegisterShipmentServiceServer(grpcSrv, grpcserver.New(svc, log))
	shipmentv1.RegisterStationServiceServer(grpcSrv, grpcserver.NewStationServer(dir))

	httpSrv := health.Server(cfg.HTTPAddr, health.Checks{
		"postgres": pool.Ping,
		"redis":    redisx.Check(rdb),
		"nats":     nc.Check,
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
	g.Go(func(ctx context.Context) error {
		log.Info("outbox relay started", "interval", cfg.OutboxPollInterval)
		return relay.Run(ctx)
	}, nil)

	err = g.Wait()
	log.Info("service stopped")
	return err
}
