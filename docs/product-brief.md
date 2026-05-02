# Product Brief

Pamie is a local long-term memory server for MCP agents. It gives agents a durable place to store, retrieve, and manage context across sessions without depending on a hosted AI provider or managed search service.

Website: <https://pamie.io>

Docker Hub: <https://hub.docker.com/repository/docker/kurocho/pamie/general>

## Target Users

- Individual developers running local agents.
- Teams operating private MCP infrastructure.
- Homelab and NAS users who want self-hosted AI memory.
- Security-conscious users who want durable memory without provider lock-in.

## Core Use Cases

- Save project facts, decisions, and preferences during agent sessions.
- Retrieve recent and older context using MCP tools.
- Pin important memories so they remain easy to find.
- Retain memory safely with explicit policies.
- Export and back up memory data for operator control.

## Non-Goals for MVP

- Hosted SaaS.
- Multi-tenant enterprise administration.
- Built-in shell execution.
- Raw SQL access through MCP.
- Mandatory vector search or hosted embeddings.

## Success Criteria

- Operators can run Pamie as a single binary.
- MCP agents can save and retrieve memories through a narrow tool surface.
- SQLite remains the local source of truth.
- Search works well with FTS5 by default, with optional local vector ranking for operators who enable it.
- Security guidance is clear for public deployment.
