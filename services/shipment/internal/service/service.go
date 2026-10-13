// Package service implements shipment use cases. Every state change writes an event to
// the outbox in the same transaction.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/Andas-LV/cargoflow/gen/go/events/v1"
	"github.com/Andas-LV/cargoflow/pkg/natsx"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/events"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository/db"
)

var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrNotFound          = errors.New("shipment not found")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type Shipment = db.Shipment

type CargoType string

const (
	CargoWagon     CargoType = "wagon"
	CargoContainer CargoType = "container"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusInTransit Status = "in_transit"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
)

// transitions lists allowed status changes.
var transitions = map[Status][]Status{
	StatusCreated:   {StatusInTransit, StatusCancelled},
	StatusInTransit: {StatusDelivered, StatusCancelled},
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Service struct {
	store *repository.Store
	cache *repository.Cache
	log   *slog.Logger
}

func New(store *repository.Store, cache *repository.Cache, log *slog.Logger) *Service {
	return &Service{store: store, cache: cache, log: log}
}

type CreateInput struct {
	OwnerID     uuid.UUID
	CargoType   CargoType
	Origin      string
	Destination string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Shipment, error) {
	in.Origin, in.Destination = strings.TrimSpace(in.Origin), strings.TrimSpace(in.Destination)
	switch {
	case in.CargoType != CargoWagon && in.CargoType != CargoContainer:
		return Shipment{}, fmt.Errorf("%w: cargo_type is required", ErrInvalidInput)
	case in.Origin == "" || in.Destination == "":
		return Shipment{}, fmt.Errorf("%w: origin and destination are required", ErrInvalidInput)
	case in.Origin == in.Destination:
		return Shipment{}, fmt.Errorf("%w: origin and destination must differ", ErrInvalidInput)
	}

	var created Shipment
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		var err error
		created, err = q.CreateShipment(ctx, db.CreateShipmentParams{
			ID:          uuid.New(),
			OwnerID:     in.OwnerID,
			CargoType:   string(in.CargoType),
			Origin:      in.Origin,
			Destination: in.Destination,
			Status:      string(StatusCreated),
		})
		if err != nil {
			return err
		}
		eventID := uuid.New()
		return events.Enqueue(ctx, q, natsx.SubjectShipmentCreated, eventID, &eventsv1.ShipmentCreated{
			EventId:     eventID.String(),
			ShipmentId:  created.ID.String(),
			OwnerId:     created.OwnerID.String(),
			CargoType:   created.CargoType,
			Origin:      created.Origin,
			Destination: created.Destination,
			OccurredAt:  timestamppb.New(created.CreatedAt),
		})
	})
	if err != nil {
		return Shipment{}, fmt.Errorf("create shipment: %w", err)
	}
	return created, nil
}

// Get returns a shipment owned by ownerID. Reads go through the Redis cache; cache errors
// are logged and never fail the request.
func (s *Service) Get(ctx context.Context, ownerID, id uuid.UUID) (Shipment, error) {
	sh, hit, err := s.cache.Get(ctx, id)
	if err != nil {
		s.log.WarnContext(ctx, "shipment cache get", "err", err)
	}
	if !hit {
		sh, err = s.store.GetShipment(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return Shipment{}, ErrNotFound
		}
		if err != nil {
			return Shipment{}, err
		}
		if err := s.cache.Set(ctx, sh); err != nil {
			s.log.WarnContext(ctx, "shipment cache set", "err", err)
		}
	}
	// Another user's shipment looks exactly like a missing one.
	if sh.OwnerID != ownerID {
		return Shipment{}, ErrNotFound
	}
	return sh, nil
}

func (s *Service) List(ctx context.Context, ownerID uuid.UUID, pageSize int32) ([]Shipment, error) {
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	pageSize = min(pageSize, maxPageSize)
	return s.store.ListShipmentsByOwner(ctx, db.ListShipmentsByOwnerParams{OwnerID: ownerID, Limit: pageSize})
}

func (s *Service) UpdateStatus(ctx context.Context, ownerID, id uuid.UUID, next Status) (Shipment, error) {
	var updated Shipment
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		cur, err := q.GetShipmentForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && cur.OwnerID != ownerID) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !canTransition(Status(cur.Status), next) {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, cur.Status, next)
		}

		updated, err = q.UpdateShipmentStatus(ctx, db.UpdateShipmentStatusParams{ID: id, Status: string(next)})
		if err != nil {
			return err
		}
		eventID := uuid.New()
		return events.Enqueue(ctx, q, natsx.SubjectShipmentStatusChanged, eventID, &eventsv1.ShipmentStatusChanged{
			EventId:    eventID.String(),
			ShipmentId: updated.ID.String(),
			OwnerId:    updated.OwnerID.String(),
			OldStatus:  cur.Status,
			NewStatus:  updated.Status,
			OccurredAt: timestamppb.New(updated.UpdatedAt),
		})
	})
	if err != nil {
		return Shipment{}, err
	}
	if err := s.cache.Delete(ctx, id); err != nil {
		s.log.WarnContext(ctx, "shipment cache delete", "err", err)
	}
	return updated, nil
}

func canTransition(from, to Status) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
