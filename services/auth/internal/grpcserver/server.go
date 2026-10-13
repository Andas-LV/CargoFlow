// Package grpcserver adapts the auth service to the generated AuthService gRPC API.
package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Andas-LV/cargoflow/gen/go/auth/v1"
	"github.com/Andas-LV/cargoflow/services/auth/internal/service"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service, log *slog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	u, err := s.svc.Register(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &authv1.RegisterResponse{UserId: u.ID.String(), Email: u.Email}, nil
}

func (s *Server) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	pair, err := s.svc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &authv1.LoginResponse{Tokens: toProto(pair)}, nil
}

func (s *Server) Refresh(ctx context.Context, req *authv1.RefreshRequest) (*authv1.RefreshResponse, error) {
	pair, err := s.svc.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &authv1.RefreshResponse{Tokens: toProto(pair)}, nil
}

func (s *Server) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	userID, err := s.svc.ValidateToken(ctx, req.GetAccessToken())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &authv1.ValidateTokenResponse{UserId: userID}, nil
}

func toProto(p service.TokenPair) *authv1.TokenPair {
	return &authv1.TokenPair{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    int64(p.ExpiresIn.Seconds()),
	}
}

// toStatus maps domain errors to gRPC codes; grpc-gateway then maps codes to HTTP statuses.
func (s *Server) toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrEmailTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, service.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, service.ErrInvalidToken):
		return status.Error(codes.Unauthenticated, service.ErrInvalidToken.Error())
	default:
		s.log.ErrorContext(ctx, "auth internal error", "err", err)
		return status.Error(codes.Internal, "internal error")
	}
}
