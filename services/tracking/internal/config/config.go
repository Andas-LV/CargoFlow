// Package config loads the tracking service configuration from environment variables.
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

	// GRPCAddr is the service address (e.g. localhost:50053); the server listens on its port.
	GRPCAddr string `env:"TRACKING_GRPC_ADDR" envDefault:":50053"`
	HTTPAddr string `env:"TRACKING_HTTP_ADDR" envDefault:":8083"`
	// ShipmentGRPCAddr is used to check that a shipment exists and belongs to the caller.
	ShipmentGRPCAddr string `env:"SHIPMENT_GRPC_ADDR,required"`

	DatabaseURL    string `env:"TRACKING_DATABASE_URL,required"`
	MigrateOnStart bool   `env:"MIGRATE_ON_START"`

	NATS natsx.Config
	OTel otelx.Config
}

// Load reads .env if present (local runs only; in production it is absent) and parses env.
func Load() (Config, error) {
	_ = godotenv.Load()
	return env.ParseAs[Config]()
}
