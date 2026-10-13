// Command notification consumes CARGO events and sends notifications.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Andas-LV/cargoflow/pkg/health"
	"github.com/Andas-LV/cargoflow/pkg/logger"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
	"github.com/Andas-LV/cargoflow/pkg/shutdown"
	"github.com/Andas-LV/cargoflow/services/notification/internal/config"
	"github.com/Andas-LV/cargoflow/services/notification/internal/consumer"
	"github.com/Andas-LV/cargoflow/services/notification/internal/notifier"
)

const serviceName = "notification"

func main() {
	if err := run(); err != nil {
		slog.Error("service stopped with error", "service", serviceName, "err", err)
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

	nc, err := natsx.Connect(cfg.NATS, serviceName, log)
	if err != nil {
		return err
	}
	defer nc.Close()
	if err := nc.EnsureStream(ctx); err != nil {
		return err
	}

	n := notifier.Multi{
		notifier.NewLog(log),
		notifier.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, log),
		notifier.NewEmail(cfg.SMTP.Host, cfg.SMTP.Port, log),
	}
	cons, err := consumer.Start(ctx, nc, n, log)
	if err != nil {
		return err
	}

	httpSrv := health.Server(cfg.HTTPAddr, health.Checks{"nats": nc.Check})

	g := shutdown.New(ctx, 10*time.Second)
	g.Go(func(ctx context.Context) error {
		log.Info("health server listening", "addr", cfg.HTTPAddr)
		return health.ListenAndServe(ctx, httpSrv)
	}, httpSrv.Shutdown)
	g.Go(func(ctx context.Context) error {
		log.Info("consumer started", "subjects", "shipment.>, tracking.>")
		<-ctx.Done()
		return nil
	}, cons.Stop)

	err = g.Wait()
	log.Info("service stopped")
	return err
}
