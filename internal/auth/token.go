package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const AccessTokenTTL = 15 * time.Minute

type Tokens struct {
	key []byte
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
