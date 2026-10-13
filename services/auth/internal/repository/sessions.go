package repository

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Sessions stores active refresh tokens in Redis as refresh:<token id> -> user id.
type Sessions struct {
	rdb *redis.Client
}

func NewSessions(rdb *redis.Client) *Sessions {
	return &Sessions{rdb: rdb}
}

func (s *Sessions) Save(ctx context.Context, tokenID, userID string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key(tokenID), userID, ttl).Err()
}

// Consume atomically reads and deletes a session, so every refresh token works exactly once.
func (s *Sessions) Consume(ctx context.Context, tokenID string) (string, error) {
	userID, err := s.rdb.GetDel(ctx, key(tokenID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	return userID, err
}

func key(tokenID string) string { return "refresh:" + tokenID }
