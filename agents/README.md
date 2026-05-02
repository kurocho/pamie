# Agents

This directory defines specialist AI coding agent roles for future Pamie work. These files are operating briefs, not executable code.

Use an agent definition when a task needs that role's perspective, review criteria, or implementation discipline. Agents should coordinate through documented interfaces, tests, and pull request summaries rather than implicit assumptions.

## Available Agents

- `product-owner.agent.md`: product scope, roadmap, semantics, and acceptance criteria.
- `go-architect.agent.md`: Go architecture, interfaces, errors, logging, and maintainability.
- `mcp-protocol.agent.md`: MCP transport, tools, resources, and protocol compatibility.
- `sqlite-storage.agent.md`: SQLite schema, migrations, WAL, FTS5, backups, and durability.
- `memory-engine.agent.md`: tiers, lifecycle, retention, pinned behavior, and events.
- `search-ranking.agent.md`: FTS search, scoring, snippets, filters, and future hybrid search.
- `security-reviewer.agent.md`: auth, prompt injection, audit logs, rate limits, and threat model.
- `test-engineer.agent.md`: unit, integration, acceptance, fixture, and CI coverage.
- `devops-release.agent.md`: Docker, Compose, CI, release artifacts, deployment docs, and backup scripts.
