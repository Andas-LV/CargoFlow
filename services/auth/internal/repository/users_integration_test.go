//go:build integration

package repository_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Andas-LV/cargoflow/pkg/postgres"
	"github.com/Andas-LV/cargoflow/services/auth/internal/repository"
	"github.com/Andas-LV/cargoflow/services/auth/migrations"
)

// Run with: go test -tags integration ./... (requires Docker).
func TestUsers_Postgres(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("auth"),
		tcpostgres.WithUsername("cargo"),
		tcpostgres.WithPassword("cargo"),
		tcpostgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := postgres.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, postgres.Migrate(ctx, pool, migrations.FS, slog.New(slog.NewTextHandler(io.Discard, nil))))

	users := repository.NewUsers(pool)

	created, err := users.Create(ctx, uuid.New(), "a@b.co", "hash")
	require.NoError(t, err)

	got, err := users.GetByEmail(ctx, "a@b.co")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	_, err = users.Create(ctx, uuid.New(), "a@b.co", "hash")
	assert.ErrorIs(t, err, repository.ErrDuplicate)

	_, err = users.GetByEmail(ctx, "missing@b.co")
	assert.ErrorIs(t, err, repository.ErrNotFound)
}
