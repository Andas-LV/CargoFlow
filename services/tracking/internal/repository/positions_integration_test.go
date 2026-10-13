//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/Andas-LV/cargoflow/pkg/mongox"
	"github.com/Andas-LV/cargoflow/services/tracking/internal/repository"
)

// Run with: go test -tags integration ./... (requires Docker).
func TestPositions_Mongo(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcmongo.Run(ctx, "mongo:7")
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	client, err := mongox.Connect(ctx, uri)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	positions := repository.NewPositions(client.Database("tracking_test"))
	require.NoError(t, positions.EnsureIndexes(ctx))

	now := time.Now().UTC().Truncate(time.Millisecond)
	first := repository.Position{ID: "e1", ShipmentID: "s1", Station: "Astana-1", RecordedAt: now}
	second := repository.Position{ID: "e2", ShipmentID: "s1", Station: "Karaganda", RecordedAt: now.Add(time.Hour)}

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

	route, err := positions.ListByShipment(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, route, 2)
	assert.Equal(t, "Astana-1", route[0].Station)
	assert.Equal(t, "Karaganda", route[1].Station)
}
