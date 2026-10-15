// Package bot receives messages sent to the Telegram bot (long polling) and answers
// /start <code> and /stop. Long polling needs no public URL, so it also works locally.
package bot

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Andas-LV/cargoflow/services/notification/internal/service"
	"github.com/Andas-LV/cargoflow/services/notification/internal/telegram"
)

const (
	pollTimeout = 30 * time.Second
	retryDelay  = 5 * time.Second
)

type Bot struct {
	api *telegram.Client
	svc *service.Service
	log *slog.Logger
}

func New(api *telegram.Client, svc *service.Service, log *slog.Logger) *Bot {
	return &Bot{api: api, svc: svc, log: log}
}

// Run polls for updates until ctx is cancelled. Only one instance may poll a bot at a time:
// a second one gets 409 Conflict from Telegram.
func (b *Bot) Run(ctx context.Context) error {
	var offset int64
	for {
		updates, err := b.api.GetUpdates(ctx, offset, pollTimeout)
		if ctx.Err() != nil {
			return nil //nolint:nilerr // shutdown cancelled the long poll: a clean stop, not a failure
		}
		if err != nil {
			var apiErr *telegram.APIError
			if errors.As(err, &apiErr) && apiErr.Code == http.StatusConflict {
				b.log.Error("telegram polling conflict: another bot instance or a webhook is active", "err", err)
			} else {
				b.log.Warn("telegram getUpdates", "err", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(retryDelay):
			}
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1 // the next call confirms this update to Telegram
			b.handle(ctx, u)
		}
	}
}

func (b *Bot) handle(ctx context.Context, u telegram.Update) {
	m := u.Message
	if m == nil || m.Chat.Type != "private" {
		return
	}

	var reply string
	var err error
	switch cmd, arg := parseCommand(m.Text); cmd {
	case "/start":
		reply, err = b.svc.HandleStart(ctx, m.Chat.ID, arg)
	case "/stop":
		reply, err = b.svc.HandleStop(ctx, m.Chat.ID)
	default:
		reply = b.svc.Help()
	}
	if err != nil {
		b.log.ErrorContext(ctx, "handle bot command", "err", err)
	}
	if err := b.api.SendMessage(ctx, m.Chat.ID, reply); err != nil {
		b.log.WarnContext(ctx, "telegram reply", "err", err)
	}
}

// parseCommand splits "/start abc" into ("/start", "abc"). "/start@cargoflow_bot abc" works too.
func parseCommand(text string) (cmd, arg string) {
	cmd, arg, _ = strings.Cut(strings.TrimSpace(text), " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(arg)
}
