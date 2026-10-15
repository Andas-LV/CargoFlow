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
	ListByShipment(ctx context.Context, shipmentID uuid.UUID) ([]Position, error)
}

type ShipmentChecker interface {
	CheckOwner(ctx context.Context, userID, shipmentID string) error
}

// Station is the part of a directory entry that tracking stores with a position.
type Station struct {
	ID       string
	Name     string // Russian name
	Lat, Lon float64
}

// StationLookup reads the station directory (owned by the shipment service).
type StationLookup interface {
	Station(ctx context.Context, id string) (Station, error)
}

type Publisher interface {
	Publish(ctx context.Context, subject, msgID string, msg proto.Message) error
}

type Service struct {
	positions PositionStore
	shipments ShipmentChecker
	stations  StationLookup
	events    Publisher
	log       *slog.Logger
	now       func() time.Time
}

func New(positions PositionStore, shipments ShipmentChecker, stations StationLookup, events Publisher, log *slog.Logger) *Service {
	return &Service{positions: positions, shipments: shipments, stations: stations, events: events, log: log, now: time.Now}
}

type AddPositionInput struct {
	UserID     string
	ShipmentID uuid.UUID
	StationID  string
}

// AddPosition stores a manual position and publishes tracking.position_updated.
//
// The event is published directly after the insert (no outbox) to keep the example small:
// if NATS is down the position is saved but the event is lost. A production version would
// use a transactional outbox, like the shipment service does.
func (s *Service) AddPosition(ctx context.Context, in AddPositionInput) (Position, error) {
	in.StationID = strings.TrimSpace(in.StationID)
	if in.StationID == "" {
		return Position{}, fmt.Errorf("%w: station_id is required", ErrInvalidInput)
	}
	if err := s.shipments.CheckOwner(ctx, in.UserID, in.ShipmentID.String()); err != nil {
		return Position{}, err
	}
	station, err := s.stations.Station(ctx, in.StationID)
	if err != nil {
		return Position{}, err
	}

	p := Position{
		ID:         uuid.New(),
		ShipmentID: in.ShipmentID,
		StationID:  station.ID,
		Station:    station.Name,
		Lat:        station.Lat,
		Lon:        station.Lon,
		Source:     SourceManual,
		RecordedAt: s.now().UTC(),
	}
	if _, err := s.positions.Insert(ctx, p); err != nil {
		return Position{}, fmt.Errorf("insert position: %w", err)
	}

	ev := &eventsv1.PositionUpdated{
		EventId:    p.ID.String(),
		ShipmentId: p.ShipmentID.String(),
		Station:    p.Station,
		Lat:        p.Lat,
		Lon:        p.Lon,
		OccurredAt: timestamppb.New(p.RecordedAt),
		OwnerId:    in.UserID, // CheckOwner above guarantees the caller owns the shipment
		StationId:  p.StationID,
	}
	if err := s.events.Publish(ctx, natsx.SubjectPositionUpdated, ev.GetEventId(), ev); err != nil {
		s.log.ErrorContext(ctx, "publish position_updated", "err", err, "position_id", p.ID)
	}
	return p, nil
}

func (s *Service) GetRoute(ctx context.Context, userID string, shipmentID uuid.UUID) ([]Position, error) {
	if err := s.shipments.CheckOwner(ctx, userID, shipmentID.String()); err != nil {
		return nil, err
	}
	return s.positions.ListByShipment(ctx, shipmentID)
}

// HandleShipmentCreated opens the route with the origin station. The event ID is used as
// the row ID, so redelivered events are ignored.
func (s *Service) HandleShipmentCreated(ctx context.Context, ev *eventsv1.ShipmentCreated) error {
	eventID, err1 := uuid.Parse(ev.GetEventId())
	shipmentID, err2 := uuid.Parse(ev.GetShipmentId())
	if err := errors.Join(err1, err2); err != nil {
		// Retrying cannot fix a malformed event: log and acknowledge it.
		s.log.ErrorContext(ctx, "drop shipment.created with bad IDs", "err", err)
		return nil
	}

	// Coordinates come from the directory; if shipment is unavailable the event is retried.
	station, err := s.stations.Station(ctx, ev.GetOriginStationId())
	if err != nil {
		return fmt.Errorf("origin station %q: %w", ev.GetOriginStationId(), err)
	}

	inserted, err := s.positions.Insert(ctx, Position{
		ID:         eventID,
		ShipmentID: shipmentID,
		StationID:  station.ID,
		Station:    station.Name,
		Lat:        station.Lat,
		Lon:        station.Lon,
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
