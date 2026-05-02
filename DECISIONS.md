# Decisions

This file records initial Architecture Decision Records. Each ADR should be revisited when implementation evidence invalidates the assumptions.

## ADR-0001: Use Go for the Service

Status: Accepted

Pamie will be written in Go because the target is a small, deployable infrastructure binary with predictable concurrency, straightforward cross-compilation, and strong standard-library support for HTTP services. Go is also a practical fit for a self-hosted service that should be easy to operate on laptops, homelabs, NAS devices, and VPS hosts.

## ADR-0002: Use SQLite as Local Source of Truth

Status: Accepted

SQLite will be the durable local database for MVP and near-term production use. It keeps Pamie local-first, easy to back up, and independent of hosted database services. The storage layer must still be designed carefully around migrations, WAL mode, transaction boundaries, and backup behavior.

## ADR-0003: Use FTS5 for MVP Search

Status: Accepted

SQLite FTS5 will power initial keyword search. It provides a good local search baseline without adding a separate search service. Ranking should be designed so future semantic search can be blended with FTS5 instead of replacing it.

## ADR-0004: Use Bearer Token Auth for MVP

Status: Accepted

Bearer token authentication is the simplest viable protection for an MCP HTTP endpoint. MVP may start with a configured token, but hardening must include hashed tokens, token IDs, scopes, rate limits, audit logging, and clear public deployment guidance.

## ADR-0005: Do Not Expose Raw SQL or Shell Tools

Status: Accepted

Pamie must expose purpose-built memory operations only. MCP clients must not be able to execute arbitrary SQL or shell commands through Pamie. This reduces data exfiltration, destructive-operation, lateral-movement, and prompt-injection risk.

## ADR-0006: Treat Stored Memory as Untrusted Data

Status: Accepted

Stored memories may contain malicious instructions, prompt-injection attempts, private data, or stale context. Retrieval responses must preserve provenance and avoid treating memory content as trusted system instructions.

## ADR-0007: Make Vector Search Optional and Future-Ready

Status: Accepted

Vector search is valuable but not required for the initial scaffold or MVP storage baseline. Search architecture should define narrow interfaces that allow sqlite-vec, libSQL vector search, or local embedding implementations later without forcing a hosted provider.

## ADR-0008: License Pamie Under AGPL-3.0-only

Status: Accepted

Pamie is network server software intended to be self-hosted and potentially offered as a service. The project will use the GNU Affero General Public License v3.0 only (`AGPL-3.0-only`) so users can run, study, modify, and share the software, while modified network deployments must offer corresponding source code to their users under the license terms.

## ADR-0009: Use modernc.org/sqlite as the Go SQLite Driver

Status: Accepted

Pamie will use `modernc.org/sqlite` for the initial SQLite implementation. The driver is pure Go, which keeps default builds free of CGO requirements and better matches Pamie's goal of simple self-hosted deployment. The selected version is pinned to keep the module compatible with Go 1.22 while still providing SQLite and FTS5 support for the MVP storage layer.

## ADR-0010: Use Soft Delete for Lifecycle and Tool Deletion

Status: Accepted

Pamie will mark memories as deleted with `deleted_at` for the current implementation instead of physically deleting rows. This keeps deletion auditable through `memory_events`, preserves event history, and avoids surprising data loss while retention policy semantics mature. Physical purge can be added later as a separate explicit policy and operator action.

## ADR-0011: Enforce Scopes at the MCP Boundary

Status: Accepted

Pamie will attach a scoped principal to authenticated MCP requests and enforce authorization before invoking tools or resources. The first implementation supports one configured token with a token ID, in-memory token hash, and scope set. This keeps service methods simple while preventing MCP clients from calling read, write, delete, stats, backup, or admin operations without explicit authorization.

## ADR-0012: Keep Runtime Packages Behind Narrow Interfaces

Status: Accepted

Pamie will keep package dependencies flowing from composition and protocol layers toward domain and storage behavior, with `cmd/pamie` as the wiring point. MCP tools and resources depend on memory service interfaces, and the memory service depends on a storage interface instead of a concrete SQLite store. The storage package still owns SQL, migrations, transactions, and repository validation.

Small shared helpers may live in `internal/util` only when multiple packages need identical behavior. This avoids broad utility APIs while keeping duplicated validation rules, such as strict JSON object decoding, consistent across package boundaries.
