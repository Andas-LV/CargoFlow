// Package consumer subscribes the tracking service to shipment events.
package consumer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/Andas-LV/cargoflow/gen/go/events/v1"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/service"
)

const durableName = "tracking"

// Start begins consuming shipment.created with the durable consumer "tracking".
func Start(ctx context.Context, nc *natsx.Conn, svc *service.Service, log *slog.Logger) (*natsx.Consumer, error) {
	handler := func(ctx context.Context, m jetstream.Msg) error {
		var ev eventsv1.ShipmentCreated
		if err := proto.Unmarshal(m.Data(), &ev); err != nil {
			// A malformed message will never succeed; ack it (by returning nil) instead of retrying.
			log.ErrorContext(ctx, "drop malformed event", "subject", m.Subject(), "err", err)
			return nil
		}
		return svc.HandleShipmentCreated(ctx, &ev)
	}

	cons, err := nc.Consume(ctx, natsx.ConsumerConfig{
		Durable:        durableName,
		FilterSubjects: []string{natsx.SubjectShipmentCreated},
	}, handler, log)
	if err != nil {
		return nil, fmt.Errorf("start tracking consumer: %w", err)
	}
	return cons, nil
}
