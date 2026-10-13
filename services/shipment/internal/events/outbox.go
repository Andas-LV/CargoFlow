package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository/db"
)

const relayBatchSize = 100

// Relay periodically moves unsent outbox rows to NATS JetStream.
//
// Delivery is at-least-once: if the process dies after publishing but before the commit,
// the row is published again. JetStream drops the duplicate thanks to Nats-Msg-Id, and
// consumers are idempotent anyway.
type Relay struct {
	store    *repository.Store
	nats     *natsx.Conn
	interval time.Duration
	log      *slog.Logger
}

func NewRelay(store *repository.Store, nats *natsx.Conn, interval time.Duration, log *slog.Logger) *Relay {
	return &Relay{store: store, nats: nats, interval: interval, log: log}
}

// Run polls the outbox until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if n, err := r.flush(ctx); err != nil {
				r.log.Error("outbox relay", "err", err, "sent", n)
			} else if n > 0 {
				r.log.Debug("outbox relay", "sent", n)
			}
		}
	}
}

// flush publishes one batch inside a transaction. Rows are locked with SKIP LOCKED, so
// several shipment replicas can relay concurrently. Rows published before an error are
// still marked as sent.
func (r *Relay) flush(ctx context.Context) (int, error) {
	var sent []uuid.UUID
	var publishErr error

	err := r.store.InTx(ctx, func(q *db.Queries) error {
		rows, err := q.LockUnsentOutbox(ctx, relayBatchSize)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if publishErr = r.publish(ctx, row); publishErr != nil {
				break
			}
			sent = append(sent, row.ID)
		}
		if len(sent) == 0 {
			return nil
		}
		return q.MarkOutboxSent(ctx, sent)
	})
	if err != nil {
		return 0, err
	}
	return len(sent), publishErr
}

func (r *Relay) publish(ctx context.Context, row db.LockUnsentOutboxRow) error {
	carrier := propagation.MapCarrier{}
	if err := json.Unmarshal(row.Headers, &carrier); err != nil {
		r.log.Warn("outbox: bad trace headers", "id", row.ID, "err", err)
	}
	// Continue the trace of the request that created the event.
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	return r.nats.PublishRaw(ctx, row.Subject, row.ID.String(), row.Payload, nil)
}
