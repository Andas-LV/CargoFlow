package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTokens() *Tokens {
	return NewTokens("access-secret", "refresh-secret", 15*time.Minute, time.Hour)
}

func TestTokens_IssueAndParse(t *testing.T) {
	tokens := newTestTokens()

	pair, refreshID, err := tokens.Issue("user-1")
	require.NoError(t, err)
	assert.Equal(t, 15*time.Minute, pair.ExpiresIn)

	userID, err := tokens.ParseAccess(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)

	userID, tokenID, err := tokens.ParseRefresh(pair.RefreshToken)
	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.Equal(t, refreshID, tokenID)
}

func TestTokens_RejectsSwappedTokenTypes(t *testing.T) {
	tokens := newTestTokens()
	pair, _, err := tokens.Issue("user-1")
	require.NoError(t, err)

	_, err = tokens.ParseAccess(pair.RefreshToken)
	assert.ErrorIs(t, err, ErrInvalidToken)

	_, _, err = tokens.ParseRefresh(pair.AccessToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokens_RejectsExpired(t *testing.T) {
	tokens := newTestTokens()
	issuedAt := time.Now()
	tokens.now = func() time.Time { return issuedAt }
	pair, _, err := tokens.Issue("user-1")
	require.NoError(t, err)

	tokens.now = func() time.Time { return issuedAt.Add(16 * time.Minute) }
	_, err = tokens.ParseAccess(pair.AccessToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokens_RejectsForeignSecret(t *testing.T) {
	pair, _, err := NewTokens("other", "other", time.Minute, time.Minute).Issue("user-1")
	require.NoError(t, err)

	_, err = newTestTokens().ParseAccess(pair.AccessToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
}
