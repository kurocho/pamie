// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/db"
)

func runTokenCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath string) error {
	if len(args) == 0 {
		return runTokenRotateCommand(ctx, []string{}, stdout, stderr, defaultDBPath)
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, `Usage:
  pamie token
  pamie token rotate [--id ID] [--scopes SCOPES] [--expires-in DURATION]
  pamie token create --id ID [--scopes SCOPES] [--expires-in DURATION]
  pamie token list
  pamie token revoke --id ID

Subcommands:
  rotate  Regenerate a token and print the new secret once. Default: id=default.
  create  Create an additional token ID and print its secret once.
  list    List token metadata without secrets.
  revoke  Disable a token ID.

Running "pamie token" is the same as "pamie token rotate --id default".
`)
		return nil
	case "rotate":
		return runTokenRotateCommand(ctx, args[1:], stdout, stderr, defaultDBPath)
	case "create":
		return runTokenCreateCommand(ctx, args[1:], stdout, stderr, defaultDBPath)
	case "list":
		return runTokenListCommand(ctx, args[1:], stdout, stderr, defaultDBPath)
	case "revoke":
		return runTokenRevokeCommand(ctx, args[1:], stdout, stderr, defaultDBPath)
	default:
		return fmt.Errorf("token: unknown subcommand %q", args[0])
	}
}

func runTokenRotateCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath string) error {
	fs := flag.NewFlagSet("token rotate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaultDBPath, "SQLite database path")
	id := fs.String("id", "default", "token ID to rotate")
	scopes := fs.String("scopes", "all", "comma-separated scopes, or all")
	expiresIn := fs.Duration("expires-in", 0, "optional token lifetime; 0 means no expiration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("token rotate: unexpected arguments: %v", fs.Args())
	}
	if *expiresIn < 0 {
		return errors.New("token rotate: --expires-in must not be negative")
	}
	store, err := db.Open(ctx, db.Options{Path: *dbPath})
	if err != nil {
		return err
	}
	defer store.Close()

	secret, err := rotateStoredToken(ctx, store, *id, *scopes, *expiresIn)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	fmt.Fprintf(stdout, "token_id: %s\n", *id)
	fmt.Fprintf(stdout, "bearer_token: %s\n", secret)
	fmt.Fprintln(stdout, "shown_once: true")
	return nil
}

func runTokenCreateCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath string) error {
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaultDBPath, "SQLite database path")
	id := fs.String("id", "", "token ID to create")
	scopes := fs.String("scopes", "all", "comma-separated scopes, or all")
	expiresIn := fs.Duration("expires-in", 0, "optional token lifetime; 0 means no expiration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("token create: unexpected arguments: %v", fs.Args())
	}
	if strings.TrimSpace(*id) == "" {
		return errors.New("token create: --id is required")
	}
	if *expiresIn < 0 {
		return errors.New("token create: --expires-in must not be negative")
	}
	store, err := db.Open(ctx, db.Options{Path: *dbPath})
	if err != nil {
		return err
	}
	defer store.Close()

	existing, err := store.Tokens().List(ctx)
	if err != nil {
		return err
	}
	for _, token := range existing {
		if token.ID == *id {
			return fmt.Errorf("token create: token %q already exists; use token rotate", *id)
		}
	}
	secret, err := rotateStoredToken(ctx, store, *id, *scopes, *expiresIn)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	fmt.Fprintf(stdout, "token_id: %s\n", *id)
	fmt.Fprintf(stdout, "bearer_token: %s\n", secret)
	fmt.Fprintln(stdout, "shown_once: true")
	return nil
}

func runTokenListCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath string) error {
	fs := flag.NewFlagSet("token list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaultDBPath, "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("token list: unexpected arguments: %v", fs.Args())
	}
	store, err := db.Open(ctx, db.Options{Path: *dbPath})
	if err != nil {
		return err
	}
	defer store.Close()

	tokens, err := store.Tokens().List(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	if len(tokens) == 0 {
		fmt.Fprintln(stdout, "No tokens found. Run `pamie token` to create one.")
		return nil
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tSCOPES\tCREATED\tLAST_USED\tEXPIRES\tREVOKED")
	for _, token := range tokens {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			token.ID,
			token.Scopes,
			formatOptionalTime(&token.CreatedAt),
			formatOptionalTime(token.LastUsedAt),
			formatOptionalTime(token.ExpiresAt),
			formatOptionalTime(token.RevokedAt),
		)
	}
	return writer.Flush()
}

func runTokenRevokeCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath string) error {
	fs := flag.NewFlagSet("token revoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaultDBPath, "SQLite database path")
	id := fs.String("id", "", "token ID to revoke")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("token revoke: unexpected arguments: %v", fs.Args())
	}
	if strings.TrimSpace(*id) == "" {
		return errors.New("token revoke: --id is required")
	}
	store, err := db.Open(ctx, db.Options{Path: *dbPath})
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Tokens().Revoke(ctx, *id, time.Now().UTC()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "database: %s\n", *dbPath)
	fmt.Fprintf(stdout, "revoked token_id: %s\n", *id)
	return nil
}

func rotateStoredToken(ctx context.Context, store *db.Store, id, scopeText string, expiresIn time.Duration) (string, error) {
	scopes, err := auth.ParseScopes(scopeText)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	secret, stored, err := auth.NewGeneratedStoredToken(id, scopes, now)
	if err != nil {
		return "", err
	}
	var expiresAt *time.Time
	if expiresIn > 0 {
		expires := now.Add(expiresIn).UTC()
		expiresAt = &expires
	}
	record := db.AuthToken{
		ID:        stored.ID,
		TokenHash: stored.TokenHash,
		TokenSalt: stored.TokenSalt,
		Scopes:    normalizeScopeText(scopeText),
		CreatedAt: stored.CreatedAt,
		ExpiresAt: expiresAt,
	}
	if err := store.Tokens().Upsert(ctx, record); err != nil {
		return "", err
	}
	return secret, nil
}

func normalizeScopeText(scopes string) string {
	scopes = strings.TrimSpace(scopes)
	if scopes == "" {
		return "all"
	}
	return scopes
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}
