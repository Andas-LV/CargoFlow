package natsx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Handler processes one message. Returning an error NAKs the message for redelivery,
// so handlers must be idempotent.
type Handler func(ctx context.Context, msg jetstream.Msg) error

// ConsumerConfig describes a durable consumer on the CARGO stream.
type ConsumerConfig struct {
	Durable        string
	FilterSubjects []string
	// MaxDeliver limits redeliveries of a failing message. Defaults to 5.
	MaxDeliver int
}

// Consumer is a running JetStream subscription.
type Consumer struct {
	cc jetstream.ConsumeContext
}

// Consume creates (or updates) a durable consumer and starts delivering messages to h.
// Every message is processed in a consumer span that continues the publisher's trace.
func (c *Conn) Consume(ctx context.Context, cfg ConsumerConfig, h Handler, log *slog.Logger) (*Consumer, error) {
	if cfg.MaxDeliver == 0 {
		cfg.MaxDeliver = 5
	}
	cons, err := c.JS.CreateOrUpdateConsumer(ctx, StreamName, jetstream.ConsumerConfig{
		Durable:        cfg.Durable,
		FilterSubjects: cfg.FilterSubjects,
		AckPolicy:      jetstream.AckExplicitPolicy,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		AckWait:        30 * time.Second,
		MaxDeliver:     cfg.MaxDeliver,
	})
	if err != nil {
		return nil, fmt.Errorf("consumer %s: %w", cfg.Durable, err)
	}

	tracer := otel.Tracer(tracerName)
	// Handlers outlive the startup ctx; shutdown happens via Consumer.Stop.
	base := context.WithoutCancel(ctx)

	cc, err := cons.Consume(func(m jetstream.Msg) {
		msgCtx, span := tracer.Start(Extract(base, m.Headers()), "process "+m.Subject(),
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "nats"),
				attribute.String("messaging.destination.name", m.Subject()),
				attribute.String("messaging.consumer.group.name", cfg.Durable),
				attribute.String("messaging.message.id", m.Headers().Get(nats.MsgIdHdr)),
			),
		)
		defer span.End()

		if err := h(msgCtx, m); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			log.ErrorContext(msgCtx, "handle message", "subject", m.Subject(), "err", err)
			_ = m.NakWithDelay(2 * time.Second)
			return
		}
		if err := m.Ack(); err != nil {
			log.ErrorContext(msgCtx, "ack message", "subject", m.Subject(), "err", err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("consume %s: %w", cfg.Durable, err)
	}
	return &Consumer{cc: cc}, nil
}

// Stop drains the subscription: no new messages are fetched and the current one finishes.
func (c *Consumer) Stop(ctx context.Context) error {
	c.cc.Drain()
	select {
	case <-c.cc.Closed():
	case <-ctx.Done():
		c.cc.Stop()
	}
	return nil
}
