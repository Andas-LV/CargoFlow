// Package events implements the transactional outbox of the shipment service:
// Enqueue writes an event in the caller's transaction, Relay delivers it to NATS.
package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/proto"

	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository/db"
)

// Enqueue stores msg in the outbox using q, which must be bound to the same transaction as
// the business change. eventID doubles as outbox row ID and Nats-Msg-Id (deduplication).
// The current trace context is saved with the event so the relay can continue the trace.
func Enqueue(ctx context.Context, q *db.Queries, subject string, eventID uuid.UUID, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", subject, err)
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	headers, err := json.Marshal(carrier)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	return q.InsertOutbox(ctx, db.InsertOutboxParams{
		ID:      eventID,
		Subject: subject,
		Payload: payload,
		Headers: headers,
	})
}
