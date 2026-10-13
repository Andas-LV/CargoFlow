// Package service implements tracking use cases: recording positions and reading routes.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/Andas-LV/cargoflow/gen/go/events/v1"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/repository"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("shipment not found")
)

const SourceManual = "manual"

type Position = repository.Position

type PositionStore interface {
	Insert(ctx context.Context, p Position) (bool, error)
	ListByShipment(ctx context.Context, shipmentID string) ([]Position, error)
}

type ShipmentChecker interface {
	CheckOwner(ctx context.Context, userID, shipmentID string) error
}

type Publisher interface {
	Publish(ctx context.Context, subject, msgID string, msg proto.Message) error
}

type Service struct {
	positions PositionStore
	shipments ShipmentChecker
	events    Publisher
	log       *slog.Logger
	now       func() time.Time
}

func New(positions PositionStore, shipments ShipmentChecker, events Publisher, log *slog.Logger) *Service {
	return &Service{positions: positions, shipments: shipments, events: events, log: log, now: time.Now}
}

type AddPositionInput struct {
	UserID     string
	ShipmentID string
	Station    string
	Lat, Lon   float64
}

// AddPosition stores a manual position and publishes tracking.position_updated.
//
// The event is published directly after the insert (no outbox) to keep the example small:
// if NATS is down the position is saved but the event is lost. A production version would
// use an outbox collection or MongoDB change streams, like the shipment service does.
func (s *Service) AddPosition(ctx context.Context, in AddPositionInput) (Position, error) {
	in.Station = strings.TrimSpace(in.Station)
	if in.Station == "" {
		return Position{}, fmt.Errorf("%w: station is required", ErrInvalidInput)
	}
	if err := s.shipments.CheckOwner(ctx, in.UserID, in.ShipmentID); err != nil {
		return Position{}, err
	}

	p := Position{
		ID:         uuid.NewString(),
		ShipmentID: in.ShipmentID,
		Station:    in.Station,
		Lat:        in.Lat,
		Lon:        in.Lon,
		Source:     SourceManual,
		RecordedAt: s.now().UTC(),
	}
	if _, err := s.positions.Insert(ctx, p); err != nil {
		return Position{}, fmt.Errorf("insert position: %w", err)
	}

	ev := &eventsv1.PositionUpdated{
		EventId:    p.ID,
		ShipmentId: p.ShipmentID,
		Station:    p.Station,
		Lat:        p.Lat,
		Lon:        p.Lon,
		OccurredAt: timestamppb.New(p.RecordedAt),
	}
	if err := s.events.Publish(ctx, natsx.SubjectPositionUpdated, ev.GetEventId(), ev); err != nil {
		s.log.ErrorContext(ctx, "publish position_updated", "err", err, "position_id", p.ID)
	}
	return p, nil
}

func (s *Service) GetRoute(ctx context.Context, userID, shipmentID string) ([]Position, error) {
	if err := s.shipments.CheckOwner(ctx, userID, shipmentID); err != nil {
		return nil, err
	}
	return s.positions.ListByShipment(ctx, shipmentID)
}

// HandleShipmentCreated opens the route with the origin station. The event ID is used as
// the document ID, so redelivered events are ignored.
func (s *Service) HandleShipmentCreated(ctx context.Context, ev *eventsv1.ShipmentCreated) error {
	inserted, err := s.positions.Insert(ctx, Position{
		ID:         ev.GetEventId(),
		ShipmentID: ev.GetShipmentId(),
		Station:    ev.GetOrigin(),
		Source:     natsx.SubjectShipmentCreated,
		RecordedAt: ev.GetOccurredAt().AsTime(),
	})
	if err != nil {
		return err
	}
	s.log.InfoContext(ctx, "route started",
		"shipment_id", ev.GetShipmentId(), "station", ev.GetOrigin(), "duplicate", !inserted)
	return nil
}
