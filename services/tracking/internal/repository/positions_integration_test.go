//go:build integration

package repository_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Andas-LV/cargoflow/pkg/postgres"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/repository"
	"github.com/Andas-LV/cargoflow/services/tracking/migrations"
)

// Run with: go test -tags integration ./... (requires Docker).
func TestPositions_Postgres(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("tracking"),
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

	positions := repository.NewPositions(pool)
	shipmentID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	first := repository.Position{ID: uuid.New(), ShipmentID: shipmentID, Station: "Astana-1", Source: "test", RecordedAt: now}
	second := repository.Position{ID: uuid.New(), ShipmentID: shipmentID, Station: "Karaganda", Source: "test", RecordedAt: now.Add(time.Hour)}

	inserted, err := positions.Insert(ctx, second)
	require.NoError(t, err)
	assert.True(t, inserted)
	inserted, err = positions.Insert(ctx, first)
	require.NoError(t, err)
	assert.True(t, inserted)

	// Same ID again: idempotent no-op.
	inserted, err = positions.Insert(ctx, first)
	require.NoError(t, err)
	assert.False(t, inserted)

	route, err := positions.ListByShipment(ctx, shipmentID)
	require.NoError(t, err)
	require.Len(t, route, 2)
	assert.Equal(t, "Astana-1", route[0].Station)
	assert.Equal(t, "Karaganda", route[1].Station)

	empty, err := positions.ListByShipment(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, empty)
}
