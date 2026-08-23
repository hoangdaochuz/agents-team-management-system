## Purpose

The system currently uses 10 logical databases, each serving a separate microservice. This
spec documents the consolidation to 4 logical databases, reducing migration pipeline complexity
while preserving the shared Postgres instance.

## ADDED Requirements

### Requirement: Consolidated database schema
The system SHALL use exactly 4 logical databases, each serving one consolidated service:

| Logical Database | Merged From | Core Tables |
|:-----------------|:------------|:------------|
| `identity_db` | auth_db, orgs_db, admin_db | users, sessions, organizations, workspaces, members, invites, signup_requests, feature_flags, audit_log |
| `workspace_db` | project_db, task_db, catalog_db, resources_db | projects, tasks, task_thread, feedback, skills, mcp_servers, knowledge_sources, plugins, rules, mcp_connections |
| `agent_db` | agent_db, settings_db | agents, agent_skills, agent_mcps, provider_keys |
| `runner_db` | runner_db (unchanged) | runs, steps, findings, artifacts |

### Requirement: Single migration pipeline
All database migrations SHALL be consolidated into a single migration pipeline that creates all
tables across the 4 databases. The current pipeline has 10 separate migration entry points
(one per service); post-consolidation, there SHALL be 4 migration entry points (one per database).

- **Current**: 10 migration pipelines, each with its own up/down scripts, tracking their own
  schema version.
- **Target**: 4 migration pipelines, each creating tables for its respective database.
  Shared infrastructure (`internal/platform/db/`) remains unchanged.

### Requirement: No isolation benefit retained
The consolidation SHALL document that the 10 databases previously provided zero meaningful
data isolation:
- All databases reside on the same Postgres instance.
- All share the same credentials (managed via the secret management pipeline).
- All share the same failure domain (a single Postgres outage takes down all services).
- Cross-service joins were impossible without distributed transactions; the consolidation
  actually enables simpler data modeling within each database.

### Requirement: Provider key encryption boundary
The `provider_keys` table within `agent_db` SHALL maintain the encryption boundary:
- Provider keys are encrypted at rest using AES-GCM.
- The master key resides in memory within the Agent Service only.
- No other service (including the Gateway or external systems) ever accesses raw provider keys.
- The credential-less-sandbox invariant is preserved: provider keys never reach container
  env/filesystem/logs.

### Requirement: Database connection pooling
Connection pools SHALL be configured per-consolidated-database rather than per-service.
The shared `internal/platform/db/` package handles pool configuration, so this change is
transparent to service code — only the `UPSTREAM_*` environment variables and database
connection strings change.

### Requirement: Migration backward compatibility
The consolidation SHALL not break existing migration workflows for new features. Adding a
new table or column SHALL follow the same pattern as before, just within one of the 4
consolidated databases rather than a separate service database.