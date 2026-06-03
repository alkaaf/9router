# Domain 9: Documentation

## Overview

Write user-facing and operator-facing documentation for the PostgreSQL migration:
deployment runbook, environment variable reference, migration guide, troubleshooting
guide, and changelog entry.

## Scope

- **In scope**: K8s deployment guide, env var reference, SQLite→PG migration
  runbook, troubleshooting, changelog
- **Out of scope**: Internal developer docs (separate effort), API docs (unchanged)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `docs/postgresql-deployment.md` | CREATE | ~120 |
| `docs/postgresql-migration-runbook.md` | CREATE | ~100 |
| `docs/postgresql-env-vars.md` | CREATE | ~60 |
| `docs/postgresql-troubleshooting.md` | CREATE | ~80 |
| `CHANGELOG.md` | MODIFY | +30 |

## Dependencies

All implementation domains (1-7) should be complete before finalizing docs.
Testing infrastructure (Domain 8) helps validate the documented procedures.

## Sub-Tasks

1. **Write PostgreSQL deployment guide**
   - Description: Step-by-step guide for deploying 9Router with PostgreSQL on
     Kubernetes. Covers: prerequisites (K8s cluster, kubectl), deploying PostgreSQL
     (self-hosted StatefulSet or managed service reference), deploying 9Router,
     creating the Secret with POSTGRES_URL, verifying the deployment, scaling
     replicas. Include a "managed PostgreSQL" section recommending RDS/Cloud SQL
     for production.
   - Acceptance: A new operator can follow the guide and have a working 9Router +
     PostgreSQL deployment within 30 minutes.
   - Risk: Low

2. **Write environment variable reference**
   - Description: Document all PostgreSQL-related env vars:
     `POSTGRES_URL` (required for PG mode), `POSTGRES_POOL_SIZE` (default 20),
     `POSTGRES_SSL` (default false). For each: type, default, required, description,
     example values (local Docker, managed PG, K8s). Include the SQLite fallback
     behavior: when POSTGRES_URL is unset, the app uses SQLite automatically.
   - Acceptance: All env vars documented with examples. SQLite fallback behavior
     clearly explained.
   - Risk: Low

3. **Write SQLite → PostgreSQL migration runbook**
   - Description: Step-by-step procedure for migrating an existing 9Router SQLite
     installation to PostgreSQL:
     1. Prerequisites: PostgreSQL instance running, network access from 9Router host
     2. Back up SQLite database (`cp data.db data.db.bak`)
     3. Install pg client libraries if needed
     4. Run `node scripts/migrate-sqlite-to-postgres.js --sqlite ./data.db
        --postgres <dsn>`
     5. Verify: check row counts, spot-check usage data
     6. Configure `POSTGRES_URL` in 9Router env
     7. Start 9Router with `POSTGRES_URL` set
     8. Monitor for 24h, verify all features work
     9. Rollback procedure: unset `POSTGRES_URL` → auto-fallback to SQLite
   - Acceptance: Runbook is clear enough for a non-developer operator to execute.
   - Risk: Medium (incomplete runbook leads to data loss)

4. **Write troubleshooting guide**
   - Description: Common issues and solutions:
     - "9Router won't start with POSTGRES_URL set" → check connection string format,
       network connectivity, PostgreSQL running
     - "Connection pool exhausted" → increase POSTGRES_POOL_SIZE, check for
       connection leaks
     - "Data looks wrong after migration" → verify migration completed, check
       row counts, re-run migration if needed
     - "Performance is slower than SQLite" → check connection pool, network latency,
       consider PgBouncer sidecar
     - "Dashboard shows no usage data" → verify usageHistory rows in PG, check
       adapter selection log output
   - Acceptance: Each issue has a clear symptom → cause → fix path.
   - Risk: Low

5. **Write changelog entry**
   - Description: Add to CHANGELOG.md:
     ```
     ## [X.Y.Z] — 2026-06-03
     ### Added
     - PostgreSQL adapter support — set POSTGRES_URL to enable
     - Kubernetes deployment manifests for multi-replica 9Router
     - Data migration script: SQLite → PostgreSQL
     ### Changed
     - Usage aggregation now uses normalized rollup tables (PostgreSQL mode)
     - Settings and API key lookups support PostgreSQL JSONB columns
     ### Migration
     - Set POSTGRES_URL to migrate from SQLite to PostgreSQL
     - Unset POSTGRES_URL to revert to SQLite (fallback is automatic)
     ```
   - Acceptance: Changelog entry clearly describes the change, migration path,
     and rollback behavior.
   - Risk: Low

6. **Update README.md with PostgreSQL section**
   - Description: Add a "PostgreSQL / Kubernetes" section to the main README:
     brief description, link to deployment guide, minimum requirements (PostgreSQL
     14+), architecture diagram (text-based).
   - Acceptance: README mentions PostgreSQL as a supported backend with a link
     to detailed docs.
   - Risk: Low

## Effort Estimate

2-3 days

## Risk Level

Low — documentation is non-blocking; can be written in parallel with testing.

## Testing Requirements

- Unit tests: N/A
- Integration tests: N/A
- Manual testing: Have a team member follow the deployment guide and migration
  runbook on a fresh environment to verify clarity and completeness.
