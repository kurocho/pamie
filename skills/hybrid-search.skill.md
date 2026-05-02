# Hybrid Search Skill

## Purpose

Design search that blends keyword, semantic, metadata, recency, tier, and scoring signals.

## When to Use

Use when adding search ranking, filters, snippets, search depth, vector interfaces, or ranking tests.

## Inputs

- FTS5 query behavior.
- Metadata filter requirements.
- Memory tier and recency signals.
- Future vector search constraints.

## Step-by-step Procedure

1. Implement reliable FTS5 search first.
2. Add filters before ranking complexity.
3. Define scoring signals and weights.
4. Add snippets with source IDs.
5. Add search depth controls and limits.
6. Add regression fixtures for ranking.
7. Keep vector search behind an optional interface.

## Output Format

- Search interfaces and implementation.
- Ranking tests.
- Search design updates.

## Checklist

- [ ] Empty and malformed queries are handled.
- [ ] Result limits are enforced.
- [ ] Filters are tested.
- [ ] Ranking is explainable.
- [ ] Vector search remains optional.

## Common Mistakes

- Adding embeddings before keyword search works.
- Returning unbounded results.
- Making ranking impossible to test.
- Letting semantic similarity ignore metadata filters.

## Security Considerations

Search results may surface sensitive or adversarial content. Preserve provenance and enforce authorization before search.
