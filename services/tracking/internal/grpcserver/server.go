// Package grpcserver adapts the tracking service to the generated TrackingService gRPC API.
package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	trackingv1 "github.com/Andas-LV/cargoflow/gen/go/tracking/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/service"
)

type Server struct {
	trackingv1.UnimplementedTrackingServiceServer
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service, log *slog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) AddPosition(ctx context.Context, req *trackingv1.AddPositionRequest) (*trackingv1.AddPositionResponse, error) {
	userID, err := grpcx.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateShipmentID(req.GetShipmentId()); err != nil {
		return nil, err
	}
	p, err := s.svc.AddPosition(ctx, service.AddPositionInput{
		UserID:     userID,
		ShipmentID: req.GetShipmentId(),
		Station:    req.GetStation(),
		Lat:        req.GetLat(),
		Lon:        req.GetLon(),
	})
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &trackingv1.AddPositionResponse{Position: toProto(p)}, nil
}

func (s *Server) GetRoute(ctx context.Context, req *trackingv1.GetRouteRequest) (*trackingv1.GetRouteResponse, error) {
	userID, err := grpcx.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateShipmentID(req.GetShipmentId()); err != nil {
		return nil, err
	}
	route, err := s.svc.GetRoute(ctx, userID, req.GetShipmentId())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &trackingv1.GetRouteResponse{Positions: make([]*trackingv1.Position, 0, len(route))}
	for _, p := range route {
		resp.Positions = append(resp.Positions, toProto(p))
	}
	return resp, nil
}

func validateShipmentID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return status.Error(codes.InvalidArgument, "shipment_id must be a UUID")
	}
	return nil
}

func toProto(p service.Position) *trackingv1.Position {
	return &trackingv1.Position{
		Id:         p.ID,
		ShipmentId: p.ShipmentID,
		Station:    p.Station,
		Lat:        p.Lat,
		Lon:        p.Lon,
		Source:     p.Source,
		RecordedAt: timestamppb.New(p.RecordedAt),
	}
}

func (s *Server) toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		s.log.ErrorContext(ctx, "tracking internal error", "err", err)
		return status.Error(codes.Internal, "internal error")
	}
}
