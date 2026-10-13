// Package config loads the shipment service configuration from environment variables.
package config

import (
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/pkg/otelx"
)

type Config struct {
	AppEnv   string `env:"APP_ENV" envDefault:"local"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// GRPCAddr is the service address (e.g. localhost:50052); the server listens on its port.
	GRPCAddr string `env:"SHIPMENT_GRPC_ADDR" envDefault:":50052"`
	HTTPAddr string `env:"SHIPMENT_HTTP_ADDR" envDefault:":8082"`

	DatabaseURL    string        `env:"SHIPMENT_DATABASE_URL,required"`
	MigrateOnStart bool          `env:"MIGRATE_ON_START"`
	RedisURL       string        `env:"REDIS_URL,required"`
	CacheTTL       time.Duration `env:"SHIPMENT_CACHE_TTL" envDefault:"5m"`

	OutboxPollInterval time.Duration `env:"OUTBOX_POLL_INTERVAL" envDefault:"1s"`

	NATS natsx.Config
	OTel otelx.Config
}

// Load reads .env if present (local runs only; in production it is absent) and parses env.
func Load() (Config, error) {
	_ = godotenv.Load()
	return env.ParseAs[Config]()
}
