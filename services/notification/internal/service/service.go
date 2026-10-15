// Package service links CargoFlow users to Telegram chats via one-time deep links.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Andas-LV/cargoflow/services/notification/internal/repository"
)

// ErrTelegramDisabled means TELEGRAM_BOT_TOKEN is not set.
var ErrTelegramDisabled = errors.New("telegram notifications are not configured")

const linkCodeTTL = 10 * time.Minute

// Bot replies shown to the user in Telegram.
const (
	replyLinked       = "Уведомления CargoFlow подключены ✅\nОтключить: /stop"
	replyBadCode      = "Ссылка недействительна или устарела. Создайте новую в приложении CargoFlow."
	replyNoCode       = "Чтобы подключить уведомления, нажмите «Подключить Telegram» в приложении CargoFlow."
	replyUnlinked     = "Уведомления CargoFlow отключены. Подключить снова можно в приложении."
	replyNotLinked    = "Уведомления и так не подключены."
	replyHelp         = "Я присылаю уведомления о ваших заявках CargoFlow.\nПодключение — в приложении, отключение — /stop."
	replyInternalFail = "Что-то пошло не так, попробуйте позже."
)

type LinkStore interface {
	Save(ctx context.Context, userID uuid.UUID, chatID int64) error
	Get(ctx context.Context, userID uuid.UUID) (repository.TelegramLink, error)
	Delete(ctx context.Context, userID uuid.UUID) error
	DeleteByChat(ctx context.Context, chatID int64) (int64, error)
}

type CodeStore interface {
	Save(ctx context.Context, code, userID string, ttl time.Duration) error
	Consume(ctx context.Context, code string) (string, error)
}

type Service struct {
	links       LinkStore
	codes       CodeStore
	botUsername string // empty when Telegram is not configured
	now         func() time.Time
}

func New(links LinkStore, codes CodeStore, botUsername string) *Service {
	return &Service{links: links, codes: codes, botUsername: botUsername, now: time.Now}
}

// CreateTelegramLink issues a one-time deep link for userID.
func (s *Service) CreateTelegramLink(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	if s.botUsername == "" {
		return "", time.Time{}, ErrTelegramDisabled
	}
	code, err := newCode()
	if err != nil {
		return "", time.Time{}, err
	}
	if err := s.codes.Save(ctx, code, userID.String(), linkCodeTTL); err != nil {
		return "", time.Time{}, fmt.Errorf("save link code: %w", err)
	}
	return "https://t.me/" + s.botUsername + "?start=" + code, s.now().Add(linkCodeTTL), nil
}

// TelegramStatus returns the user's link, or linked=false.
func (s *Service) TelegramStatus(ctx context.Context, userID uuid.UUID) (repository.TelegramLink, bool, error) {
	link, err := s.links.Get(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.TelegramLink{}, false, nil
	}
	if err != nil {
		return repository.TelegramLink{}, false, err
	}
	return link, true, nil
}

func (s *Service) UnlinkTelegram(ctx context.Context, userID uuid.UUID) error {
	return s.links.Delete(ctx, userID)
}

// ChatID returns the Telegram chat of a user (as a string ID from an event), or ok=false.
func (s *Service) ChatID(ctx context.Context, userID string) (int64, bool, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return 0, false, nil //nolint:nilerr // events without a valid owner cannot be routed: skip, do not retry
	}
	link, ok, err := s.TelegramStatus(ctx, id)
	return link.ChatID, ok, err
}

// HandleStart processes "/start <code>" sent to the bot and returns the reply text.
func (s *Service) HandleStart(ctx context.Context, chatID int64, code string) (string, error) {
	if code == "" {
		return replyNoCode, nil
	}
	rawUserID, err := s.codes.Consume(ctx, code)
	if errors.Is(err, repository.ErrNotFound) {
		return replyBadCode, nil
	}
	if err != nil {
		return replyInternalFail, err
	}
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		return replyInternalFail, fmt.Errorf("bad user id in link code: %w", err)
	}
	if err := s.links.Save(ctx, userID, chatID); err != nil {
		return replyInternalFail, err
	}
	return replyLinked, nil
}

// HandleStop processes "/stop": every account linked to this chat stops receiving notifications.
func (s *Service) HandleStop(ctx context.Context, chatID int64) (string, error) {
	n, err := s.links.DeleteByChat(ctx, chatID)
	if err != nil {
		return replyInternalFail, err
	}
	if n == 0 {
		return replyNotLinked, nil
	}
	return replyUnlinked, nil
}

// Help is the reply to any other message.
func (s *Service) Help() string { return replyHelp }

// newCode returns 16 URL-safe random characters, valid as a Telegram start parameter.
func newCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
