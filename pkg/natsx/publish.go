package natsx

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

const (
	tracerName       = "github.com/Andas-LV/cargoflow/pkg/natsx"
	contentTypeProto = "application/protobuf"
)

// Publish marshals msg and publishes it to subject in a producer span.
// msgID becomes the Nats-Msg-Id header, so JetStream drops duplicates within the stream's
// Duplicates window; pass a stable event ID to make retries idempotent.
func (c *Conn) Publish(ctx context.Context, subject, msgID string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", subject, err)
	}
	return c.PublishRaw(ctx, subject, msgID, data, nil)
}

// PublishRaw publishes an already encoded payload. Extra headers (may be nil) are copied
// before the current trace context is injected.
func (c *Conn) PublishRaw(ctx context.Context, subject, msgID string, data []byte, header nats.Header) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "publish "+subject,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.message.id", msgID),
		),
	)
	defer span.End()

	m := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	for k, v := range header {
		m.Header[k] = v
	}
	m.Header.Set("Content-Type", contentTypeProto)
	m.Header.Set(nats.MsgIdHdr, msgID)
	Inject(ctx, m.Header)

	if _, err := c.JS.PublishMsg(ctx, m); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
