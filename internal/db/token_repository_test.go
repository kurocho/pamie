// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/pamie/internal/auth"
)

func TestTokenRepositoryUpsertListActiveAndTouch(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Options{Path: filepath.Join(t.TempDir(), "pamie.db")})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	secret, stored, err := auth.NewGeneratedStoredToken("agent-a", auth.AllScopes(), now)
	if err != nil {
		t.Fatalf("NewGeneratedStoredToken() error = %v", err)
	}
	record := AuthToken{
		ID:        stored.ID,
		TokenHash: stored.TokenHash,
		TokenSalt: stored.TokenSalt,
		Scopes:    "all",
		CreatedAt: stored.CreatedAt,
	}
	if err := store.Tokens().Upsert(ctx, record); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	active, err := store.Tokens().ListActive(ctx, now)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(active) != 1 || active[0].ID != "agent-a" {
		t.Fatalf("active tokens = %+v, want agent-a", active)
	}
	if !auth.VerifyBearerTokenHash(secret, active[0].TokenSalt, active[0].TokenHash) {
		t.Fatal("stored token hash does not verify generated secret")
	}

	usedAt := now.Add(time.Minute)
	if err := store.Tokens().Touch(ctx, "agent-a", usedAt); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	listed, err := store.Tokens().List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if listed[0].LastUsedAt == nil || !listed[0].LastUsedAt.Equal(usedAt) {
		t.Fatalf("LastUsedAt = %v, want %v", listed[0].LastUsedAt, usedAt)
	}
}

func TestTokenRepositoryRevokeAndExpiration(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Options{Path: filepath.Join(t.TempDir(), "pamie.db")})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	_, stored, err := auth.NewGeneratedStoredToken("agent-a", auth.AllScopes(), now)
	if err != nil {
		t.Fatalf("NewGeneratedStoredToken() error = %v", err)
	}
	expiresAt := now.Add(-time.Minute)
	if err := store.Tokens().Upsert(ctx, AuthToken{
		ID:        stored.ID,
		TokenHash: stored.TokenHash,
		TokenSalt: stored.TokenSalt,
		Scopes:    "all",
		CreatedAt: stored.CreatedAt,
		ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	active, err := store.Tokens().ListActive(ctx, now)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active tokens = %+v, want none for expired token", active)
	}

	_, stored, err = auth.NewGeneratedStoredToken("agent-a", auth.AllScopes(), now)
	if err != nil {
		t.Fatalf("NewGeneratedStoredToken() error = %v", err)
	}
	if err := store.Tokens().Upsert(ctx, AuthToken{
		ID:        stored.ID,
		TokenHash: stored.TokenHash,
		TokenSalt: stored.TokenSalt,
		Scopes:    "all",
		CreatedAt: stored.CreatedAt,
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if err := store.Tokens().Revoke(ctx, "agent-a", now); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	active, err = store.Tokens().ListActive(ctx, now)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active tokens = %+v, want none after revoke", active)
	}
}
