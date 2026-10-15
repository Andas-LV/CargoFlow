package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Andas-LV/cargoflow/services/shipment/internal/repository/db"
)

// Cache keeps shipments in Redis as JSON under shipment:v2:<id>.
// The version changes with the cached struct, so old entries are simply never read.
type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewCache(rdb *redis.Client, ttl time.Duration) *Cache {
	return &Cache{rdb: rdb, ttl: ttl}
}

// Get returns (shipment, true) on a hit and (zero, false) on a miss.
func (c *Cache) Get(ctx context.Context, id uuid.UUID) (db.Shipment, bool, error) {
	raw, err := c.rdb.Get(ctx, cacheKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return db.Shipment{}, false, nil
	}
	if err != nil {
		return db.Shipment{}, false, err
	}
	var s db.Shipment
	if err := json.Unmarshal(raw, &s); err != nil {
		return db.Shipment{}, false, err
	}
	return s, true, nil
}

func (c *Cache) Set(ctx context.Context, s db.Shipment) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, cacheKey(s.ID), raw, c.ttl).Err()
}

func (c *Cache) Delete(ctx context.Context, id uuid.UUID) error {
	return c.rdb.Del(ctx, cacheKey(id)).Err()
}

func cacheKey(id uuid.UUID) string { return "shipment:v2:" + id.String() }
