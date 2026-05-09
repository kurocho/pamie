// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"time"

	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/db"
)

type databaseTokenSource struct {
	repo *db.TokenRepository
}

func (s databaseTokenSource) ActiveTokens(ctx context.Context, now time.Time) ([]auth.StoredToken, error) {
	records, err := s.repo.ListActive(ctx, now)
	if err != nil {
		return nil, err
	}
	tokens := make([]auth.StoredToken, 0, len(records))
	for _, record := range records {
		scopes, err := auth.ParseScopes(record.Scopes)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, auth.StoredToken{
			ID:         record.ID,
			TokenHash:  record.TokenHash,
			TokenSalt:  record.TokenSalt,
			Scopes:     scopes,
			CreatedAt:  record.CreatedAt,
			LastUsedAt: record.LastUsedAt,
			RevokedAt:  record.RevokedAt,
			ExpiresAt:  record.ExpiresAt,
		})
	}
	return tokens, nil
}

func (s databaseTokenSource) RecordTokenUsed(ctx context.Context, tokenID string, usedAt time.Time) error {
	return s.repo.Touch(ctx, tokenID, usedAt)
}
