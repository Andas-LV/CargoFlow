package natsx

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestInjectExtract_RoundTrip(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "publish")
	defer span.End()

	h := nats.Header{}
	Inject(ctx, h)
	require.NotEmpty(t, h.Get("traceparent"))

	got := trace.SpanContextFromContext(Extract(context.Background(), h))
	assert.True(t, got.IsRemote())
	assert.Equal(t, span.SpanContext().TraceID(), got.TraceID())
	assert.Equal(t, span.SpanContext().SpanID(), got.SpanID())
}
