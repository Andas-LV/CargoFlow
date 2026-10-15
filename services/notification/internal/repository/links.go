// Package repository stores Telegram links in Postgres and one-time link codes in Redis.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Andas-LV/cargoflow/services/notification/internal/repository/db"
)

var ErrNotFound = errors.New("not found")

// TelegramLink connects a user to the Telegram chat that receives their notifications.
type TelegramLink = db.TelegramLink

type Links struct {
	q *db.Queries
}

func NewLinks(pool *pgxpool.Pool) *Links {
	return &Links{q: db.New(pool)}
}

func (r *Links) Save(ctx context.Context, userID uuid.UUID, chatID int64) error {
	return r.q.UpsertTelegramLink(ctx, db.UpsertTelegramLinkParams{UserID: userID, ChatID: chatID})
}

func (r *Links) Get(ctx context.Context, userID uuid.UUID) (TelegramLink, error) {
	l, err := r.q.GetTelegramLink(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TelegramLink{}, ErrNotFound
	}
	return l, err
}

func (r *Links) Delete(ctx context.Context, userID uuid.UUID) error {
	return r.q.DeleteTelegramLink(ctx, userID)
}

// DeleteByChat removes every link pointing to chatID and reports how many were removed.
func (r *Links) DeleteByChat(ctx context.Context, chatID int64) (int64, error) {
	return r.q.DeleteTelegramLinksByChat(ctx, chatID)
}
