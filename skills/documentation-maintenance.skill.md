# Documentation Maintenance Skill

## Purpose

Keep architecture, roadmap, task, security, and operator documentation synchronized with implementation.

## When to Use

Use when behavior changes, roadmap phases complete, security assumptions change, or new operational procedures are added.

## Inputs

- Code changes.
- Design decisions.
- Test results.
- Roadmap phase.
- Security review notes.

## Step-by-step Procedure

1. Identify docs affected by the change.
2. Update current behavior before future behavior.
3. Mark completed tasks in `TASKS.md`.
4. Add ADRs for meaningful architecture decisions.
5. Update security docs for endpoint, auth, or data changes.
6. Keep examples runnable and minimal.
7. Remove stale statements.

## Output Format

- Updated Markdown files.
- Clear summary of behavior changes.
- Task and ADR updates when needed.

## Checklist

- [ ] README current status is accurate.
- [ ] Architecture docs match code boundaries.
- [ ] Security docs match endpoint behavior.
- [ ] Roadmap and tasks are in sync.
- [ ] Examples use current commands.

## Common Mistakes

- Updating only README.
- Leaving future plans written as current behavior.
- Forgetting security docs.
- Adding examples that cannot run.

## Security Considerations

Docs should not normalize insecure public deployment. Examples must avoid real tokens, secrets, and private memory content.
