// Package notifier delivers notifications. Telegram and email are stubs that only log
// what would be sent; replace their Notify bodies with real API calls later.
package notifier

import (
	"context"
	"errors"
	"log/slog"
)

// Message is a channel-agnostic notification.
type Message struct {
	// UserID is the recipient (shipment owner); empty when unknown.
	UserID string
	Title  string
	Text   string
}

type Notifier interface {
	Notify(ctx context.Context, msg Message) error
}

// Multi fans a message out to several notifiers and joins their errors.
type Multi []Notifier

func (m Multi) Notify(ctx context.Context, msg Message) error {
	var errs []error
	for _, n := range m {
		if err := n.Notify(ctx, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Log writes every notification to the service log. Always enabled, so the event flow is
// visible locally without any external accounts.
type Log struct{ log *slog.Logger }

func NewLog(log *slog.Logger) Log { return Log{log: log} }

func (l Log) Notify(ctx context.Context, msg Message) error {
	l.log.InfoContext(ctx, "notification", "user_id", msg.UserID, "title", msg.Title, "text", msg.Text)
	return nil
}

// Telegram is a stub for the Telegram Bot API (sendMessage).
type Telegram struct {
	botToken, chatID string
	log              *slog.Logger
}

func NewTelegram(botToken, chatID string, log *slog.Logger) Telegram {
	return Telegram{botToken: botToken, chatID: chatID, log: log}
}

func (t Telegram) Notify(ctx context.Context, msg Message) error {
	if t.botToken == "" || t.chatID == "" {
		return nil // not configured
	}
	// TODO: POST https://api.telegram.org/bot<token>/sendMessage {chat_id, text}.
	t.log.DebugContext(ctx, "telegram stub: would send", "chat_id", t.chatID, "title", msg.Title)
	return nil
}

// Email is a stub for SMTP delivery.
type Email struct {
	host string
	port int
	log  *slog.Logger
}

func NewEmail(host string, port int, log *slog.Logger) Email {
	return Email{host: host, port: port, log: log}
}

func (e Email) Notify(ctx context.Context, msg Message) error {
	if e.host == "" {
		return nil // not configured
	}
	// TODO: look up the user's email and send via net/smtp.
	e.log.DebugContext(ctx, "email stub: would send", "smtp_host", e.host, "title", msg.Title)
	return nil
}
