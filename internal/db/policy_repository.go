// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"database/sql"
	"fmt"
)

// RetentionPolicyRepository stores retention policies.
type RetentionPolicyRepository struct {
	exec executor
}

// Create inserts a retention policy.
func (r *RetentionPolicyRepository) Create(ctx context.Context, policy RetentionPolicy) error {
	if err := validateRetentionPolicy(policy); err != nil {
		return err
	}

	_, err := r.exec.ExecContext(ctx, `
INSERT INTO retention_policies (id, name, scope_json, rules_json, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		policy.ID,
		policy.Name,
		defaultJSON(policy.ScopeJSON),
		defaultJSON(policy.RulesJSON),
		boolInt(policy.Enabled),
		formatTime(policy.CreatedAt),
		formatTime(policy.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create retention policy: %w", err)
	}
	return nil
}

// ListEnabled returns enabled retention policies in stable order.
func (r *RetentionPolicyRepository) ListEnabled(ctx context.Context) ([]RetentionPolicy, error) {
	rows, err := r.exec.QueryContext(ctx, `
SELECT id, name, scope_json, rules_json, enabled, created_at, updated_at
FROM retention_policies
WHERE enabled = 1
ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list retention policies: %w", err)
	}
	defer rows.Close()

	var policies []RetentionPolicy
	for rows.Next() {
		policy, err := scanRetentionPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list retention policies: %w", err)
	}
	return policies, nil
}

func scanRetentionPolicy(row scanner) (RetentionPolicy, error) {
	var policy RetentionPolicy
	var enabled int
	var createdAt, updatedAt string
	if err := row.Scan(
		&policy.ID,
		&policy.Name,
		&policy.ScopeJSON,
		&policy.RulesJSON,
		&enabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return RetentionPolicy{}, ErrNotFound
		}
		return RetentionPolicy{}, fmt.Errorf("scan retention policy: %w", err)
	}
	var err error
	policy.Enabled = intBool(enabled)
	if policy.CreatedAt, err = parseTime(createdAt); err != nil {
		return RetentionPolicy{}, fmt.Errorf("parse policy created_at: %w", err)
	}
	if policy.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return RetentionPolicy{}, fmt.Errorf("parse policy updated_at: %w", err)
	}
	return policy, nil
}

func validateRetentionPolicy(policy RetentionPolicy) error {
	if err := requireID("retention policy", policy.ID); err != nil {
		return err
	}
	if policy.Name == "" {
		return fmt.Errorf("%w: retention policy name must not be empty", ErrInvalid)
	}
	if policy.CreatedAt.IsZero() {
		return fmt.Errorf("%w: retention policy created_at must not be zero", ErrInvalid)
	}
	if policy.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: retention policy updated_at must not be zero", ErrInvalid)
	}
	return nil
}
