// Package consumer turns CARGO stream events into notifications.
package consumer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/Andas-LV/cargoflow/gen/go/events/v1"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/services/notification/internal/notifier"
)

const durableName = "notification"

// Start consumes all shipment.* and tracking.* events with the durable consumer "notification".
func Start(ctx context.Context, nc *natsx.Conn, n notifier.Notifier, log *slog.Logger) (*natsx.Consumer, error) {
	handler := func(ctx context.Context, m jetstream.Msg) error {
		msg, err := toMessage(m.Subject(), m.Data())
		if err != nil {
			// Unknown or malformed events are dropped instead of being redelivered forever.
			log.WarnContext(ctx, "skip event", "subject", m.Subject(), "err", err)
			return nil
		}
		return n.Notify(ctx, msg)
	}

	cons, err := nc.Consume(ctx, natsx.ConsumerConfig{
		Durable:        durableName,
		FilterSubjects: []string{"shipment.>", "tracking.>"},
	}, handler, log)
	if err != nil {
		return nil, fmt.Errorf("start notification consumer: %w", err)
	}
	return cons, nil
}

func toMessage(subject string, data []byte) (notifier.Message, error) {
	switch subject {
	case natsx.SubjectShipmentCreated:
		var ev eventsv1.ShipmentCreated
		if err := proto.Unmarshal(data, &ev); err != nil {
			return notifier.Message{}, err
		}
		return notifier.Message{
			Title: "Shipment created",
			Text:  fmt.Sprintf("Shipment %s (%s): %s → %s", ev.GetShipmentId(), ev.GetCargoType(), ev.GetOrigin(), ev.GetDestination()),
		}, nil

	case natsx.SubjectShipmentStatusChanged:
		var ev eventsv1.ShipmentStatusChanged
		if err := proto.Unmarshal(data, &ev); err != nil {
			return notifier.Message{}, err
		}
		return notifier.Message{
			Title: "Shipment status changed",
			Text:  fmt.Sprintf("Shipment %s: %s → %s", ev.GetShipmentId(), ev.GetOldStatus(), ev.GetNewStatus()),
		}, nil

	case natsx.SubjectPositionUpdated:
		var ev eventsv1.PositionUpdated
		if err := proto.Unmarshal(data, &ev); err != nil {
			return notifier.Message{}, err
		}
		return notifier.Message{
			Title: "Shipment moved",
			Text:  fmt.Sprintf("Shipment %s is at %s", ev.GetShipmentId(), ev.GetStation()),
		}, nil

	default:
		return notifier.Message{}, fmt.Errorf("unknown subject %q", subject)
	}
}
