package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const AccessTokenTTL = 15 * time.Minute

type Tokens struct {
	key        []byte
	userExists func(context.Context, string) (bool, error)
}

func NewTokens(secret string) (*Tokens, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &Tokens{key: []byte(secret)}, nil
}

func (t *Tokens) Issue(userID string) (string, error) {
	var id pgtype.UUID
	if err := id.Scan(userID); err != nil || !id.Valid {
		return "", fmt.Errorf("invalid user ID")
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer: "pulsechat", Audience: jwt.ClaimStrings{"pulsechat"}, Subject: userID,
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.key)
}

func (t *Tokens) Validate(raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return t.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(),
		jwt.WithIssuer("pulsechat"), jwt.WithAudience("pulsechat"), jwt.WithIssuedAt())
	if err != nil {
		return "", fmt.Errorf("validate access token: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(claims.Subject); err != nil || !id.Valid {
		return "", fmt.Errorf("invalid token subject")
	}
	return claims.Subject, nil
}

// WithUserLookup must be configured before serving requests. It revokes access
// to deleted accounts across API instances without keeping token state in memory.
func (t *Tokens) WithUserLookup(lookup func(context.Context, string) (bool, error)) *Tokens {
	t.userExists = lookup
	return t
}
func (t *Tokens) ValidateContext(ctx context.Context, raw string) (string, error) {
	id, err := t.Validate(raw)
	if err != nil {
		return "", err
	}
	if t.userExists != nil {
		ok, err := t.userExists(ctx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("account no longer exists")
		}
	}
	return id, nil
}
