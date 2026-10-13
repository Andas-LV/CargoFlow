// Package otelx initialises OpenTelemetry tracing with an OTLP/gRPC exporter.
//
// Named otelx (not otel) to avoid clashing with go.opentelemetry.io/otel at call sites.
package otelx

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

// Config is embedded into every service config.
type Config struct {
	// Endpoint is host:port of an OTLP/gRPC collector. Empty disables tracing.
	// A scheme (http:// or https://) is accepted and stripped.
	Endpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	// Headers is a comma-separated list of key=value pairs, e.g. "Authorization=Basic xxx".
	Headers  string `env:"OTEL_EXPORTER_OTLP_HEADERS"`
	Insecure bool   `env:"OTEL_EXPORTER_OTLP_INSECURE"`
}

// Init installs the global TracerProvider and W3C propagators.
// The returned function flushes pending spans and must be called on shutdown.
// When cfg.Endpoint is empty only propagators are installed and tracing stays a no-op.
func Init(ctx context.Context, serviceName string, cfg Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if cfg.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	endpoint, insecure := cfg.Endpoint, cfg.Insecure
	switch {
	case strings.HasPrefix(endpoint, "http://"):
		endpoint, insecure = strings.TrimPrefix(endpoint, "http://"), true
	case strings.HasPrefix(endpoint, "https://"):
		endpoint, insecure = strings.TrimPrefix(endpoint, "https://"), false
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(strings.TrimSuffix(endpoint, "/"))}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})))
	}
	if h := parseHeaders(cfg.Headers); len(h) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(h))
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func parseHeaders(s string) map[string]string {
	headers := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return headers
}
