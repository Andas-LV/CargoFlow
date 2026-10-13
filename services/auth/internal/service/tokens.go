package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	issuer          = "cargoflow-auth"
	audienceAccess  = "access"
	audienceRefresh = "refresh"
)

// Tokens issues and parses HS256 JWTs. Access and refresh tokens use different secrets
// and audiences, so one can never be used in place of the other.
type Tokens struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
	now           func() time.Time
}

func NewTokens(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Tokens {
	return &Tokens{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		now:           time.Now,
	}
}

// TokenPair is returned to clients after login or refresh.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

// Issue creates a token pair for userID. refreshID (the refresh token's jti) is what the
// session store keys on.
func (t *Tokens) Issue(userID string) (pair TokenPair, refreshID string, err error) {
	access, _, err := t.sign(userID, audienceAccess, t.accessTTL, t.accessSecret)
	if err != nil {
		return TokenPair{}, "", err
	}
	refresh, refreshID, err := t.sign(userID, audienceRefresh, t.refreshTTL, t.refreshSecret)
	if err != nil {
		return TokenPair{}, "", err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: t.accessTTL}, refreshID, nil
}

// ParseAccess validates an access token and returns its subject (user ID).
func (t *Tokens) ParseAccess(token string) (string, error) {
	claims, err := t.parse(token, audienceAccess, t.accessSecret)
	if err != nil {
		return "", err
	}
	return claims.Subject, nil
}

// ParseRefresh validates a refresh token and returns its subject and token ID.
func (t *Tokens) ParseRefresh(token string) (userID, tokenID string, err error) {
	claims, err := t.parse(token, audienceRefresh, t.refreshSecret)
	if err != nil {
		return "", "", err
	}
	return claims.Subject, claims.ID, nil
}

func (t *Tokens) sign(userID, audience string, ttl time.Duration, secret []byte) (string, string, error) {
	now := t.now()
	id := uuid.NewString()
	claims := jwt.RegisteredClaims{
		ID:        id,
		Issuer:    issuer,
		Subject:   userID,
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", "", fmt.Errorf("sign %s token: %w", audience, err)
	}
	return signed, id, nil
}

func (t *Tokens) parse(token, audience string, secret []byte) (*jwt.RegisteredClaims, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: empty subject", ErrInvalidToken)
	}
	return claims, nil
}
