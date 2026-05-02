# Lifecycle Retention Skill

## Purpose

Implement retention policy, promotion, demotion, archive, and deletion behavior.

## When to Use

Use when adding lifecycle jobs, retention policy schema, deletion rules, archive behavior, or promotion on access.

## Inputs

- Tier rules.
- Policy definitions.
- Access logs.
- Memory events.
- Operator deletion expectations.

## Step-by-step Procedure

1. Define policy scope and precedence.
2. Evaluate lifecycle decisions without mutating state.
3. Apply mutations in transactions.
4. Protect pinned memory by default.
5. Archive before deletion when policy requires it.
6. Record every lifecycle change as an event.
7. Add tests for boundary timestamps and policy exceptions.

## Output Format

- Lifecycle service code.
- Policy documentation.
- Tests for promotion, demotion, archive, and deletion.

## Checklist

- [ ] Deletion requires explicit policy or authorized request.
- [ ] Pinned memory is protected by default.
- [ ] Lifecycle jobs are deterministic.
- [ ] Events explain every transition.
- [ ] Tests use controlled clocks.

## Common Mistakes

- Hard-deleting during demotion.
- Making policies impossible to explain.
- Running lifecycle without transactions.
- Forgetting audit implications.

## Security Considerations

Deletion and retention changes are sensitive operations. They should be authorized, audited, and reversible where practical.
