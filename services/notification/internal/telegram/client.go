// Package telegram is a minimal Telegram Bot API client: getMe, sendMessage, getUpdates.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// APIError is an error reported by Telegram, e.g. 403 "Forbidden: bot was blocked by the user".
type APIError struct {
	Code        int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// Client talks to the Bot API. Errors never contain the bot token.
type Client struct {
	token string
	http  *http.Client
}

// New returns a client. getUpdates long-polls, so the HTTP timeout must exceed its poll timeout.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 60 * time.Second}}
}

// Configured reports whether a bot token is set.
func (c *Client) Configured() bool { return c.token != "" }

// User is the bot's own account (getMe).
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// GetMe returns the bot account; its Username is needed for t.me deep links.
func (c *Client) GetMe(ctx context.Context) (User, error) {
	var u User
	err := c.call(ctx, "getMe", struct{}{}, &u)
	return u, err
}

// SendMessage sends plain text to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text}, nil)
}

// Update is an incoming event for the bot. Only private text messages are used.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	Text string `json:"text"`
	Chat Chat   `json:"chat"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private", "group", ...
}

// GetUpdates long-polls for new updates starting at offset (last update_id + 1).
// Passing a higher offset tells Telegram the earlier updates were processed.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": []string{"message"},
	}, &updates)
	return updates, err
}

// call POSTs params as JSON to method and decodes the "result" field into out (may be nil).
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s: build request", method) // the URL contains the token
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error prints the full URL, i.e. the bot token. Keep only the cause.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("telegram %s: decode response (status %d): %w", method, resp.StatusCode, err)
	}
	if !envelope.OK {
		return &APIError{Code: envelope.ErrorCode, Description: envelope.Description}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
