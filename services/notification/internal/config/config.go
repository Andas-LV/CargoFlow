// Package config loads the notification service configuration from environment variables.
package config

import (
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
)

type Config struct {
	AppEnv   string `env:"APP_ENV" envDefault:"local"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// GRPCAddr is the service address (e.g. localhost:50054); the server listens on its port.
	GRPCAddr string `env:"NOTIFICATION_GRPC_ADDR" envDefault:":50054"`
	HTTPAddr string `env:"NOTIFICATION_HTTP_ADDR" envDefault:":8085"`

	DatabaseURL    string `env:"NOTIFICATION_DATABASE_URL,required"`
	MigrateOnStart bool   `env:"MIGRATE_ON_START"`
	RedisURL       string `env:"REDIS_URL,required"`

	// TelegramBotToken comes from @BotFather. Empty disables Telegram: no notifications, no bot.
	TelegramBotToken string `env:"TELEGRAM_BOT_TOKEN"`

	NATS natsx.Config
	OTel otelx.Config
}

// Load reads .env if present (local runs only; in production it is absent) and parses env.
func Load() (Config, error) {
	_ = godotenv.Load()
	return env.ParseAs[Config]()
}
