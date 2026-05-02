// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/your-org/pamie/internal/db"
)

const (
	defaultLifecycleLimit = 500
	maxLifecycleLimit     = 5000
)

type LifecycleOptions struct {
	Limit int
}

type LifecycleReport struct {
	Evaluated        int               `json:"evaluated"`
	Promoted         int               `json:"promoted"`
	Demoted          int               `json:"demoted"`
	Archived         int               `json:"archived"`
	Deleted          int               `json:"deleted"`
	SkippedPinned    int               `json:"skipped_pinned"`
	SkippedImportant int               `json:"skipped_important"`
	Changes          []LifecycleChange `json:"changes"`
}

type LifecycleChange struct {
	MemoryID string `json:"memory_id"`
	Action   string `json:"action"`
	FromTier string `json:"from_tier,omitempty"`
	ToTier   string `json:"to_tier,omitempty"`
	Reason   string `json:"reason"`
	PolicyID string `json:"policy_id,omitempty"`
}

type lifecycleRules struct {
	DemoteWorkingAfter         time.Duration
	DemoteHotAfter             time.Duration
	DemoteWarmAfter            time.Duration
	ArchiveColdAfter           time.Duration
	DeleteArchivedAfter        time.Duration
	PromoteAccessCount         int
	PromoteAccessWindow        time.Duration
	ProtectPinned              bool
	ProtectImportanceAtOrAbove int
}

type policyRulesJSON struct {
	DemoteWorkingAfterDays     int   `json:"demote_working_after_days"`
	DemoteHotAfterDays         int   `json:"demote_hot_after_days"`
	DemoteWarmAfterDays        int   `json:"demote_warm_after_days"`
	ArchiveColdAfterDays       int   `json:"archive_cold_after_days"`
	DeleteArchivedAfterDays    int   `json:"delete_archived_after_days"`
	PromoteAccessCount         int   `json:"promote_access_count"`
	PromoteAccessWindowDays    int   `json:"promote_access_window_days"`
	ProtectPinned              *bool `json:"protect_pinned"`
	ProtectImportanceAtOrAbove int   `json:"protect_importance_at_or_above"`
}

type policyScope struct {
	Tiers         []string `json:"tiers"`
	Sources       []string `json:"sources"`
	IncludePinned *bool    `json:"include_pinned"`
}

type lifecyclePolicy struct {
	ID    string
	Name  string
	Scope policyScope
	Rules lifecycleRules
}

func defaultLifecycleRules() lifecycleRules {
	return lifecycleRules{
		DemoteWorkingAfter:         24 * time.Hour,
		DemoteHotAfter:             7 * 24 * time.Hour,
		DemoteWarmAfter:            30 * 24 * time.Hour,
		ArchiveColdAfter:           90 * 24 * time.Hour,
		DeleteArchivedAfter:        0,
		PromoteAccessCount:         3,
		PromoteAccessWindow:        7 * 24 * time.Hour,
		ProtectPinned:              true,
		ProtectImportanceAtOrAbove: 90,
	}
}

func (s *Service) RunLifecycle(ctx context.Context, opts LifecycleOptions) (LifecycleReport, error) {
	store, err := s.requireStore()
	if err != nil {
		return LifecycleReport{}, err
	}
	limit := normalizeLifecycleLimit(opts.Limit)
	now := s.now().UTC()
	var report LifecycleReport

	err = store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
		policies, err := loadLifecyclePolicies(ctx, tx.Policies())
		if err != nil {
			return err
		}
		items, err := tx.Memories().ListActive(ctx, db.LifecycleListOptions{Limit: limit})
		if err != nil {
			return mapDBError(err)
		}
		report.Evaluated = len(items)
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			change, skipped, err := s.evaluateLifecycleItem(ctx, tx, item, policies, now)
			if err != nil {
				return err
			}
			switch skipped {
			case "pinned":
				report.SkippedPinned++
			case "important":
				report.SkippedImportant++
			}
			if change == nil {
				continue
			}
			report.Changes = append(report.Changes, *change)
			switch change.Action {
			case "promoted":
				report.Promoted++
			case "demoted":
				report.Demoted++
			case "archived":
				report.Archived++
			case "deleted":
				report.Deleted++
			}
		}
		return nil
	})
	if err != nil {
		return LifecycleReport{}, err
	}
	return report, nil
}

func (s *Service) evaluateLifecycleItem(ctx context.Context, tx *db.Tx, item db.MemoryItem, policies []lifecyclePolicy, now time.Time) (*LifecycleChange, string, error) {
	policy := policyForItem(item, policies)
	rules := policy.Rules

	if change, err := s.promoteByAccess(ctx, tx, item, policy, now); err != nil || change != nil {
		return change, "", err
	}

	if rules.ProtectPinned && item.Pinned {
		if wouldChangeByAge(item, rules, now) {
			return nil, "pinned", nil
		}
		return nil, "", nil
	}
	if rules.ProtectImportanceAtOrAbove > 0 && item.Importance >= rules.ProtectImportanceAtOrAbove {
		if wouldChangeByAge(item, rules, now) {
			return nil, "important", nil
		}
		return nil, "", nil
	}

	if item.Tier == db.TierArchive && rules.DeleteArchivedAfter > 0 && item.ArchivedAt != nil && !now.Before(item.ArchivedAt.Add(rules.DeleteArchivedAfter)) {
		change, err := s.lifecycleDelete(ctx, tx, item, policy, now)
		return change, "", err
	}
	if item.Tier == db.TierCold && !now.Before(lastActivity(item).Add(rules.ArchiveColdAfter)) {
		change, err := s.lifecycleArchive(ctx, tx, item, policy, now)
		return change, "", err
	}
	if next, ok := demotionTarget(item, rules, now); ok {
		change, err := s.lifecycleTierChange(ctx, tx, item, next, "demoted", "inactive memory aged into lower tier", policy, now)
		return change, "", err
	}

	return nil, "", nil
}

func (s *Service) promoteByAccess(ctx context.Context, tx *db.Tx, item db.MemoryItem, policy lifecyclePolicy, now time.Time) (*LifecycleChange, error) {
	if policy.Rules.PromoteAccessCount <= 0 {
		return nil, nil
	}
	next, ok := promotionTarget(item.Tier)
	if !ok {
		return nil, nil
	}
	count, err := tx.AccessLog().CountSince(ctx, item.ID, now.Add(-policy.Rules.PromoteAccessWindow))
	if err != nil {
		return nil, mapDBError(err)
	}
	if count < policy.Rules.PromoteAccessCount {
		return nil, nil
	}
	return s.lifecycleTierChange(ctx, tx, item, next, "promoted", "recent access threshold reached", policy, now)
}

func (s *Service) lifecycleTierChange(ctx context.Context, tx *db.Tx, item db.MemoryItem, target db.Tier, action string, reason string, policy lifecyclePolicy, now time.Time) (*LifecycleChange, error) {
	update := db.MemoryUpdate{
		Tier:      &target,
		UpdatedAt: now,
	}
	if action == "promoted" && item.Tier == db.TierArchive {
		update.ClearArchivedAt = true
	}
	if err := tx.Memories().UpdateItem(ctx, item.ID, update); err != nil {
		return nil, mapDBError(err)
	}
	change := LifecycleChange{
		MemoryID: item.ID,
		Action:   action,
		FromTier: string(item.Tier),
		ToTier:   string(target),
		Reason:   reason,
		PolicyID: policy.ID,
	}
	return &change, s.recordLifecycleEvent(ctx, tx, change, now)
}

func (s *Service) lifecycleArchive(ctx context.Context, tx *db.Tx, item db.MemoryItem, policy lifecyclePolicy, now time.Time) (*LifecycleChange, error) {
	target := db.TierArchive
	if err := tx.Memories().UpdateItem(ctx, item.ID, db.MemoryUpdate{
		Tier:       &target,
		ArchivedAt: &now,
		UpdatedAt:  now,
	}); err != nil {
		return nil, mapDBError(err)
	}
	change := LifecycleChange{
		MemoryID: item.ID,
		Action:   "archived",
		FromTier: string(item.Tier),
		ToTier:   string(target),
		Reason:   "cold memory exceeded archive threshold",
		PolicyID: policy.ID,
	}
	return &change, s.recordLifecycleEvent(ctx, tx, change, now)
}

func (s *Service) lifecycleDelete(ctx context.Context, tx *db.Tx, item db.MemoryItem, policy lifecyclePolicy, now time.Time) (*LifecycleChange, error) {
	if err := tx.Memories().UpdateItem(ctx, item.ID, db.MemoryUpdate{
		DeletedAt: &now,
		UpdatedAt: now,
	}); err != nil {
		return nil, mapDBError(err)
	}
	change := LifecycleChange{
		MemoryID: item.ID,
		Action:   "deleted",
		FromTier: string(item.Tier),
		Reason:   "archive retention threshold exceeded",
		PolicyID: policy.ID,
	}
	return &change, s.recordLifecycleEvent(ctx, tx, change, now)
}

func (s *Service) recordLifecycleEvent(ctx context.Context, tx *db.Tx, change LifecycleChange, now time.Time) error {
	payload, err := json.Marshal(change)
	if err != nil {
		return err
	}
	_, err = tx.Memories().RecordEvent(ctx, db.MemoryEvent{
		MemoryID:         change.MemoryID,
		EventType:        "lifecycle_" + change.Action,
		EventPayloadJSON: string(payload),
		CreatedAt:        now,
	})
	return mapDBError(err)
}

func loadLifecyclePolicies(ctx context.Context, repo *db.RetentionPolicyRepository) ([]lifecyclePolicy, error) {
	policies, err := repo.ListEnabled(ctx)
	if err != nil {
		return nil, mapDBError(err)
	}
	out := make([]lifecyclePolicy, 0, len(policies))
	for _, policy := range policies {
		parsed, err := parseLifecyclePolicy(policy)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

func parseLifecyclePolicy(policy db.RetentionPolicy) (lifecyclePolicy, error) {
	scope := policyScope{}
	if policy.ScopeJSON != "" {
		if err := json.Unmarshal([]byte(policy.ScopeJSON), &scope); err != nil {
			return lifecyclePolicy{}, fmt.Errorf("%w: invalid retention policy scope %q", ErrInvalid, policy.ID)
		}
	}
	rulesJSON := policyRulesJSON{}
	if policy.RulesJSON != "" {
		if err := json.Unmarshal([]byte(policy.RulesJSON), &rulesJSON); err != nil {
			return lifecyclePolicy{}, fmt.Errorf("%w: invalid retention policy rules %q", ErrInvalid, policy.ID)
		}
	}
	return lifecyclePolicy{
		ID:    policy.ID,
		Name:  policy.Name,
		Scope: scope,
		Rules: mergeLifecycleRules(defaultLifecycleRules(), rulesJSON),
	}, nil
}

func mergeLifecycleRules(base lifecycleRules, overrides policyRulesJSON) lifecycleRules {
	if overrides.DemoteWorkingAfterDays > 0 {
		base.DemoteWorkingAfter = days(overrides.DemoteWorkingAfterDays)
	}
	if overrides.DemoteHotAfterDays > 0 {
		base.DemoteHotAfter = days(overrides.DemoteHotAfterDays)
	}
	if overrides.DemoteWarmAfterDays > 0 {
		base.DemoteWarmAfter = days(overrides.DemoteWarmAfterDays)
	}
	if overrides.ArchiveColdAfterDays > 0 {
		base.ArchiveColdAfter = days(overrides.ArchiveColdAfterDays)
	}
	if overrides.DeleteArchivedAfterDays > 0 {
		base.DeleteArchivedAfter = days(overrides.DeleteArchivedAfterDays)
	}
	if overrides.PromoteAccessCount > 0 {
		base.PromoteAccessCount = overrides.PromoteAccessCount
	}
	if overrides.PromoteAccessWindowDays > 0 {
		base.PromoteAccessWindow = days(overrides.PromoteAccessWindowDays)
	}
	if overrides.ProtectPinned != nil {
		base.ProtectPinned = *overrides.ProtectPinned
	}
	if overrides.ProtectImportanceAtOrAbove > 0 {
		base.ProtectImportanceAtOrAbove = overrides.ProtectImportanceAtOrAbove
	}
	return base
}

func policyForItem(item db.MemoryItem, policies []lifecyclePolicy) lifecyclePolicy {
	for _, policy := range policies {
		if policy.Scope.matches(item) {
			return policy
		}
	}
	return lifecyclePolicy{
		ID:    "builtin-default",
		Name:  "Built-in default lifecycle policy",
		Scope: policyScope{},
		Rules: defaultLifecycleRules(),
	}
}

func (s policyScope) matches(item db.MemoryItem) bool {
	if len(s.Tiers) > 0 && !slices.Contains(s.Tiers, string(item.Tier)) {
		return false
	}
	if len(s.Sources) > 0 && !slices.Contains(s.Sources, item.Source) {
		return false
	}
	if s.IncludePinned != nil && !*s.IncludePinned && item.Pinned {
		return false
	}
	return true
}

func demotionTarget(item db.MemoryItem, rules lifecycleRules, now time.Time) (db.Tier, bool) {
	idleSince := lastActivity(item)
	switch item.Tier {
	case db.TierWorking:
		return db.TierHot, !now.Before(idleSince.Add(rules.DemoteWorkingAfter))
	case db.TierHot:
		return db.TierWarm, !now.Before(idleSince.Add(rules.DemoteHotAfter))
	case db.TierWarm:
		return db.TierCold, !now.Before(idleSince.Add(rules.DemoteWarmAfter))
	default:
		return "", false
	}
}

func promotionTarget(tier db.Tier) (db.Tier, bool) {
	switch tier {
	case db.TierArchive:
		return db.TierCold, true
	case db.TierCold:
		return db.TierWarm, true
	case db.TierWarm:
		return db.TierHot, true
	default:
		return "", false
	}
}

func wouldChangeByAge(item db.MemoryItem, rules lifecycleRules, now time.Time) bool {
	if item.Tier == db.TierArchive && rules.DeleteArchivedAfter > 0 && item.ArchivedAt != nil && !now.Before(item.ArchivedAt.Add(rules.DeleteArchivedAfter)) {
		return true
	}
	if item.Tier == db.TierCold && !now.Before(lastActivity(item).Add(rules.ArchiveColdAfter)) {
		return true
	}
	_, ok := demotionTarget(item, rules, now)
	return ok
}

func lastActivity(item db.MemoryItem) time.Time {
	latest := item.UpdatedAt
	if item.LastAccessedAt != nil && item.LastAccessedAt.After(latest) {
		latest = *item.LastAccessedAt
	}
	if item.CreatedAt.After(latest) {
		latest = item.CreatedAt
	}
	return latest
}

func normalizeLifecycleLimit(limit int) int {
	if limit <= 0 {
		return defaultLifecycleLimit
	}
	if limit > maxLifecycleLimit {
		return maxLifecycleLimit
	}
	return limit
}

func days(value int) time.Duration {
	return time.Duration(value) * 24 * time.Hour
}
