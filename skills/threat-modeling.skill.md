# Threat Modeling Skill

## Purpose

Review Pamie's public MCP server attack surface and data handling risks.

## When to Use

Use before adding public endpoints, new MCP tools, backup/export, auth changes, or deployment guidance.

## Inputs

- Endpoint and tool list.
- Data flow.
- Auth model.
- Deployment model.
- Stored data categories.

## Step-by-step Procedure

1. Identify assets.
2. Identify trust boundaries.
3. List realistic attackers and misuse cases.
4. Map threats to controls.
5. Identify missing tests or documentation.
6. Create actionable hardening tasks.
7. Update `SECURITY.md` and `docs/threat-model.md`.

## Output Format

- Threat model notes.
- Risk-ranked findings.
- Mitigation tasks.
- Documentation updates.

## Checklist

- [ ] Auth is reviewed.
- [ ] Prompt injection is reviewed.
- [ ] Tool misuse is reviewed.
- [ ] Backup exposure is reviewed.
- [ ] Logs and audit data are reviewed.

## Common Mistakes

- Treating MCP clients as trusted.
- Ignoring stored memory as an input channel.
- Forgetting backup files.
- Producing vague risks without mitigations.

## Security Considerations

Prioritize controls that reduce blast radius: narrow tools, explicit auth, scopes, rate limits, audit logs, and safe defaults.
