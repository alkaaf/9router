# Domain Breakdown Index — imp4 PostgreSQL Migration

Nine implementation domains for the 9Router PostgreSQL migration, ordered by dependency.
Domains 4, 8, and 9 can be executed in parallel after Domains 1-3 complete.

| # | Domain File | Summary | Effort | Risk | Depends On |
|---|-------------|---------|--------|------|------------|
| 1 | [01-adapter-foundation.md](01-adapter-foundation.md) | postgresAdapter.js, adapter interface, singleton | 3-5 days | Low | — |
| 2 | [02-schema-postgres.md](02-schema-postgres.md) | schema.postgres.js: 12 tables, 19 indexes, type mapping | 2-3 days | Low | Domain 1 |
| 3 | [03-driver-selection.md](03-driver-selection.md) | driver.js: env-based POSTGRES_URL detection, fallback chain | 1-2 days | Low | Domain 1 |
| 4 | [04-repository-migration-easy.md](04-repository-migration-easy.md) | 10 repos: remove JSON parse/stringify, verify adapter interface | 3-5 days | Low | Domains 1-3 |
| 5 | [05-usagerepo-rewrite.md](05-usagerepo-rewrite.md) | usageRepo.js: JSONB rewrite, normalized aggregation, multi-row INSERT | 5-10 days | High | Domain 4 |
| 6 | [06-k8s-deployment.md](06-k8s-deployment.md) | StatefulSet, Deployment, Services, Secrets, PVC | 3-5 days | Low | Domain 2 |
| 7 | [07-data-migration.md](07-data-migration.md) | scripts/migrate-sqlite-to-postgres.js, rollback | 2-3 days | Medium | Domains 1-2 |
| 8 | [08-testing-infrastructure.md](08-testing-infrastructure.md) | Docker Compose, integration tests, CI pipeline | 2-3 days | Low | Domains 1-4 |
| 9 | [09-documentation.md](09-documentation.md) | User guides, runbooks, changelog, K8s deploy docs | 2-3 days | Low | Domains 1-7 |

## Parallelization Strategy

**Wave 1** (sequential): Domains 1 → 2 → 3 (foundation must be solid)
**Wave 2** (parallel after Wave 1): Domain 4, Domain 6, Domain 8
**Wave 3** (after Wave 2): Domain 5 (depends on 4), Domain 7 (depends on 1-2), Domain 9 (depends on all)
**Wave 4** (final): Domain 9 completes after all implementation done
