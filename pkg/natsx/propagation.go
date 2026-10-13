package natsx

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// HeaderCarrier adapts nats.Header to propagation.TextMapCarrier.
type HeaderCarrier nats.Header

var _ propagation.TextMapCarrier = HeaderCarrier{}

func (c HeaderCarrier) Get(key string) string { return nats.Header(c).Get(key) }

func (c HeaderCarrier) Set(key, value string) { nats.Header(c).Set(key, value) }

func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// Inject writes the trace context of ctx into h (traceparent, tracestate, baggage).
func Inject(ctx context.Context, h nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, HeaderCarrier(h))
}

// Extract returns ctx enriched with the trace context found in h.
func Extract(ctx context.Context, h nats.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, HeaderCarrier(h))
}
