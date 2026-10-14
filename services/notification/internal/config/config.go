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

	HTTPAddr string `env:"NOTIFICATION_HTTP_ADDR" envDefault:":8085"`

	Telegram Telegram

	NATS natsx.Config
	OTel otelx.Config
}

type Telegram struct {
	BotToken string `env:"TELEGRAM_BOT_TOKEN"`
	ChatID   string `env:"TELEGRAM_CHAT_ID"`
}

// Load reads .env if present (local runs only; in production it is absent) and parses env.
func Load() (Config, error) {
	_ = godotenv.Load()
	return env.ParseAs[Config]()
}
