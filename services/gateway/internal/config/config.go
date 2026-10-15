// Package config loads the gateway configuration from environment variables.
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

	HTTPAddr string `env:"GATEWAY_HTTP_ADDR" envDefault:":8080"`

	AuthGRPCAddr         string `env:"AUTH_GRPC_ADDR,required"`
	ShipmentGRPCAddr     string `env:"SHIPMENT_GRPC_ADDR,required"`
	TrackingGRPCAddr     string `env:"TRACKING_GRPC_ADDR,required"`
	NotificationGRPCAddr string `env:"NOTIFICATION_GRPC_ADDR,required"`

	CORSOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:5173"`

	RedisURL          string        `env:"REDIS_URL,required"`
	RateLimitRequests int           `env:"RATE_LIMIT_REQUESTS" envDefault:"100"`
	RateLimitWindow   time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`

	OTel otelx.Config
}

// Load reads .env if present (local runs only; in production it is absent) and parses env.
func Load() (Config, error) {
	_ = godotenv.Load()
	return env.ParseAs[Config]()
}
