package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Andas-LV/cargoflow/services/auth/internal/repository"
)

type fakeUsers struct {
	mu      sync.Mutex
	byEmail map[string]repository.User
}

func (f *fakeUsers) Create(_ context.Context, id uuid.UUID, email, hash string) (repository.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byEmail[email]; ok {
		return repository.User{}, repository.ErrDuplicate
	}
	u := repository.User{ID: id, Email: email, PasswordHash: hash, CreatedAt: time.Now()}
	f.byEmail[email] = u
	return u, nil
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (repository.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byEmail[email]
	if !ok {
		return repository.User{}, repository.ErrNotFound
	}
	return u, nil
}

type fakeSessions struct {
	mu       sync.Mutex
	sessions map[string]string
}

func (f *fakeSessions) Save(_ context.Context, tokenID, userID string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[tokenID] = userID
	return nil
}

func (f *fakeSessions) Consume(_ context.Context, tokenID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	userID, ok := f.sessions[tokenID]
	if !ok {
		return "", repository.ErrNotFound
	}
	delete(f.sessions, tokenID)
	return userID, nil
}

func newTestService() *Service {
	return New(
		&fakeUsers{byEmail: map[string]repository.User{}},
		&fakeSessions{sessions: map[string]string{}},
		newTestTokens(),
	)
}

func TestService_RegisterLoginRefresh(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	u, err := svc.Register(ctx, "  Driver@Example.com ", "password123")
	require.NoError(t, err)
	assert.Equal(t, "driver@example.com", u.Email)

	_, err = svc.Register(ctx, "driver@example.com", "password123")
	assert.ErrorIs(t, err, ErrEmailTaken)

	pair, err := svc.Login(ctx, "DRIVER@example.com", "password123")
	require.NoError(t, err)

	userID, err := svc.ValidateToken(ctx, pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, u.ID.String(), userID)

	rotated, err := svc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, pair.RefreshToken, rotated.RefreshToken)

	// A refresh token works only once.
	_, err = svc.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestService_RegisterValidation(t *testing.T) {
	svc := newTestService()
	tests := map[string]struct{ email, password string }{
		"bad email":      {"not-an-email", "password123"},
		"short password": {"a@b.co", "short"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), tc.email, tc.password)
			assert.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestService_LoginWrongCredentials(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	_, err := svc.Register(ctx, "a@b.co", "password123")
	require.NoError(t, err)

	_, err = svc.Login(ctx, "a@b.co", "wrong-password")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	_, err = svc.Login(ctx, "nobody@b.co", "password123")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}
