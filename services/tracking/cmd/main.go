// Command tracking runs the tracking gRPC service and its shipment event consumer.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	shipmentv1 "github.com/Andas-LV/cargoflow/gen/go/shipment/v1"
	trackingv1 "github.com/Andas-LV/cargoflow/gen/go/tracking/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/pkg/health"
	"github.com/Andas-LV/cargoflow/pkg/logger"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
	"github.com/Andas-LV/cargoflow/pkg/postgres"
	"github.com/Andas-LV/cargoflow/pkg/shutdown"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/config"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/consumer"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/grpcserver"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/repository"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/service"
	"github.com/Andas-LV/cargoflow/services/tracking/migrations"
)

const serviceName = "tracking"

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
	positions := repository.NewPositions(pool)

	nc, err := natsx.Connect(cfg.NATS, serviceName, log)
	if err != nil {
		return err
	}
	defer nc.Close()
	if err := nc.EnsureStream(ctx); err != nil {
		return err
	}

	shipmentConn, err := grpcx.NewClient(cfg.ShipmentGRPCAddr)
	if err != nil {
		return err
	}
	defer func() { _ = shipmentConn.Close() }()
	shipments := service.NewGRPCShipments(
		shipmentv1.NewShipmentServiceClient(shipmentConn),
		shipmentv1.NewStationServiceClient(shipmentConn),
	)

	svc := service.New(positions, shipments, shipments, nc, log)

	cons, err := consumer.Start(ctx, nc, svc, log)
	if err != nil {
		return err
	}

	grpcSrv, grpcHealth := grpcx.NewServer(log)
	trackingv1.RegisterTrackingServiceServer(grpcSrv, grpcserver.New(svc, log))

	httpSrv := health.Server(cfg.HTTPAddr, health.Checks{
		"postgres": pool.Ping,
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
		log.Info("consumer started", "subjects", natsx.SubjectShipmentCreated)
		<-ctx.Done()
		return nil
	}, cons.Stop)

	err = g.Wait()
	log.Info("service stopped")
	return err
}
