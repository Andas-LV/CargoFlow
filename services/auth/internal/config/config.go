// Package config loads the auth service configuration from environment variables.
package config

import (
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/Andas-LV/cargoflow/pkg/otelx"
)

type Config struct {
	AppEnv   string `env:"APP_ENV" envDefault:"local"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// GRPCAddr is the service address (e.g. localhost:50051); the server listens on its port.
	GRPCAddr string `env:"AUTH_GRPC_ADDR" envDefault:":50051"`
	HTTPAddr string `env:"AUTH_HTTP_ADDR" envDefault:":8081"`

	DatabaseURL    string `env:"AUTH_DATABASE_URL,required"`
	MigrateOnStart bool   `env:"MIGRATE_ON_START"`
	RedisURL       string `env:"REDIS_URL,required"`

	JWT  JWT
	OTel otelx.Config
}

type JWT struct {
	AccessSecret  string        `env:"JWT_ACCESS_SECRET,required"`
	RefreshSecret string        `env:"JWT_REFRESH_SECRET,required"`
	AccessTTL     time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	RefreshTTL    time.Duration `env:"JWT_REFRESH_TTL" envDefault:"168h"`
}

// Load reads .env if present (local runs only; in production it is absent) and parses env.
func Load() (Config, error) {
	_ = godotenv.Load()
	return env.ParseAs[Config]()
}
