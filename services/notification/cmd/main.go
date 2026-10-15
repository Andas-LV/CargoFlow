// Command notification consumes CARGO events and sends notifications to users' Telegram chats.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	notificationv1 "github.com/Andas-LV/cargoflow/gen/go/notification/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/pkg/health"
	"github.com/Andas-LV/cargoflow/pkg/logger"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
	"github.com/Andas-LV/cargoflow/pkg/postgres"
	"github.com/Andas-LV/cargoflow/pkg/redisx"
	"github.com/Andas-LV/cargoflow/pkg/shutdown"
	"github.com/Andas-LV/cargoflow/services/notification/internal/bot"
	"github.com/Andas-LV/cargoflow/services/notification/internal/config"
	"github.com/Andas-LV/cargoflow/services/notification/internal/consumer"
	"github.com/Andas-LV/cargoflow/services/notification/internal/grpcserver"
	"github.com/Andas-LV/cargoflow/services/notification/internal/notifier"
	"github.com/Andas-LV/cargoflow/services/notification/internal/repository"
	"github.com/Andas-LV/cargoflow/services/notification/internal/service"
	"github.com/Andas-LV/cargoflow/services/notification/internal/telegram"
	"github.com/Andas-LV/cargoflow/services/notification/migrations"
)

const serviceName = "notification"

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

	// The bot username is needed for t.me deep links; asking Telegram also validates the token.
	tg := telegram.New(cfg.TelegramBotToken)
	var botUsername string
	if tg.Configured() {
		me, err := tg.GetMe(ctx)
		if err != nil {
			return fmt.Errorf("telegram bot token check: %w", err)
		}
		botUsername = me.Username
		log.Info("telegram bot ready", "username", botUsername)
	} else {
		log.Warn("TELEGRAM_BOT_TOKEN is empty: telegram notifications are disabled")
	}

	svc := service.New(repository.NewLinks(pool), repository.NewLinkCodes(rdb), botUsername)

	cons, err := consumer.Start(ctx, nc, notifier.NewTelegram(tg, svc, log), log)
	if err != nil {
		return err
	}

	grpcSrv, grpcHealth := grpcx.NewServer(log)
	notificationv1.RegisterNotificationServiceServer(grpcSrv, grpcserver.New(svc, log))

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
		log.Info("consumer started", "subjects", "shipment.>, tracking.>")
		<-ctx.Done()
		return nil
	}, cons.Stop)
	if tg.Configured() {
		g.Go(func(ctx context.Context) error {
			log.Info("telegram bot polling started")
			return bot.New(tg, svc, log).Run(ctx)
		}, nil)
	}

	err = g.Wait()
	log.Info("service stopped")
	return err
}
