// Package grpcserver adapts the notification service to the generated NotificationService gRPC API.
package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/Andas-LV/cargoflow/gen/go/notification/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
	"github.com/Andas-LV/cargoflow/services/notification/internal/service"
)

type Server struct {
	notificationv1.UnimplementedNotificationServiceServer
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service, log *slog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) CreateTelegramLink(ctx context.Context, _ *notificationv1.CreateTelegramLinkRequest) (*notificationv1.CreateTelegramLinkResponse, error) {
	userID, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	url, expiresAt, err := s.svc.CreateTelegramLink(ctx, userID)
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &notificationv1.CreateTelegramLinkResponse{Url: url, ExpiresAt: timestamppb.New(expiresAt)}, nil
}

func (s *Server) GetTelegramStatus(ctx context.Context, _ *notificationv1.GetTelegramStatusRequest) (*notificationv1.GetTelegramStatusResponse, error) {
	userID, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	link, linked, err := s.svc.TelegramStatus(ctx, userID)
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &notificationv1.GetTelegramStatusResponse{Linked: linked}
	if linked {
		resp.LinkedAt = timestamppb.New(link.LinkedAt)
	}
	return resp, nil
}

func (s *Server) UnlinkTelegram(ctx context.Context, _ *notificationv1.UnlinkTelegramRequest) (*notificationv1.UnlinkTelegramResponse, error) {
	userID, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svc.UnlinkTelegram(ctx, userID); err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &notificationv1.UnlinkTelegramResponse{}, nil
}

func currentUser(ctx context.Context) (uuid.UUID, error) {
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

func (s *Server) toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrTelegramDisabled):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		s.log.ErrorContext(ctx, "notification internal error", "err", err)
		return status.Error(codes.Internal, "internal error")
	}
}
