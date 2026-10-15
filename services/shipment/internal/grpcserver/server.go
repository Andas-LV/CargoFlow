// Package grpcserver adapts the shipment service to the generated ShipmentService gRPC API.
package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	shipmentv1 "github.com/Andas-LV/cargoflow/gen/go/shipment/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/service"
)

type Server struct {
	shipmentv1.UnimplementedShipmentServiceServer
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service, log *slog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) CreateShipment(ctx context.Context, req *shipmentv1.CreateShipmentRequest) (*shipmentv1.CreateShipmentResponse, error) {
	owner, err := ownerID(ctx)
	if err != nil {
		return nil, err
	}
	sh, err := s.svc.Create(ctx, service.CreateInput{
		OwnerID:              owner,
		CargoType:            cargoFromProto[req.GetCargoType()],
		OriginStationID:      req.GetOriginStationId(),
		DestinationStationID: req.GetDestinationStationId(),
	})
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &shipmentv1.CreateShipmentResponse{Shipment: toProto(sh)}, nil
}

func (s *Server) GetShipment(ctx context.Context, req *shipmentv1.GetShipmentRequest) (*shipmentv1.GetShipmentResponse, error) {
	owner, err := ownerID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	sh, err := s.svc.Get(ctx, owner, id)
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &shipmentv1.GetShipmentResponse{Shipment: toProto(sh)}, nil
}

func (s *Server) ListShipments(ctx context.Context, req *shipmentv1.ListShipmentsRequest) (*shipmentv1.ListShipmentsResponse, error) {
	owner, err := ownerID(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.svc.List(ctx, owner, req.GetPageSize())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &shipmentv1.ListShipmentsResponse{Shipments: make([]*shipmentv1.Shipment, 0, len(list))}
	for _, sh := range list {
		resp.Shipments = append(resp.Shipments, toProto(sh))
	}
	return resp, nil
}

func (s *Server) UpdateShipmentStatus(ctx context.Context, req *shipmentv1.UpdateShipmentStatusRequest) (*shipmentv1.UpdateShipmentStatusResponse, error) {
	owner, err := ownerID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId())
	if err != nil {
		return nil, err
	}
	next, ok := statusFromProto[req.GetStatus()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status is required")
	}
	sh, err := s.svc.UpdateStatus(ctx, owner, id, next)
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &shipmentv1.UpdateShipmentStatusResponse{Shipment: toProto(sh)}, nil
}

func ownerID(ctx context.Context) (uuid.UUID, error) {
	raw, err := grpcx.RequireUserID(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "malformed user identity")
	}
	return id, nil
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "id must be a UUID")
	}
	return id, nil
}

var (
	cargoFromProto = map[shipmentv1.CargoType]service.CargoType{
		shipmentv1.CargoType_CARGO_TYPE_WAGON:     service.CargoWagon,
		shipmentv1.CargoType_CARGO_TYPE_CONTAINER: service.CargoContainer,
	}
	cargoToProto = map[string]shipmentv1.CargoType{
		string(service.CargoWagon):     shipmentv1.CargoType_CARGO_TYPE_WAGON,
		string(service.CargoContainer): shipmentv1.CargoType_CARGO_TYPE_CONTAINER,
	}
	statusFromProto = map[shipmentv1.ShipmentStatus]service.Status{
		shipmentv1.ShipmentStatus_SHIPMENT_STATUS_CREATED:    service.StatusCreated,
		shipmentv1.ShipmentStatus_SHIPMENT_STATUS_IN_TRANSIT: service.StatusInTransit,
		shipmentv1.ShipmentStatus_SHIPMENT_STATUS_DELIVERED:  service.StatusDelivered,
		shipmentv1.ShipmentStatus_SHIPMENT_STATUS_CANCELLED:  service.StatusCancelled,
	}
	statusToProto = map[string]shipmentv1.ShipmentStatus{
		string(service.StatusCreated):   shipmentv1.ShipmentStatus_SHIPMENT_STATUS_CREATED,
		string(service.StatusInTransit): shipmentv1.ShipmentStatus_SHIPMENT_STATUS_IN_TRANSIT,
		string(service.StatusDelivered): shipmentv1.ShipmentStatus_SHIPMENT_STATUS_DELIVERED,
		string(service.StatusCancelled): shipmentv1.ShipmentStatus_SHIPMENT_STATUS_CANCELLED,
	}
)

func toProto(sh service.View) *shipmentv1.Shipment {
	return &shipmentv1.Shipment{
		Id:                 sh.ID.String(),
		OwnerId:            sh.OwnerID.String(),
		CargoType:          cargoToProto[sh.CargoType],
		Status:             statusToProto[sh.Status],
		CreatedAt:          timestamppb.New(sh.CreatedAt),
		UpdatedAt:          timestamppb.New(sh.UpdatedAt),
		OriginStation:      stationToProto(sh.Origin),
		DestinationStation: stationToProto(sh.Destination),
	}
}

func (s *Server) toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrInvalidTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		s.log.ErrorContext(ctx, "shipment internal error", "err", err)
		return status.Error(codes.Internal, "internal error")
	}
}
