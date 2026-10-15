// Package notifier delivers notifications to the Telegram chat of the shipment owner.
package notifier

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Andas-LV/cargoflow/services/notification/internal/telegram"
)

// Message is a notification for one user.
type Message struct {
	// UserID is the recipient (shipment owner).
	UserID string
	Title  string
	Text   string
}

// Notifier sends a message somewhere. The consumer depends on this interface, so tests or
// other channels can be plugged in without touching it.
type Notifier interface {
	Notify(ctx context.Context, msg Message) error
}

// ChatResolver finds the Telegram chat linked to a user.
type ChatResolver interface {
	ChatID(ctx context.Context, userID string) (int64, bool, error)
}

// Telegram sends each message to the chat the recipient linked via the bot.
type Telegram struct {
	api   *telegram.Client
	chats ChatResolver
	log   *slog.Logger
}

func NewTelegram(api *telegram.Client, chats ChatResolver, log *slog.Logger) Telegram {
	return Telegram{api: api, chats: chats, log: log}
}

func (t Telegram) Notify(ctx context.Context, msg Message) error {
	if !t.api.Configured() {
		t.log.DebugContext(ctx, "telegram not configured, notification skipped", "title", msg.Title)
		return nil
	}

	chatID, ok, err := t.chats.ChatID(ctx, msg.UserID)
	if err != nil {
		return err // DB problem: retry later
	}
	if !ok {
		t.log.DebugContext(ctx, "user has no telegram linked, notification skipped", "user_id", msg.UserID, "title", msg.Title)
		return nil
	}

	err = t.api.SendMessage(ctx, chatID, msg.Title+"\n"+msg.Text)
	var apiErr *telegram.APIError
	if errors.As(err, &apiErr) && (apiErr.Code == http.StatusForbidden || apiErr.Code == http.StatusBadRequest) {
		// Blocked bot, deleted chat, etc.: retrying will not help, so drop the message.
		t.log.WarnContext(ctx, "telegram rejected notification", "user_id", msg.UserID, "err", err)
		return nil
	}
	if err != nil {
		return err // network or Telegram outage: the event will be redelivered
	}

	t.log.InfoContext(ctx, "telegram notification sent", "user_id", msg.UserID, "title", msg.Title)
	return nil
}
