// Package redisx connects to Redis.
package redisx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// New parses url (redis:// or rediss:// for TLS, e.g. Upstash), connects and pings.
func New(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return rdb, nil
}

// Check returns a readiness check for rdb.
func Check(rdb *redis.Client) func(ctx context.Context) error {
	return func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
}
