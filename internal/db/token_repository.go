// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// TokenRepository stores persistent Bearer token metadata.
type TokenRepository struct {
	exec executor
}

// Upsert creates or replaces a token record for the same token ID.
func (r *TokenRepository) Upsert(ctx context.Context, token AuthToken) error {
	if err := validateAuthToken(token); err != nil {
		return err
	}

	_, err := r.exec.ExecContext(ctx, `
	INSERT INTO auth_tokens (
	  id, token_hash, token_salt, scopes, created_at, last_used_at, revoked_at, expires_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
	  token_hash = excluded.token_hash,
	  token_salt = excluded.token_salt,
	  scopes = excluded.scopes,
	  created_at = excluded.created_at,
	  last_used_at = NULL,
	  revoked_at = NULL,
	  expires_at = excluded.expires_at`,
		token.ID,
		token.TokenHash,
		token.TokenSalt,
		token.Scopes,
		formatTime(token.CreatedAt),
		nullableTime(token.LastUsedAt),
		nullableTime(token.RevokedAt),
		nullableTime(token.ExpiresAt),
	)
	if err != nil {
		return fmt.Errorf("upsert auth token: %w", err)
	}
	return nil
}

// List returns token metadata ordered by creation time.
func (r *TokenRepository) List(ctx context.Context) ([]AuthToken, error) {
	rows, err := r.exec.QueryContext(ctx, `
	SELECT id, token_hash, token_salt, scopes, created_at, last_used_at, revoked_at, expires_at
	FROM auth_tokens
	ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list auth tokens: %w", err)
	}
	defer rows.Close()

	var tokens []AuthToken
	for rows.Next() {
		token, err := scanAuthToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list auth tokens: %w", err)
	}
	return tokens, nil
}

// ListActive returns tokens that can authenticate at now.
func (r *TokenRepository) ListActive(ctx context.Context, now time.Time) ([]AuthToken, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := r.exec.QueryContext(ctx, `
	SELECT id, token_hash, token_salt, scopes, created_at, last_used_at, revoked_at, expires_at
	FROM auth_tokens
	WHERE revoked_at IS NULL
	  AND (expires_at IS NULL OR expires_at > ?)
	ORDER BY created_at, id`, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("list active auth tokens: %w", err)
	}
	defer rows.Close()

	var tokens []AuthToken
	for rows.Next() {
		token, err := scanAuthToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active auth tokens: %w", err)
	}
	return tokens, nil
}

// CountActive counts tokens that can authenticate at now.
func (r *TokenRepository) CountActive(ctx context.Context, now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var count int
	if err := r.exec.QueryRowContext(ctx, `
	SELECT COUNT(*)
	FROM auth_tokens
	WHERE revoked_at IS NULL
	  AND (expires_at IS NULL OR expires_at > ?)`, formatTime(now)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active auth tokens: %w", err)
	}
	return count, nil
}

// Revoke marks a token disabled without deleting its metadata.
func (r *TokenRepository) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	if err := requireID("token", id); err != nil {
		return err
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	result, err := r.exec.ExecContext(ctx, `
	UPDATE auth_tokens
	SET revoked_at = ?
	WHERE id = ? AND revoked_at IS NULL`, formatTime(revokedAt), id)
	if err != nil {
		return fmt.Errorf("revoke auth token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke auth token rows: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Touch records a token's most recent successful use.
func (r *TokenRepository) Touch(ctx context.Context, id string, usedAt time.Time) error {
	if err := requireID("token", id); err != nil {
		return err
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	_, err := r.exec.ExecContext(ctx, `
	UPDATE auth_tokens
	SET last_used_at = ?
	WHERE id = ? AND revoked_at IS NULL`, formatTime(usedAt), id)
	if err != nil {
		return fmt.Errorf("touch auth token: %w", err)
	}
	return nil
}

type authTokenScanner interface {
	Scan(dest ...any) error
}

func scanAuthToken(scanner authTokenScanner) (AuthToken, error) {
	var token AuthToken
	var createdAt string
	var lastUsedAt, revokedAt, expiresAt sql.NullString
	if err := scanner.Scan(
		&token.ID,
		&token.TokenHash,
		&token.TokenSalt,
		&token.Scopes,
		&createdAt,
		&lastUsedAt,
		&revokedAt,
		&expiresAt,
	); err != nil {
		return AuthToken{}, fmt.Errorf("scan auth token: %w", err)
	}
	var err error
	if token.CreatedAt, err = parseTime(createdAt); err != nil {
		return AuthToken{}, fmt.Errorf("parse auth token created_at: %w", err)
	}
	if token.LastUsedAt, err = parseNullableTime(lastUsedAt); err != nil {
		return AuthToken{}, fmt.Errorf("parse auth token last_used_at: %w", err)
	}
	if token.RevokedAt, err = parseNullableTime(revokedAt); err != nil {
		return AuthToken{}, fmt.Errorf("parse auth token revoked_at: %w", err)
	}
	if token.ExpiresAt, err = parseNullableTime(expiresAt); err != nil {
		return AuthToken{}, fmt.Errorf("parse auth token expires_at: %w", err)
	}
	return token, nil
}

func validateAuthToken(token AuthToken) error {
	if err := requireID("token", token.ID); err != nil {
		return err
	}
	if token.TokenHash == "" {
		return fmt.Errorf("%w: token hash must not be empty", ErrInvalid)
	}
	if token.TokenSalt == "" {
		return fmt.Errorf("%w: token salt must not be empty", ErrInvalid)
	}
	if token.Scopes == "" {
		return fmt.Errorf("%w: token scopes must not be empty", ErrInvalid)
	}
	if token.CreatedAt.IsZero() {
		return fmt.Errorf("%w: token created_at must not be zero", ErrInvalid)
	}
	return nil
}
