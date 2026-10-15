package repository

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// LinkCodes stores one-time Telegram link codes in Redis as tglink:<code> -> user id.
type LinkCodes struct {
	rdb *redis.Client
}

func NewLinkCodes(rdb *redis.Client) *LinkCodes {
	return &LinkCodes{rdb: rdb}
}

func (c *LinkCodes) Save(ctx context.Context, code, userID string, ttl time.Duration) error {
	return c.rdb.Set(ctx, codeKey(code), userID, ttl).Err()
}

// Consume atomically reads and deletes a code, so each link works exactly once.
func (c *LinkCodes) Consume(ctx context.Context, code string) (string, error) {
	userID, err := c.rdb.GetDel(ctx, codeKey(code)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	return userID, err
}

func codeKey(code string) string { return "tglink:" + code }
