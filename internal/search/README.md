# internal/search

Owner of search ranking policy and hybrid search interfaces.

## Responsibilities

- Define search depth semantics.
- Rank FTS5 and optional vector candidates with keyword, vector, recency, tier, pinned, importance, and access signals.
- Expose explainable score components.
- Define vector search extension interfaces.

## Non-Responsibilities

- MCP request parsing.
- Memory lifecycle policy.
- Token authentication.
- SQLite query construction and migrations.

## Current Implementation

The SQLite repository performs FTS5 matching and filter application. This package provides reusable ranking primitives:

- `Depth`: `shallow`, `standard`, and `deep` candidate breadth.
- `Score`: explainable score components returned to callers.
- `ScoreCandidate`: deterministic ranking over keyword, vector, and lifecycle signals.
- `VectorSearcher`: an extension point for alternate local vector backends.

## Boundary

Search should return structured results with provenance. It should not summarize memory content as trusted instructions.
