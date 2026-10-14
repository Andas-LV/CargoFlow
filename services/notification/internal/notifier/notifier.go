// Package notifier delivers notifications to Telegram.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Message is a notification to deliver.
type Message struct {
	Title string
	Text  string
}

// Notifier sends a message somewhere. The consumer depends on this interface, so tests or
// other channels can be plugged in without touching it.
type Notifier interface {
	Notify(ctx context.Context, msg Message) error
}

// Telegram sends notifications via the Telegram Bot API (sendMessage) to one chat.
type Telegram struct {
	botToken, chatID string
	client           *http.Client
	log              *slog.Logger
}

func NewTelegram(botToken, chatID string, log *slog.Logger) Telegram {
	return Telegram{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 5 * time.Second}, // never wait for Telegram forever
		log:      log,
	}
}

func (t Telegram) Notify(ctx context.Context, msg Message) error {
	if t.botToken == "" || t.chatID == "" {
		t.log.DebugContext(ctx, "telegram not configured, notification skipped", "title", msg.Title)
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"chat_id": t.chatID,
		"text":    msg.Title + "\n" + msg.Text,
	})
	if err != nil {
		return err
	}

	endpoint := "https://api.telegram.org/bot" + t.botToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: build request") // the URL contains the token: never put it in an error
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// *url.Error prints the full URL, i.e. the bot token. Keep only the cause.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Telegram explains failures in the body: {"ok":false,"description":"Bad Request: chat not found"}.
		var apiErr struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&apiErr)
		return fmt.Errorf("telegram: status %d: %s", resp.StatusCode, apiErr.Description)
	}

	t.log.InfoContext(ctx, "telegram notification sent", "title", msg.Title)
	return nil
}
