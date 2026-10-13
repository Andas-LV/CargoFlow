// Package service implements the auth use cases: register, login, refresh, validate.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/Andas-LV/cargoflow/services/auth/internal/repository"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid token")
)

const minPasswordLen = 8

type UserStore interface {
	Create(ctx context.Context, id uuid.UUID, email, passwordHash string) (repository.User, error)
	GetByEmail(ctx context.Context, email string) (repository.User, error)
}

type SessionStore interface {
	Save(ctx context.Context, tokenID, userID string, ttl time.Duration) error
	Consume(ctx context.Context, tokenID string) (userID string, err error)
}

type Service struct {
	users      UserStore
	sessions   SessionStore
	tokens     *Tokens
	refreshTTL time.Duration
}

func New(users UserStore, sessions SessionStore, tokens *Tokens) *Service {
	return &Service{users: users, sessions: sessions, tokens: tokens, refreshTTL: tokens.refreshTTL}
}

func (s *Service) Register(ctx context.Context, email, password string) (repository.User, error) {
	email = normalizeEmail(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return repository.User{}, fmt.Errorf("%w: email is not valid", ErrInvalidInput)
	}
	if len(password) < minPasswordLen {
		return repository.User{}, fmt.Errorf("%w: password must be at least %d characters", ErrInvalidInput, minPasswordLen)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return repository.User{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := s.users.Create(ctx, uuid.New(), email, string(hash))
	if errors.Is(err, repository.ErrDuplicate) {
		return repository.User{}, ErrEmailTaken
	}
	return u, err
}

// dummyHash is compared against when the user does not exist, so that Login takes the same
// time for unknown emails and wrong passwords (no user enumeration via timing).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	u, err := s.users.GetByEmail(ctx, normalizeEmail(email))
	switch {
	case errors.Is(err, repository.ErrNotFound):
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return TokenPair{}, ErrInvalidCredentials
	case err != nil:
		return TokenPair{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	return s.issue(ctx, u.ID.String())
}

// Refresh rotates the refresh token: the old one is consumed and cannot be reused.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	userID, tokenID, err := s.tokens.ParseRefresh(refreshToken)
	if err != nil {
		return TokenPair{}, err
	}
	stored, err := s.sessions.Consume(ctx, tokenID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return TokenPair{}, fmt.Errorf("%w: refresh token revoked or already used", ErrInvalidToken)
	case err != nil:
		return TokenPair{}, err
	case stored != userID:
		return TokenPair{}, fmt.Errorf("%w: session mismatch", ErrInvalidToken)
	}
	return s.issue(ctx, userID)
}

func (s *Service) ValidateToken(_ context.Context, accessToken string) (string, error) {
	return s.tokens.ParseAccess(accessToken)
}

func (s *Service) issue(ctx context.Context, userID string) (TokenPair, error) {
	pair, refreshID, err := s.tokens.Issue(userID)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.sessions.Save(ctx, refreshID, userID, s.refreshTTL); err != nil {
		return TokenPair{}, fmt.Errorf("save session: %w", err)
	}
	return pair, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
