package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Andas-LV/cargoflow/services/notification/internal/repository"
)

type fakeLinks map[uuid.UUID]repository.TelegramLink

func (f fakeLinks) Save(_ context.Context, userID uuid.UUID, chatID int64) error {
	f[userID] = repository.TelegramLink{UserID: userID, ChatID: chatID, LinkedAt: time.Now()}
	return nil
}

func (f fakeLinks) Get(_ context.Context, userID uuid.UUID) (repository.TelegramLink, error) {
	l, ok := f[userID]
	if !ok {
		return repository.TelegramLink{}, repository.ErrNotFound
	}
	return l, nil
}

func (f fakeLinks) Delete(_ context.Context, userID uuid.UUID) error {
	delete(f, userID)
	return nil
}

func (f fakeLinks) DeleteByChat(_ context.Context, chatID int64) (int64, error) {
	var n int64
	for id, l := range f {
		if l.ChatID == chatID {
			delete(f, id)
			n++
		}
	}
	return n, nil
}

type fakeCodes map[string]string

func (f fakeCodes) Save(_ context.Context, code, userID string, _ time.Duration) error {
	f[code] = userID
	return nil
}

func (f fakeCodes) Consume(_ context.Context, code string) (string, error) {
	userID, ok := f[code]
	if !ok {
		return "", repository.ErrNotFound
	}
	delete(f, code)
	return userID, nil
}

func TestTelegramLinkFlow(t *testing.T) {
	ctx := context.Background()
	links := fakeLinks{}
	svc := New(links, fakeCodes{}, "cargoflow_bot")
	user := uuid.New()
	const chat int64 = 683045561

	// 1. The app asks for a deep link.
	url, expiresAt, err := svc.CreateTelegramLink(ctx, user)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(url, "https://t.me/cargoflow_bot?start="))
	assert.WithinDuration(t, time.Now().Add(linkCodeTTL), expiresAt, time.Second)
	code := strings.TrimPrefix(url, "https://t.me/cargoflow_bot?start=")

	// Not linked until the user presses Start.
	_, ok, err := svc.ChatID(ctx, user.String())
	require.NoError(t, err)
	assert.False(t, ok)

	// 2. The user presses Start: the bot receives "/start <code>".
	reply, err := svc.HandleStart(ctx, chat, code)
	require.NoError(t, err)
	assert.Equal(t, replyLinked, reply)

	gotChat, ok, err := svc.ChatID(ctx, user.String())
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, chat, gotChat)

	// The code is one-time.
	reply, err = svc.HandleStart(ctx, 111, code)
	require.NoError(t, err)
	assert.Equal(t, replyBadCode, reply)

	// 3. /stop unlinks.
	reply, err = svc.HandleStop(ctx, chat)
	require.NoError(t, err)
	assert.Equal(t, replyUnlinked, reply)
	_, ok, _ = svc.ChatID(ctx, user.String())
	assert.False(t, ok)
}

func TestCreateTelegramLink_Disabled(t *testing.T) {
	_, _, err := New(fakeLinks{}, fakeCodes{}, "").CreateTelegramLink(context.Background(), uuid.New())
	assert.ErrorIs(t, err, ErrTelegramDisabled)
}

func TestChatID_EventWithoutOwner(t *testing.T) {
	_, ok, err := New(fakeLinks{}, fakeCodes{}, "bot").ChatID(context.Background(), "")
	require.NoError(t, err)
	assert.False(t, ok)
}
