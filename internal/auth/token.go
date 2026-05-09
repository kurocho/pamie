// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	tokenSecretBytes = 32
	tokenSaltBytes   = 16
	tokenPrefix      = "pamie_"
)

// StoredToken is a persisted Bearer token record without the raw token secret.
type StoredToken struct {
	ID         string
	TokenHash  string
	TokenSalt  string
	Scopes     ScopeSet
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	ExpiresAt  *time.Time
}

// TokenSource loads active stored tokens for dynamic authentication.
type TokenSource interface {
	ActiveTokens(ctx context.Context, now time.Time) ([]StoredToken, error)
	RecordTokenUsed(ctx context.Context, tokenID string, usedAt time.Time) error
}

// GenerateBearerToken creates a high-entropy token that is shown once to the operator.
func GenerateBearerToken() (string, error) {
	var raw [tokenSecretBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// NewGeneratedStoredToken returns a raw token secret and the hash-only record to persist.
func NewGeneratedStoredToken(id string, scopes ScopeSet, now time.Time) (string, StoredToken, error) {
	secret, err := GenerateBearerToken()
	if err != nil {
		return "", StoredToken{}, err
	}
	stored, err := NewStoredToken(id, secret, scopes, now)
	if err != nil {
		return "", StoredToken{}, err
	}
	return secret, stored, nil
}

// NewStoredToken hashes a raw token for durable storage.
func NewStoredToken(id, secret string, scopes ScopeSet, now time.Time) (StoredToken, error) {
	if err := ValidateTokenID(id); err != nil {
		return StoredToken{}, err
	}
	if err := ValidateBearerSecret(secret); err != nil {
		return StoredToken{}, err
	}
	if scopes == nil {
		scopes = AllScopes()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	salt, err := GenerateTokenSalt()
	if err != nil {
		return StoredToken{}, err
	}
	return StoredToken{
		ID:        id,
		TokenSalt: salt,
		TokenHash: HashBearerToken(secret, salt),
		Scopes:    scopes,
		CreatedAt: now.UTC(),
	}, nil
}

// GenerateTokenSalt creates a per-token random salt for hash storage.
func GenerateTokenSalt() (string, error) {
	var raw [tokenSaltBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// HashBearerToken hashes a Bearer token with its persisted salt.
func HashBearerToken(secret, salt string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + secret))
	return hex.EncodeToString(sum[:])
}

// VerifyBearerTokenHash compares a raw token with a stored salted hash.
func VerifyBearerTokenHash(secret, salt, hash string) bool {
	expected := HashBearerToken(secret, salt)
	normalizedHash := strings.ToLower(strings.TrimSpace(hash))
	if len(normalizedHash) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(normalizedHash)) == 1
}

// ValidateBearerSecret checks that a token can be carried safely in an Authorization header.
func ValidateBearerSecret(secret string) error {
	if secret == "" {
		return ErrMissingToken
	}
	if strings.TrimSpace(secret) != secret || strings.ContainsAny(secret, " \t\r\n") {
		return ErrMalformedAuth
	}
	return nil
}

// ValidateTokenID checks the non-secret operator-facing token identifier.
func ValidateTokenID(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("token id must not be empty")
	}
	if strings.TrimSpace(id) != id || strings.ContainsAny(id, " \t\r\n") {
		return errors.New("token id must not contain whitespace")
	}
	return nil
}
