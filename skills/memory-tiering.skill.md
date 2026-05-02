# Memory Tiering Skill

## Purpose

Design working, hot, warm, cold, and archive memory behavior.

## When to Use

Use when defining tier semantics, ranking tier boosts, lifecycle transitions, or user-facing tier documentation.

## Inputs

- Product semantics.
- Memory age and access signals.
- Pinned and importance behavior.
- Retention policy constraints.

## Step-by-step Procedure

1. Define what each tier means operationally.
2. Identify transition signals for promotion and demotion.
3. Define pinned and important exceptions.
4. Decide how tiers affect ranking and retrieval.
5. Record tier changes as events.
6. Add deterministic tests for edge cases.
7. Document the behavior for operators.

## Output Format

- Tier rules.
- Domain code or design docs.
- Tests and examples.

## Checklist

- [ ] Fresh memory starts in the expected tier.
- [ ] Old memory remains retrievable.
- [ ] Pinned memory remains easy to retrieve.
- [ ] Promotion and demotion are explainable.
- [ ] Events record tier changes.

## Common Mistakes

- Making tiers pure labels with no behavior.
- Deleting old memory without retention policy.
- Letting archive mean inaccessible.
- Ignoring access frequency.

## Security Considerations

Tiering should not make untrusted memory authoritative. Ranking higher does not mean trusting the content more.
