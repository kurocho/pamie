# Docker Deployment Skill

## Purpose

Design Docker, Compose, Caddy, VPS, and NAS deployment assets for Pamie.

## When to Use

Use after the HTTP server exists and deployment artifacts are needed.

## Inputs

- Binary build process.
- Runtime config variables.
- Data directory path.
- Port and health endpoints.
- Backup requirements.

## Step-by-step Procedure

1. Confirm the binary can run outside Docker.
2. Add a multi-stage Dockerfile.
3. Run as a non-root user where practical.
4. Mount a persistent data volume.
5. Add Compose with explicit environment variables.
6. Add reverse proxy examples with HTTPS.
7. Document backup and restore commands.

## Output Format

- Dockerfile.
- Compose file.
- Reverse proxy examples.
- Deployment docs.
- CI build updates.

## Checklist

- [ ] Image does not contain secrets.
- [ ] Data directory is persistent.
- [ ] Health check is defined.
- [ ] HTTPS guidance is present.
- [ ] Non-root runtime is considered.

## Common Mistakes

- Adding Docker before the server exists.
- Baking tokens into images.
- Using ephemeral SQLite storage.
- Publishing insecure public examples.

## Security Considerations

Public examples must require HTTPS and Bearer auth. Database and backup volumes need restrictive permissions.
