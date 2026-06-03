# imp4 Task Index — PostgreSQL Migration

This directory contains 28 atomic tasks that compose the 9Router PostgreSQL migration. Tasks are organized by domain and ordered by dependency.

## Domain Map

| # | Domain | Tasks | Priority Range |
|---|--------|-------|----------------|
| 1 | [Adapter Foundation](./01-adapter-foundation/) | 3 | P0 |
| 2 | [PostgreSQL Schema](./02-schema-postgres/) | 3 | P0-P1 |
| 3 | [Driver Selection](./03-driver-selection/) | 2 | P0-P1 |
| 4 | [Repository Migration (Easy)](./04-repository-migration-easy/) | 10 | P0-P2 |
| 5 | [usageRepo Rewrite](./05-usagerepo-rewrite/) | 4 | P0 (high risk) |
| 6 | [K8s Deployment](./06-k8s-deployment/) | 4 | P0 |
| 7 | [Data Migration](./07-data-migration/) | 2 | P0-P1 |
| 8 | [Testing Infrastructure](./08-testing-infrastructure/) | 3 | P0-P1 |
| 9 | [Documentation](./09-documentation/) | 2 | P1 |

## Task List (28 atomic tasks)

### Domain 1: Adapter Foundation
- [01.01-postgres-adapter-core](./01-adapter-foundation/01.01-postgres-adapter-core.md) — P0, L effort
- [01.02-adapter-interface-tests](./01-adapter-foundation/01.02-adapter-interface-tests.md) — P0, M effort
- [01.03-postgres-driver-install](./01-adapter-foundation/01.03-postgres-driver-install.md) — P0, XS effort

### Domain 2: PostgreSQL Schema
- [02.01-postgres-schema-tables](./02-schema-postgres/02.01-postgres-schema-tables.md) — P0, M effort
- [02.02-postgres-schema-indexes](./02-schema-postgres/02.02-postgres-schema-indexes.md) — P0, S effort
- [02.03-postgres-schema-seed](./02-schema-postgres/02.03-postgres-schema-seed.md) — P1, S effort

### Domain 3: Driver Selection
- [03.01-driver-env-detection](./03-driver-selection/03.01-driver-env-detection.md) — P0, M effort
- [03.02-driver-fallback-logic](./03-driver-selection/03.02-driver-fallback-logic.md) — P1, XS effort

### Domain 4: Repository Migration (Easy)
- [04.01-settings-repo](./04-repository-migration-easy/04.01-settings-repo.md) — P0
- [04.02-apikeys-repo](./04-repository-migration-easy/04.02-apikeys-repo.md) — P0
- [04.03-connections-repo](./04-repository-migration-easy/04.03-connections-repo.md) — P0
- [04.04-proxy-pools-repo](./04-repository-migration-easy/04.04-proxy-pools-repo.md) — P1
- [04.05-request-details-repo](./04-repository-migration-easy/04.05-request-details-repo.md) — P0
- [04.06-request-logs-repo](./04-repository-migration-easy/04.06-request-logs-repo.md) — P2 (verify only)
- [04.07-batch-requests-repo](./04-repository-migration-easy/04.07-batch-requests-repo.md) — P2 (verify only)
- [04.08-rate-limits-repo](./04-repository-migration-easy/04.08-rate-limits-repo.md) — P1 (verify only)
- [04.09-rules-repo](./04-repository-migration-easy/04.09-rules-repo.md) — P1 (verify only)
- [04.10-profiles-repo](./04-repository-migration-easy/04.10-profiles-repo.md) — P0

### Domain 5: usageRepo Rewrite (HIGH RISK)
- [05.01-usagerepo-batch-insert](./05-usagerepo-rewrite/05.01-usagerepo-batch-insert.md) — P0
- [05.02-usagerepo-json-aggregation](./05-usagerepo-rewrite/05.02-usagerepo-json-aggregation.md) — P0
- [05.03-usagerepo-async-flush](./05-usagerepo-rewrite/05.03-usagerepo-async-flush.md) — P0
- [05.04-usagerepo-timescale-consider](./05-usagerepo-rewrite/05.04-usagerepo-timescale-consider.md) — P0

### Domain 6: K8s Deployment
- [06.01-k8s-postgres-statefulset](./06-k8s-deployment/06.01-k8s-postgres-statefulset.md) — P0
- [06.02-k8s-9router-deployment](./06-k8s-deployment/06.02-k8s-9router-deployment.md) — P0
- [06.03-k8s-services](./06-k8s-deployment/06.03-k8s-services.md) — P0
- [06.04-k8s-secrets](./06-k8s-deployment/06.04-k8s-secrets.md) — P0

### Domain 7: Data Migration
- [07.01-migration-script-read](./07-data-migration/07.01-migration-script-read.md) — P0
- [07.02-migration-script-write](./07-data-migration/07.02-migration-script-write.md) — P1

### Domain 8: Testing Infrastructure
- [08.01-docker-compose](./08-testing-infrastructure/08.01-docker-compose.md) — P0
- [08.02-integration-tests](./08-testing-infrastructure/08.02-integration-tests.md) — P0
- [08.03-ci-pipeline](./08-testing-infrastructure/08.03-ci-pipeline.md) — P1

### Domain 9: Documentation
- [09.01-user-guide](./09-documentation/09.01-user-guide.md) — P1
- [09.02-runbook](./09-documentation/09.02-runbook.md) — P1

## Execution Order

### Wave 1 (Foundation, sequential)
1. 01.03 (install pg)
2. 01.01 (postgresAdapter)
3. 01.02 (adapter tests)
4. 02.01 (schema tables)
5. 02.02 (schema indexes)
6. 02.03 (schema seed)
7. 03.01 (driver env detection)
8. 03.02 (PRAGMA guard)

### Wave 2 (Parallel after Wave 1)
- Domain 4 (all 10 repo tasks) — can be split across team members
- Domain 6 (all 4 K8s tasks)
- Domain 8.01 (docker-compose for tests)

### Wave 3 (After Wave 2)
- Domain 5 (usageRepo rewrite, sequential — all 4 tasks)
- Domain 7.01 (migration script — depends on Domain 5 rollup tables)
- Domain 7.02 (rollback script — depends on 7.01)
- Domain 8.02 (integration tests)
- Domain 8.03 (CI pipeline)

### Wave 4 (Final)
- Domain 9.01 (user guide)
- Domain 9.02 (runbook + changelog)

## Key Risk Areas

- **Domain 5 (usageRepo rewrite)**: The JSON-blob-to-normalized-tables change is the most architecturally significant. Concurrent write correctness is load-bearing. Output shape must be bit-identical for the dashboard.
- **Domain 5.03 (atomic counter)**: The race condition in the read-modify-write pattern must be fixed.
- **Domain 7 (data migration)**: Large databases need careful streaming; JSON blob parsing can fail on malformed data.

## Success Criteria (overall)

- [ ] 9Router runs on PostgreSQL with `POSTGRES_URL` set
- [ ] SQLite path still works when `POSTGRES_URL` is unset
- [ ] All existing unit tests pass with both adapters
- [ ] 100 parallel writes produce 100 rows (counter invariant)
- [ ] Multi-replica deployment (2+ pods) is safe for concurrent writes
- [ ] Data migration from SQLite to PostgreSQL preserves all data
- [ ] K8s deployment manifests deploy a working stack
- [ ] Documentation covers deployment, migration, and troubleshooting
